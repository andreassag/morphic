package dupfinder

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/exterex/morphic/internal/shared"
)

// MediaEntry describes one file entry within a duplicate group.
type MediaEntry struct {
	Path              string  `json:"path"`
	Filename          string  `json:"filename"`
	Directory         string  `json:"directory"`
	Width             int     `json:"width"`
	Height            int     `json:"height"`
	Resolution        string  `json:"resolution"`
	Format            string  `json:"format,omitempty"`
	Duration          float64 `json:"duration,omitempty"`
	DurationFormatted string  `json:"duration_formatted,omitempty"`
	FPS               float64 `json:"fps,omitempty"`
	FileSize          int64   `json:"file_size"`
	FileSizeFormatted string  `json:"file_size_formatted"`
	Similarity        float64 `json:"similarity"`
	Type              string  `json:"type"`
}

// ScanJob represents a running or completed dupfinder job.
type ScanJob struct {
	shared.Job
	mu sync.Mutex

	Folder         string         `json:"folder"`
	ScanType       string         `json:"scan_type"` // "images", "videos", "both"
	ImageThreshold float64        `json:"image_threshold"`
	VideoThreshold float64        `json:"video_threshold"`
	ImageGroups    [][]MediaEntry `json:"image_groups,omitempty"`
	VideoGroups    [][]MediaEntry `json:"video_groups,omitempty"`
	TotalFound     int            `json:"total_files_found"`
	TotalProcessed int            `json:"total_files_processed"`
	SpaceSavings   int64          `json:"space_savings"`
}

var store = shared.NewJobStore[ScanJob]()

// StartCleanup starts background cleanup of expired jobs.
func StartCleanup(ctx context.Context, ttl time.Duration) {
	store.StartCleanup(ctx, ttl, func(j *ScanJob) time.Time {
		return j.DoneAt
	})
}

// StartJob creates and launches a new dupfinder job.
func StartJob(parentCtx context.Context, folder, scanType string, imageThreshold, videoThreshold float64) string {
	job := &ScanJob{
		Job:            shared.NewJob(),
		Folder:         folder,
		ScanType:       scanType,
		ImageThreshold: imageThreshold,
		VideoThreshold: videoThreshold,
	}
	job.Status = shared.JobStatusRunning
	store.Set(job.ID, job)

	ctx, cancel := context.WithCancel(parentCtx)
	store.RegisterCancel(job.ID, cancel)

	go runScan(ctx, job)
	return job.ID
}

// GetJob retrieves a job by ID.
func GetJob(id string) (*ScanJob, bool) {
	return store.Get(id)
}

// CancelJob cancels a running job by ID.
func CancelJob(id string) bool {
	return store.Cancel(id)
}

func runScan(ctx context.Context, job *ScanJob) {
	defer func() {
		if r := recover(); r != nil {
			job.mu.Lock()
			job.Status = shared.JobStatusFailed
			job.Error = fmt.Sprintf("%v", r)
			job.DoneAt = time.Now()
			job.mu.Unlock()
		}
	}()

	job.mu.Lock()
	job.Message = fmt.Sprintf("Scanning folder: %s", job.Folder)
	job.mu.Unlock()

	// Image scan
	if job.ScanType == "images" || job.ScanType == "both" {
		scanImages(ctx, job)
	}

	// Check for cancellation between phases
	select {
	case <-ctx.Done():
		job.mu.Lock()
		job.Status = shared.JobStatusCancelled
		job.DoneAt = time.Now()
		job.Message = "Scan was interrupted"
		job.mu.Unlock()
		return
	default:
	}

	// Video scan
	if job.ScanType == "videos" || job.ScanType == "both" {
		scanVideos(ctx, job)
	}

	// Check for cancellation before finalising
	select {
	case <-ctx.Done():
		job.mu.Lock()
		job.Status = shared.JobStatusCancelled
		job.DoneAt = time.Now()
		job.Message = "Scan was interrupted"
		job.mu.Unlock()
		return
	default:
	}

	// Finalise
	job.mu.Lock()
	job.SpaceSavings = calculateSpaceSavings(job)
	job.Status = shared.JobStatusDone
	job.Progress = 1.0
	job.DoneAt = time.Now()
	elapsed := job.DoneAt.Sub(job.StartedAt).Seconds()
	totalGroups := len(job.ImageGroups) + len(job.VideoGroups)
	job.Message = fmt.Sprintf("Done! Found %d duplicate group(s) in %.1fs. Potential savings: %s",
		totalGroups, elapsed, shared.FormatFileSize(job.SpaceSavings))
	job.mu.Unlock()
}

