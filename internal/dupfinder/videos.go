package dupfinder

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"log/slog"
	"math/bits"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/andreassag/morphic/internal/database"
	"github.com/andreassag/morphic/internal/shared"
	"github.com/corona10/goimagehash"
	"github.com/jackc/pgx/v5/pgxpool"
)

// VideoInfo stores information about a video file.
type VideoInfo struct {
	Path        string   `json:"path"`
	Duration    float64  `json:"duration"`
	FPS         float64  `json:"fps"`
	FrameCount  int      `json:"frame_count"`
	Width       int      `json:"width"`
	Height      int      `json:"height"`
	FileSize    int64    `json:"file_size"`
	FrameHashes []uint64 `json:"-"`
	HasHash     bool     `json:"-"`
}

// ComputeVideoHashes extracts frames and computes perceptual hashes with PostgreSQL cache lookup.
func ComputeVideoHashes(ctx context.Context, pool *pgxpool.Pool, path string, numFrames int) VideoInfo {
	info := VideoInfo{Path: path}

	if ctx.Err() != nil {
		return info
	}

	st, err := os.Stat(path)
	if err != nil {
		return info
	}
	info.FileSize = st.Size()
	modTime := st.ModTime()

	// Check PostgreSQL cache
	if pool != nil {
		cached, err := database.LookupHash(ctx, pool, path, info.FileSize, modTime)
		if err == nil && cached != nil && cached.PHash != 0 {
			info.Width = cached.Width
			info.Height = cached.Height
			info.Duration = cached.Duration
			info.FPS = cached.FPS
			info.FrameHashes = []uint64{cached.PHash, cached.AHash, cached.DHash}
			info.HasHash = true
			return info
		}
	}

	candidates := shared.FFmpegCandidates()
	if len(candidates) == 0 {
		slog.Warn("dupfinder: ffmpeg not found in PATH")
		return info
	}
	ffmpegBin := candidates[0]
	probeBin := strings.Replace(ffmpegBin, "ffmpeg", "ffprobe", 1)

	// Get video metadata via ffprobe
	probeOut, err := exec.CommandContext(ctx, probeBin,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,duration,r_frame_rate,nb_frames",
		"-of", "csv=p=0",
		shared.PathForBin(probeBin, path),
	).Output()
	if err != nil {
		slog.Warn("dupfinder: ffprobe failed", "path", path, "err", err)
		return info
	}

	if ctx.Err() != nil {
		return info
	}

	parts := strings.Split(strings.TrimSpace(string(probeOut)), ",")
	if len(parts) >= 1 {
		info.Width, _ = strconv.Atoi(parts[0])
	}
	if len(parts) >= 2 {
		info.Height, _ = strconv.Atoi(parts[1])
	}
	if len(parts) >= 3 && parts[2] != "" && parts[2] != "N/A" {
		info.Duration, _ = strconv.ParseFloat(parts[2], 64)
	}
	if len(parts) >= 4 {
		info.FPS = parseFPS(parts[3])
	}
	if len(parts) >= 5 {
		info.FrameCount, _ = strconv.Atoi(parts[4])
	}

	if info.Duration <= 0 && info.FrameCount > 0 && info.FPS > 0 {
		info.Duration = float64(info.FrameCount) / info.FPS
	}

	if info.Duration <= 0 {
		return info
	}

	// Extract frames at regular intervals using ffmpeg in-memory pipe
	frameHashes := extractAndHashFrames(ctx, ffmpegBin, path, info.Duration, numFrames)
	info.FrameHashes = frameHashes
	info.HasHash = len(frameHashes) > 0

	// Store primary representative hashes in PostgreSQL
	if pool != nil && info.HasHash {
		var ph, ah, dh uint64
		if len(frameHashes) > 0 {
			ph = frameHashes[0]
		}
		if len(frameHashes) > 1 {
			ah = frameHashes[len(frameHashes)/2]
		}
		if len(frameHashes) > 2 {
			dh = frameHashes[len(frameHashes)-1]
		}

		_ = database.StoreHash(ctx, pool, database.HashRow{
			Path:     path,
			FileSize: info.FileSize,
			ModTime:  modTime,
			PHash:    ph,
			AHash:    ah,
			DHash:    dh,
			Width:    info.Width,
			Height:   info.Height,
			Duration: info.Duration,
			Format:   shared.NormaliseExt(filepath.Ext(path)),
			FPS:      info.FPS,
		})
	}

	return info
}

