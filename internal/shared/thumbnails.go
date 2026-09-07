package shared

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/disintegration/imaging"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	DefaultThumbnailSize    = 200
	DefaultThumbnailQuality = 80
)

// ThumbnailCache provides a thread-safe cache for generated thumbnails.
type ThumbnailCache struct {
	mu    sync.RWMutex
	store map[string][]byte
	pool  *pgxpool.Pool
}

var thumbnailCache = &ThumbnailCache{
	store: make(map[string][]byte),
}

// SetThumbnailDBPool sets the PostgreSQL pool for L2 persistent thumbnail caching.
func SetThumbnailDBPool(pool *pgxpool.Pool) {
	thumbnailCache.mu.Lock()
	thumbnailCache.pool = pool
	thumbnailCache.mu.Unlock()
}

// GenerateImageThumbnail creates a JPEG thumbnail for an image file.
func GenerateImageThumbnail(ctx context.Context, path string, size int) ([]byte, error) {
	if size <= 0 {
		size = DefaultThumbnailSize
	}

	cacheKey := fmt.Sprintf("%s:%d", path, size)
	thumbnailCache.mu.RLock()
	if data, ok := thumbnailCache.store[cacheKey]; ok {
		thumbnailCache.mu.RUnlock()
		return data, nil
	}
	pool := thumbnailCache.pool
	thumbnailCache.mu.RUnlock()

	// Check L2 PostgreSQL cache
	var fileSize int64
	var modTime time.Time
	if st, err := os.Stat(path); err == nil {
		fileSize = st.Size()
		modTime = st.ModTime()
		if pool != nil {
			var thumbData []byte
			err := pool.QueryRow(ctx, `
				SELECT thumb_data FROM thumbnail_cache
				WHERE path = $1 AND file_size = $2 AND mod_time = $3 AND thumb_size = $4
				LIMIT 1;
			`, path, fileSize, modTime, size).Scan(&thumbData)
			if err == nil && len(thumbData) > 0 {
				thumbnailCache.mu.Lock()
				thumbnailCache.store[cacheKey] = thumbData
				thumbnailCache.mu.Unlock()
				return thumbData, nil
			}
		}
	}

	ext := NormaliseExt(filepath.Ext(path))

	if ext == ".avif" {
		data, err := extractImageFrame(ctx, path, "00:00:00", size)
		if err != nil {
			return nil, fmt.Errorf("failed to generate AVIF thumbnail %s: %w", path, err)
		}

		storeThumbData(ctx, pool, path, fileSize, modTime, size, cacheKey, data)
		return data, nil
	}

	img, err := imaging.Open(path, imaging.AutoOrientation(true))
	if err != nil {
		// Fallback to ffmpeg for formats that imaging can't decode.
		ffData, ffErr := extractImageFrame(ctx, path, "00:00:00", size)
		if ffErr == nil {
			storeThumbData(ctx, pool, path, fileSize, modTime, size, cacheKey, ffData)
			return ffData, nil
		}
		return nil, fmt.Errorf("failed to open image %s: %w (ffmpeg fallback: %v)", path, err, ffErr)
	}

	thumb := imaging.Fit(img, size, size, imaging.Lanczos)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, thumb, &jpeg.Options{Quality: DefaultThumbnailQuality}); err != nil {
		return nil, fmt.Errorf("failed to encode thumbnail: %w", err)
	}

	data := buf.Bytes()
	storeThumbData(ctx, pool, path, fileSize, modTime, size, cacheKey, data)
	return data, nil
}