func scanImages(ctx context.Context, job *ScanJob) {
	job.mu.Lock()
	job.Message = "Finding image files..."
	job.mu.Unlock()

	files, err := shared.FindImageFiles(job.Folder)
	if err != nil {
		slog.Error("dupfinder: finding image files failed", "folder", job.Folder, "err", err)
		job.mu.Lock()
		job.Error = err.Error()
		job.mu.Unlock()
		return
	}

	job.mu.Lock()
	job.TotalFound += len(files)
	job.Message = fmt.Sprintf("Found %d images. Processing hashes...", len(files))
	job.Progress = 0.1
	job.mu.Unlock()

	if len(files) == 0 {
		return
	}

	infos := ProcessImages(ctx, files, shared.DefaultNumWorkers, func(processed, total int) {
		job.mu.Lock()
		job.TotalProcessed = processed
		pctBase := 0.1
		pctSpan := 0.3
		if job.ScanType != "both" {
			pctSpan = 0.6
		}
		if total > 0 {
			job.Progress = pctBase + (float64(processed)/float64(total))*pctSpan
		}
		job.mu.Unlock()
	})

	if ctx.Err() != nil {
		return
	}

	job.mu.Lock()
	job.Progress = 0.4
	job.Message = fmt.Sprintf("Processed %d images. Finding duplicates...", len(infos))
	job.mu.Unlock()

	groups := FindImageDuplicates(ctx, infos, job.ImageThreshold, func(progress float64) {
		job.mu.Lock()
		if job.ScanType == "both" {
			job.Progress = 0.4 + (progress * 0.1)
		} else {
			job.Progress = 0.7 + (progress * 0.25)
		}
		job.mu.Unlock()
	})

	formatted := formatImageGroups(groups, infos)

	job.mu.Lock()
	job.ImageGroups = formatted
	if job.ScanType == "both" {
		job.Progress = 0.5
	} else {
		job.Progress = 0.95
	}
	job.mu.Unlock()
}

func scanVideos(ctx context.Context, job *ScanJob) {
	job.mu.Lock()
	job.Message = "Finding video files..."
	job.mu.Unlock()

	files, err := shared.FindVideoFiles(job.Folder)
	if err != nil {
		slog.Error("dupfinder: finding video files failed", "folder", job.Folder, "err", err)
		job.mu.Lock()
		job.Error = err.Error()
		job.mu.Unlock()
		return
	}

	job.mu.Lock()
	job.TotalFound += len(files)
	job.Message = fmt.Sprintf("Found %d videos. Processing hashes...", len(files))
	if job.ScanType == "both" {
		job.Progress = 0.55
	} else {
		job.Progress = 0.1
	}
	job.mu.Unlock()

	if len(files) == 0 {
		return
	}

	infos := ProcessVideos(ctx, files, shared.DefaultNumFrames, shared.DefaultNumWorkers, func(processed, total int) {
		job.mu.Lock()
		job.TotalProcessed = processed
		pctBase := 0.55
		pctSpan := 0.25
		if job.ScanType != "both" {
			pctBase = 0.1
			pctSpan = 0.6
		}
		if total > 0 {
			job.Progress = pctBase + (float64(processed)/float64(total))*pctSpan
		}
		job.mu.Unlock()
	})

	if ctx.Err() != nil {
		return
	}

	job.mu.Lock()
	job.Progress = 0.8
	job.Message = fmt.Sprintf("Processed %d videos. Finding duplicates...", len(infos))
	job.mu.Unlock()

	groups := FindVideoDuplicates(ctx, infos, job.VideoThreshold, func(progress float64) {
		job.mu.Lock()
		if job.ScanType == "both" {
			job.Progress = 0.8 + (progress * 0.15)
		} else {
			job.Progress = 0.7 + (progress * 0.25)
		}
		job.mu.Unlock()
	})

	formatted := formatVideoGroups(groups, infos)

	job.mu.Lock()
	job.VideoGroups = formatted
	job.Progress = 0.95
	job.mu.Unlock()
}