// extractAndHashFrames extracts frames at intervals and hashes them in-memory without temp files.
func extractAndHashFrames(ctx context.Context, bin, path string, duration float64, numFrames int) []uint64 {
	startTime := duration * 0.05
	endTime := duration * 0.95
	if endTime <= startTime {
		startTime = 0
		endTime = duration
	}

	interval := (endTime - startTime) / float64(numFrames+1)
	var hashes []uint64

	for i := 0; i < numFrames; i++ {
		if ctx.Err() != nil {
			return hashes
		}

		ts := startTime + float64(i+1)*interval

		cmd := exec.CommandContext(ctx, bin, "-y",
			"-ss", fmt.Sprintf("%.3f", ts),
			"-i", shared.PathForBin(bin, path),
			"-vframes", "1",
			"-f", "image2pipe",
			"-vcodec", "mjpeg",
			"-q:v", "2",
			"pipe:1",
		)

		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = nil

		if err := cmd.Run(); err != nil || stdout.Len() == 0 {
			continue
		}

		img, _, err := image.Decode(bytes.NewReader(stdout.Bytes()))
		if err != nil {
			continue
		}

		ph, err := goimagehash.PerceptionHash(img)
		if err != nil {
			continue
		}
		hashes = append(hashes, ph.GetHash())
	}

	return hashes
}

func parseFPS(s string) float64 {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		parts := strings.Split(s, "/")
		if len(parts) == 2 {
			num, err1 := strconv.ParseFloat(parts[0], 64)
			den, err2 := strconv.ParseFloat(parts[1], 64)
			if err1 == nil && err2 == nil && den != 0 {
				return num / den
			}
		}
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// ProcessVideos hashes all videos concurrently and returns successful results.
func ProcessVideos(ctx context.Context, pool *pgxpool.Pool, files []shared.FileInfo, numFrames, numWorkers int, progressCb func(processed, total int)) map[string]*VideoInfo {
	result := make(map[string]*VideoInfo)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, numWorkers)

	total := len(files)
	var processed int64

	for _, f := range files {
		select {
		case <-ctx.Done():
			wg.Wait()
			return result
		default:
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(fi shared.FileInfo) {
			defer wg.Done()
			defer func() { <-sem }()

			info := ComputeVideoHashes(ctx, pool, fi.Path, numFrames)
			if info.HasHash {
				mu.Lock()
				result[fi.Path] = &info
				mu.Unlock()
			}

			count := atomic.AddInt64(&processed, 1)
			if progressCb != nil {
				progressCb(int(count), total)
			}
		}(f)
	}
	wg.Wait()
	return result
}

// computeVideoSimilarity compares frame hashes between two videos.
func computeVideoSimilarity(a, b *VideoInfo) float64 {
	if len(a.FrameHashes) == 0 || len(b.FrameHashes) == 0 {
		return 0
	}

	minFrames := len(a.FrameHashes)
	if len(b.FrameHashes) < minFrames {
		minFrames = len(b.FrameHashes)
	}
	if minFrames == 0 {
		return 0
	}

	var totalSim float64
	for i := 0; i < minFrames; i++ {
		dist := bits.OnesCount64(a.FrameHashes[i] ^ b.FrameHashes[i])
		sim := 1.0 - float64(dist)/64.0
		totalSim += sim
	}

	return totalSim / float64(minFrames)
}

// FindVideoDuplicates finds groups of duplicate videos.
func FindVideoDuplicates(ctx context.Context, infos map[string]*VideoInfo, threshold float64, progressCb func(float64)) [][]DuplicateEntry {
	var paths []string
	for path := range infos {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var groups [][]DuplicateEntry
	assigned := make(map[string]bool)
	total := len(paths)
	step := total / 50
	if step < 1 {
		step = 1
	}

	for i := 0; i < total; i++ {
		if i%step == 0 {
			if ctx.Err() != nil {
				return groups
			}
			if progressCb != nil && total > 0 {
				progressCb(float64(i) / float64(total))
			}
		}

		if assigned[paths[i]] {
			continue
		}

		group := []DuplicateEntry{{Path: paths[i], Similarity: 1.0}}

		for j := i + 1; j < total; j++ {
			if assigned[paths[j]] {
				continue
			}

			sim := computeVideoSimilarity(infos[paths[i]], infos[paths[j]])
			if sim >= threshold {
				group = append(group, DuplicateEntry{Path: paths[j], Similarity: sim})
				assigned[paths[j]] = true
			}
		}

		if len(group) > 1 {
			assigned[paths[i]] = true
			groups = append(groups, group)
		}
	}

	if progressCb != nil {
		progressCb(1.0)
	}

	return groups
}
