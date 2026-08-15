package dupfinder

import (
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/andreassag/morphic/internal/shared"
)

// ScanJob represents a running or completed dupfinder job.
type ScanJob struct {
	shared.Job
	mu sync.Mutex

	Folder         string                     `json:"folder"`
	ScanType       string                     `json:"scan_type"` // "images", "videos", "both"
	ImageThreshold float64                    `json:"image_threshold"`
	VideoThreshold float64                    `json:"video_threshold"`
	ImageGroups    [][]map[string]interface{} `json:"image_groups,omitempty"`
	VideoGroups    [][]map[string]interface{} `json:"video_groups,omitempty"`
	TotalFound     int                        `json:"total_files_found"`
	TotalProcessed int                        `json:"total_files_processed"`
	SpaceSavings   int64                      `json:"space_savings"`
}

var store = shared.NewJobStore[ScanJob]()

func init() {
	store.StartCleanup(30*time.Minute, func(j *ScanJob) time.Time {
		return j.DoneAt
	})
}

// StartJob creates and launches a new dupfinder job.
func StartJob(folder, scanType string, imageThreshold, videoThreshold float64) string {
	job := &ScanJob{
		Job:            shared.NewJob(),
		Folder:         folder,
		ScanType:       scanType,
		ImageThreshold: imageThreshold,
		VideoThreshold: videoThreshold,
	}
	job.Status = shared.JobStatusRunning
	store.Set(job.ID, job)
	go runScan(job)
	return job.ID
}

// GetJob retrieves a job by ID.
func GetJob(id string) (*ScanJob, bool) {
	return store.Get(id)
}

func runScan(job *ScanJob) {
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
		scanImages(job)
	}

	// Check for cancellation between phases
	select {
	case <-job.Ctx().Done():
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
		scanVideos(job)
	}

	// Check for cancellation before finalising
	select {
	case <-job.Ctx().Done():
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

func scanImages(job *ScanJob) {
	job.mu.Lock()
	job.Message = "Finding image files..."
	job.mu.Unlock()

	files, err := shared.FindImageFiles(job.Folder)
	if err != nil {
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

	infos := ProcessImages(job.Ctx(), files, shared.DefaultNumWorkers)

	// Return early if cancelled during hash processing
	select {
	case <-job.Ctx().Done():
		return
	default:
	}

	job.mu.Lock()
	job.TotalProcessed += len(infos)
	job.Progress = 0.4
	job.Message = fmt.Sprintf("Processed %d images. Finding duplicates...", len(infos))
	job.mu.Unlock()

	groups := FindImageDuplicates(infos, job.ImageThreshold)
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

func scanVideos(job *ScanJob) {
	job.mu.Lock()
	job.Message = "Finding video files..."
	job.mu.Unlock()

	files, err := shared.FindVideoFiles(job.Folder)
	if err != nil {
		return
	}

	job.mu.Lock()
	job.TotalFound += len(files)
	job.Message = fmt.Sprintf("Found %d videos. Processing hashes...", len(files))
	if job.ScanType == "both" {
		job.Progress = 0.6
	} else {
		job.Progress = 0.1
	}
	job.mu.Unlock()

	if len(files) == 0 {
		return
	}

	infos := ProcessVideos(job.Ctx(), files, shared.DefaultNumFrames, shared.DefaultNumWorkers)

	// Return early if cancelled during hash processing
	select {
	case <-job.Ctx().Done():
		return
	default:
	}

	job.mu.Lock()
	job.TotalProcessed += len(infos)
	if job.ScanType == "both" {
		job.Progress = 0.8
	} else {
		job.Progress = 0.7
	}
	job.Message = fmt.Sprintf("Processed %d videos. Finding duplicates...", len(infos))
	job.mu.Unlock()

	groups := FindVideoDuplicates(infos, job.VideoThreshold)
	formatted := formatVideoGroups(groups, infos)

	job.mu.Lock()
	job.VideoGroups = formatted
	job.Progress = 0.95
	job.mu.Unlock()
}

func formatImageGroups(groups [][]DuplicateEntry, infos map[string]*ImageInfo) [][]map[string]interface{} {
	var result [][]map[string]interface{}
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

		var formatted []map[string]interface{}
		for _, entry := range group {
			info := infos[entry.Path]
			if info == nil {
				continue
			}
			formatted = append(formatted, map[string]interface{}{
				"path":                entry.Path,
				"filename":            filepath.Base(entry.Path),
				"directory":           filepath.Dir(entry.Path),
				"width":               info.Width,
				"height":              info.Height,
				"resolution":          fmt.Sprintf("%dx%d", info.Width, info.Height),
				"format":              info.Format,
				"file_size":           info.FileSize,
				"file_size_formatted": shared.FormatFileSize(info.FileSize),
				"similarity":          float64(int(entry.Similarity*1000)) / 10,
				"type":                "image",
			})
		}
		if len(formatted) > 1 {
			result = append(result, formatted)
		}
	}
	return result
}

func formatVideoGroups(groups [][]DuplicateEntry, infos map[string]*VideoInfo) [][]map[string]interface{} {
	var result [][]map[string]interface{}
	for _, group := range groups {
		sort.Slice(group, func(i, j int) bool {
			ai := infos[group[i].Path]
			aj := infos[group[j].Path]
			if ai == nil || aj == nil {
				return false
			}
			return ai.FileSize > aj.FileSize
		})

		var formatted []map[string]interface{}
		for _, entry := range group {
			info := infos[entry.Path]
			if info == nil {
				continue
			}
			formatted = append(formatted, map[string]interface{}{
				"path":                entry.Path,
				"filename":            filepath.Base(entry.Path),
				"directory":           filepath.Dir(entry.Path),
				"width":               info.Width,
				"height":              info.Height,
				"resolution":          fmt.Sprintf("%dx%d", info.Width, info.Height),
				"duration":            info.Duration,
				"duration_formatted":  shared.FormatDuration(info.Duration),
				"fps":                 float64(int(info.FPS*10)) / 10,
				"file_size":           info.FileSize,
				"file_size_formatted": shared.FormatFileSize(info.FileSize),
				"similarity":          float64(int(entry.Similarity*1000)) / 10,
				"type":                "video",
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
	allGroups := append(job.ImageGroups, job.VideoGroups...)
	for _, group := range allGroups {
		var sizes []int64
		for _, item := range group {
			if s, ok := item["file_size"].(int64); ok {
				sizes = append(sizes, s)
			}
		}
		if len(sizes) > 1 {
			sort.Slice(sizes, func(i, j int) bool { return sizes[i] < sizes[j] })
			for _, s := range sizes[:len(sizes)-1] {
				total += s
			}
		}
	}
	return total
}
