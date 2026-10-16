package resizer

import (
	"context"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andreassag/morphic/internal/shared"
	"github.com/disintegration/imaging"
)

// ResizeTarget defines dimensions and fitting mode for a resized variant.
type ResizeTarget struct {
	Name   string `json:"name"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Fit    bool   `json:"fit"`
}

// BatchResizeRequest specifies source, destination, and selected presets.
type BatchResizeRequest struct {
	SourceDir      string   `json:"source_dir"`
	DestinationDir string   `json:"destination_dir"`
	Presets        []string `json:"presets"`       // "web-optimized", "social-media", "thumbnails"
	CustomWidths   []int    `json:"custom_widths"` // e.g. [800, 1024]
	Quality        int      `json:"quality"`       // 1 - 100
}

// ResizerJob tracks the background batch resizing job.
type ResizerJob struct {
	shared.Job
	SourceDir      string   `json:"source_dir"`
	DestinationDir string   `json:"destination_dir"`
	TotalImages    int      `json:"total_images"`
	ProcessedCount int      `json:"processed_count"`
	GeneratedFiles []string `json:"generated_files"`
}

var jobs = shared.NewJobStore[ResizerJob]()

// GetJob retrieves a resizer job by ID.
func GetJob(id string) (*ResizerJob, bool) {
	return jobs.Get(id)
}

// SetJob stores a resizer job by ID.
func SetJob(id string, job *ResizerJob) {
	jobs.Set(id, job)
}

// RegisterCancel registers a cancel func.
func RegisterCancel(id string, cancel context.CancelFunc) {
	jobs.RegisterCancel(id, cancel)
}

// CancelJob cancels a resizer job.
func CancelJob(id string) bool {
	return jobs.Cancel(id)
}

// StartCleanup periodically cleans up expired jobs.
func StartCleanup(ctx context.Context, ttl time.Duration) {
	jobs.StartCleanup(ctx, ttl, func(j *ResizerJob) time.Time {
		return j.DoneAt
	})
}

// ResolveTargets resolves list of ResizeTargets from presets and custom widths.
func ResolveTargets(presets []string, customWidths []int) []ResizeTarget {
	var targets []ResizeTarget
	seen := make(map[string]bool)

	addTarget := func(t ResizeTarget) {
		if !seen[t.Name] {
			seen[t.Name] = true
			targets = append(targets, t)
		}
	}

	for _, p := range presets {
		switch strings.ToLower(p) {
		case "web-optimized", "web":
			addTarget(ResizeTarget{Name: "web_1920", Width: 1920, Height: 0})
			addTarget(ResizeTarget{Name: "web_1280", Width: 1280, Height: 0})
		case "social-media", "social":
			addTarget(ResizeTarget{Name: "social_square_1080", Width: 1080, Height: 1080, Fit: true})
			addTarget(ResizeTarget{Name: "social_story_1080x1920", Width: 1080, Height: 1920, Fit: true})
			addTarget(ResizeTarget{Name: "social_feed_1200x630", Width: 1200, Height: 630, Fit: true})
		case "thumbnails", "thumbs":
			addTarget(ResizeTarget{Name: "thumb_600", Width: 600, Height: 0})
			addTarget(ResizeTarget{Name: "thumb_300", Width: 300, Height: 0})
			addTarget(ResizeTarget{Name: "thumb_150", Width: 150, Height: 0})
		}
	}

	for _, w := range customWidths {
		if w > 0 {
			name := fmt.Sprintf("custom_%d", w)
			addTarget(ResizeTarget{Name: name, Width: w, Height: 0})
		}
	}

	if len(targets) == 0 {
		// Default to standard web-optimized
		targets = append(targets,
			ResizeTarget{Name: "web_1920", Width: 1920, Height: 0},
			ResizeTarget{Name: "web_1280", Width: 1280, Height: 0},
		)
	}

	return targets
}

// BatchResize processes images in SourceDir and exports resized variants to DestinationDir.
func BatchResize(ctx context.Context, req BatchResizeRequest, onProgress func(done, total int)) ([]string, error) {
	safeSrc, err := shared.ValidateSafeDirPath(req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("validate source directory: %w", err)
	}

	safeDest, err := shared.ValidateSafeDirPath(req.DestinationDir)
	if err != nil {
		return nil, fmt.Errorf("validate destination directory: %w", err)
	}

	targets := ResolveTargets(req.Presets, req.CustomWidths)

	var imageFiles []string
	err = filepath.WalkDir(safeSrc, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if !d.IsDir() && shared.IsImage(path) {
			imageFiles = append(imageFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking images in %s: %w", safeSrc, err)
	}

	total := len(imageFiles)
	if total == 0 {
		return nil, nil
	}

	quality := req.Quality
	if quality <= 0 || quality > 100 {
		quality = 85
	}

	numWorkers := runtime.NumCPU()
	if numWorkers > 8 {
		numWorkers = 8
	}
	if numWorkers > total {
		numWorkers = total
	}

	type task struct {
		filePath string
		relPath  string
	}

	tasksCh := make(chan task, total)
	for _, f := range imageFiles {
		rel, rErr := filepath.Rel(safeSrc, f)
		if rErr != nil {
			rel = filepath.Base(f)
		}
		tasksCh <- task{filePath: f, relPath: rel}
	}
	close(tasksCh)

	var wg sync.WaitGroup
	var completed int64
	var generatedMu sync.Mutex
	var generatedFiles []string

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range tasksCh {
				select {
				case <-ctx.Done():
					return
				default:
				}

				srcImg, oErr := imaging.Open(t.filePath, imaging.AutoOrientation(true))
				if oErr != nil {
					continue
				}

				for _, target := range targets {
					select {
					case <-ctx.Done():
						return
					default:
					}

					var resized image.Image
					if target.Fit && target.Height > 0 {
						resized = imaging.Fit(srcImg, target.Width, target.Height, imaging.Lanczos)
					} else {
						resized = imaging.Resize(srcImg, target.Width, target.Height, imaging.Lanczos)
					}

					outPath := filepath.Join(safeDest, target.Name, t.relPath)
					if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
						continue
					}

					if err := imaging.Save(resized, outPath, imaging.JPEGQuality(quality)); err == nil {
						generatedMu.Lock()
						generatedFiles = append(generatedFiles, outPath)
						generatedMu.Unlock()
					}
				}

				done := atomic.AddInt64(&completed, 1)
				if onProgress != nil {
					onProgress(int(done), total)
				}
			}
		}()
	}

	wg.Wait()

	if ctx.Err() != nil {
		return generatedFiles, ctx.Err()
	}

	return generatedFiles, nil
}