func formatImageGroups(groups [][]DuplicateEntry, infos map[string]*ImageInfo) [][]MediaEntry {
	var result [][]MediaEntry
	for _, group := range groups {
		// Sort by file size descending
		sort.Slice(group, func(i, j int) bool {
			ai := infos[group[i].Path]
			aj := infos[group[j].Path]
			if ai == nil || aj == nil {
				return false
			}
			return ai.FileSize > aj.FileSize
		})

		var formatted []MediaEntry
		for _, entry := range group {
			info := infos[entry.Path]
			if info == nil {
				continue
			}
			formatted = append(formatted, MediaEntry{
				Path:              entry.Path,
				Filename:          filepath.Base(entry.Path),
				Directory:         filepath.Dir(entry.Path),
				Width:             info.Width,
				Height:            info.Height,
				Resolution:        fmt.Sprintf("%dx%d", info.Width, info.Height),
				Format:            info.Format,
				FileSize:          info.FileSize,
				FileSizeFormatted: shared.FormatFileSize(info.FileSize),
				Similarity:        float64(int(entry.Similarity*1000)) / 10,
				Type:              "image",
			})
		}
		if len(formatted) > 1 {
			result = append(result, formatted)
		}
	}
	return result
}

func formatVideoGroups(groups [][]DuplicateEntry, infos map[string]*VideoInfo) [][]MediaEntry {
	var result [][]MediaEntry
	for _, group := range groups {
		sort.Slice(group, func(i, j int) bool {
			ai := infos[group[i].Path]
			aj := infos[group[j].Path]
			if ai == nil || aj == nil {
				return false
			}
			return ai.FileSize > aj.FileSize
		})

		var formatted []MediaEntry
		for _, entry := range group {
			info := infos[entry.Path]
			if info == nil {
				continue
			}
			formatted = append(formatted, MediaEntry{
				Path:              entry.Path,
				Filename:          filepath.Base(entry.Path),
				Directory:         filepath.Dir(entry.Path),
				Width:             info.Width,
				Height:            info.Height,
				Resolution:        fmt.Sprintf("%dx%d", info.Width, info.Height),
				Duration:          info.Duration,
				DurationFormatted: shared.FormatDuration(info.Duration),
				FPS:               float64(int(info.FPS*10)) / 10,
				FileSize:          info.FileSize,
				FileSizeFormatted: shared.FormatFileSize(info.FileSize),
				Similarity:        float64(int(entry.Similarity*1000)) / 10,
				Type:              "video",
			})
		}
		if len(formatted) > 1 {
			result = append(result, formatted)
		}
	}
	return result
}

func calculateSpaceSavings(job *ScanJob) int64 {
	var total int64
	for _, group := range job.ImageGroups {
		total += groupSavings(group)
	}
	for _, group := range job.VideoGroups {
		total += groupSavings(group)
	}
	return total
}

func groupSavings(group []MediaEntry) int64 {
	if len(group) <= 1 {
		return 0
	}
	var sizes []int64
	for _, item := range group {
		sizes = append(sizes, item.FileSize)
	}
	sort.Slice(sizes, func(i, j int) bool { return sizes[i] < sizes[j] })
	var total int64
	for _, s := range sizes[:len(sizes)-1] {
		total += s
	}
	return total
}