// GenerateVideoThumbnail creates a JPEG thumbnail for a video file using ffmpeg.
func GenerateVideoThumbnail(ctx context.Context, path string, size int) ([]byte, error) {
	if size <= 0 {
		size = DefaultThumbnailSize
	}

	cacheKey := fmt.Sprintf("video:%s:%d", path, size)
	thumbnailCache.mu.RLock()
	if data, ok := thumbnailCache.store[cacheKey]; ok {
		thumbnailCache.mu.RUnlock()
		return data, nil
	}
	pool := thumbnailCache.pool
	thumbnailCache.mu.RUnlock()

	// Check L2 PostgreSQL cache
	var fileSize int64
	var modTime time.Time
	if st, err := os.Stat(path); err == nil {
		fileSize = st.Size()
		modTime = st.ModTime()
		if pool != nil {
			var thumbData []byte
			err := pool.QueryRow(ctx, `
				SELECT thumb_data FROM thumbnail_cache
				WHERE path = $1 AND file_size = $2 AND mod_time = $3 AND thumb_size = $4
				LIMIT 1;
			`, path, fileSize, modTime, size).Scan(&thumbData)
			if err == nil && len(thumbData) > 0 {
				thumbnailCache.mu.Lock()
				thumbnailCache.store[cacheKey] = thumbData
				thumbnailCache.mu.Unlock()
				return thumbData, nil
			}
		}
	}

	// Try extracting frame at 1 second, fallback to 0 seconds
	data, err := extractVideoFrame(ctx, path, "00:00:01", size)
	if err != nil {
		data, err = extractVideoFrame(ctx, path, "00:00:00", size)
		if err != nil {
			return nil, fmt.Errorf("failed to extract video frame from %s: %w", path, err)
		}
	}

	storeThumbData(ctx, pool, path, fileSize, modTime, size, cacheKey, data)
	return data, nil
}

func storeThumbData(ctx context.Context, pool *pgxpool.Pool, path string, fileSize int64, modTime time.Time, size int, cacheKey string, data []byte) {
	thumbnailCache.mu.Lock()
	thumbnailCache.store[cacheKey] = data
	thumbnailCache.mu.Unlock()

	if pool != nil && fileSize > 0 && !modTime.IsZero() {
		go func() {
			_, _ = pool.Exec(context.Background(), `
				INSERT INTO thumbnail_cache (path, file_size, mod_time, thumb_size, thumb_data, created_at)
				VALUES ($1, $2, $3, $4, $5, NOW())
				ON CONFLICT (path, file_size, mod_time, thumb_size) DO UPDATE SET
					thumb_data = EXCLUDED.thumb_data;
			`, path, fileSize, modTime, size, data)
		}()
	}
}

func extractVideoFrame(ctx context.Context, videoPath, seekTime string, size int) ([]byte, error) {
	img, err := extractImageFromFFmpeg(ctx, videoPath, seekTime, size)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, imaging.Fit(img, size, size, imaging.Lanczos), &jpeg.Options{Quality: DefaultThumbnailQuality}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func extractImageFrame(ctx context.Context, imagePath, seekTime string, size int) ([]byte, error) {
	img, err := extractImageFromFFmpeg(ctx, imagePath, seekTime, size)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, imaging.Fit(img, size, size, imaging.Lanczos), &jpeg.Options{Quality: DefaultThumbnailQuality}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// OpenImageFile opens an image from any format supported by imaging or ffmpeg.
func OpenImageFile(ctx context.Context, path string) (image.Image, error) {
	img, err := imaging.Open(path, imaging.AutoOrientation(true))
	if err == nil {
		return img, nil
	}
	// imaging failed — try ffmpeg (handles AVIF, HEIC, …)
	return extractImageFromFFmpeg(ctx, path, "00:00:00", 0)
}

func extractImageFromFFmpeg(ctx context.Context, srcPath, seekTime string, size int) (image.Image, error) {
	bins := FFmpegCandidates()
	if len(bins) == 0 {
		return nil, fmt.Errorf("ffmpeg not found in PATH")
	}

	var lastErr error
	for _, bin := range bins {
		for _, codec := range []string{"png", "mjpeg"} {
			args := []string{"-ss", seekTime, "-i", PathForBin(bin, srcPath), "-frames:v", "1"}
			if size > 0 {
				args = append(args, "-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", size, size))
			}
			args = append(args, "-f", "image2pipe", "-vcodec", codec)
			if codec == "mjpeg" {
				args = append(args, "-q:v", "5")
			}
			args = append(args, "pipe:1")

			var stdout, stderr bytes.Buffer
			cmd := exec.CommandContext(ctx, bin, args...)
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr

			if err := cmd.Run(); err != nil {
				lastErr = fmt.Errorf("%s/%s failed: %w (stderr: %s)", bin, codec, err, stderr.String())
				continue
			}
			if stdout.Len() == 0 {
				lastErr = fmt.Errorf("%s/%s: no output produced", bin, codec)
				continue
			}

			img, _, err := image.Decode(bytes.NewReader(stdout.Bytes()))
			if err != nil {
				lastErr = fmt.Errorf("%s/%s: decode failed: %w", bin, codec, err)
				continue
			}
			return img, nil
		}
	}
	return nil, fmt.Errorf("all ffmpeg variants failed for %s: %w", srcPath, lastErr)
}
