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
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/corona10/goimagehash"
	"github.com/exterex/morphic/internal/shared"
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

// ComputeVideoHashes extracts frames and computes perceptual hashes.
func ComputeVideoHashes(ctx context.Context, path string, numFrames int) VideoInfo {
	info := VideoInfo{Path: path}

	if ctx.Err() != nil {
		return info
	}

	st, err := os.Stat(path)
	if err != nil {
		return info
	}
	info.FileSize = st.Size()

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
// It stops accepting new work when ctx is cancelled.
func ProcessVideos(ctx context.Context, files []shared.FileInfo, numFrames, numWorkers int, progressCb func(processed, total int)) map[string]*VideoInfo {
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

			info := ComputeVideoHashes(ctx, fi.Path, numFrames)
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

// ComputeVideoSimilarity computes similarity between two videos using
// frame-level hash comparison.
func ComputeVideoSimilarity(a, b *VideoInfo) float64 {
	if len(a.FrameHashes) == 0 || len(b.FrameHashes) == 0 {
		return 0
	}

	var total float64
	for _, h1 := range a.FrameHashes {
		bestSim := 0.0
		for _, h2 := range b.FrameHashes {
			dist := bits.OnesCount64(h1 ^ h2)
			sim := 1.0 - float64(dist)/64.0
			if sim > bestSim {
				bestSim = sim
			}
		}
		total += bestSim
	}

	return total / float64(len(a.FrameHashes))
}

// FindVideoDuplicates finds groups of duplicate videos with batching and cancellation support.
func FindVideoDuplicates(ctx context.Context, infos map[string]*VideoInfo, threshold float64, progressCb func(float64)) [][]DuplicateEntry {
	paths := make([]string, 0, len(infos))
	for p := range infos {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	assigned := make(map[string]bool)
	var groups [][]DuplicateEntry

	totalPaths := len(paths)
	for i := 0; i < totalPaths; i++ {
		if i%10 == 0 {
			if ctx.Err() != nil {
				return groups
			}
			if progressCb != nil && totalPaths > 0 {
				progressCb(float64(i) / float64(totalPaths))
			}
		}

		if assigned[paths[i]] {
			continue
		}
		group := []DuplicateEntry{{Path: paths[i], Similarity: 1.0}}

		for j := i + 1; j < totalPaths; j++ {
			if assigned[paths[j]] {
				continue
			}
			sim := ComputeVideoSimilarity(infos[paths[i]], infos[paths[j]])
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
