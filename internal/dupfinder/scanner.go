package dupfinder

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/exterex/morphic/internal/database"
	"github.com/exterex/morphic/internal/events"
	"github.com/exterex/morphic/internal/shared"
	"github.com/jackc/pgx/v5/pgxpool"
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
// DuplicateGroup represents a cluster of duplicate media files with group-level similarity.
type DuplicateGroup struct {
	Items           []MediaEntry `json:"items"`
	GroupSimilarity float64      `json:"group_similarity"`
}

type ScanJob struct {
	shared.Job
	mu sync.Mutex

	Folder         string           `json:"folder"`
	ScanType       string           `json:"scan_type"` // "images", "videos", "both"
	ImageThreshold float64          `json:"image_threshold"`
	VideoThreshold float64          `json:"video_threshold"`
	ImageGroups    []DuplicateGroup `json:"image_groups,omitempty"`
	VideoGroups    []DuplicateGroup `json:"video_groups,omitempty"`
	TotalFound     int              `json:"total_files_found"`
	TotalProcessed int              `json:"total_files_processed"`
	SpaceSavings   int64            `json:"space_savings"`
	Pool           *pgxpool.Pool    `json:"-"`
}

var (
	store  = shared.NewJobStore[ScanJob]()
	dbPool *pgxpool.Pool
)

// SetDBPool sets the database pool for dupfinder.
func SetDBPool(pool *pgxpool.Pool) {
	dbPool = pool
}

// StartCleanup starts background cleanup of expired jobs.
func StartCleanup(ctx context.Context, ttl time.Duration) {
	store.StartCleanup(ctx, ttl, func(j *ScanJob) time.Time {
		return j.DoneAt
	})
}

// StartJob creates and launches a new dupfinder job.
func StartJob(parentCtx context.Context, pool *pgxpool.Pool, folder, scanType string, imageThreshold, videoThreshold float64) string {
	if pool == nil {
		pool = dbPool
	}

	job := &ScanJob{
		Job:            shared.NewJob(),
		Folder:         folder,
		ScanType:       scanType,
		ImageThreshold: imageThreshold,
		VideoThreshold: videoThreshold,
		Pool:           pool,
	}
	job.Status = shared.JobStatusRunning
	store.Set(job.ID, job)

	if pool != nil {
		_ = database.CreateJob(parentCtx, pool, database.JobRecord{
			ID:        job.ID,
			Type:      "dupfinder",
			Status:    string(job.Status),
			Progress:  job.Progress,
			Message:   "Starting scan...",
			StartedAt: job.StartedAt,
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
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

			if job.Pool != nil {
				_ = database.UpdateJobStatus(context.Background(), job.Pool, job.ID, "failed", job.Progress, "Scan failed", job.Error, nil)
			}
			events.DefaultBus.Publish("dupfinder", "job_failed", ginMap(job))
		}
	}()

	// Background ticker emits heartbeat progress events every 1 second during long operations
	heartbeatDone := make(chan struct{})
	defer close(heartbeatDone)
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatDone:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				job.mu.Lock()
				status := job.Status
				job.mu.Unlock()
				if status == shared.JobStatusRunning {
					publishProgress(job)
				} else {
					return
				}
			}
		}
	}()

	// 1. Unified Discovery Phase (0% -> 5%)
	job.mu.Lock()
	job.Message = "Discovering media files..."
	job.Progress = 0.02
	job.mu.Unlock()
	publishProgress(job)

	var imageFiles, videoFiles []shared.FileInfo
	var err error

	if job.ScanType == "images" || job.ScanType == "both" {
		imageFiles, err = shared.FindImageFiles(job.Folder)
		if err != nil {
			slog.Error("dupfinder: finding image files failed", "folder", job.Folder, "err", err)
			job.mu.Lock()
			job.Error = err.Error()
			job.mu.Unlock()
		}
	}

	if job.ScanType == "videos" || job.ScanType == "both" {
		videoFiles, err = shared.FindVideoFiles(job.Folder)
		if err != nil {
			slog.Error("dupfinder: finding video files failed", "folder", job.Folder, "err", err)
			job.mu.Lock()
			job.Error = err.Error()
			job.mu.Unlock()
		}
	}

	totalImages := len(imageFiles)
	totalVideos := len(videoFiles)
	totalFiles := totalImages + totalVideos

	job.mu.Lock()
	job.TotalFound = totalFiles
	job.TotalProcessed = 0
	if totalFiles == 0 {
		job.Message = "No media files found in folder"
		job.Progress = 1.0
	} else {
		job.Message = fmt.Sprintf("Found %d file(s) (%d images, %d videos). Processing hashes...", totalFiles, totalImages, totalVideos)
		job.Progress = 0.05
	}
	job.mu.Unlock()
	publishProgress(job)

	if totalFiles == 0 {
		job.mu.Lock()
		job.Status = shared.JobStatusDone
		job.DoneAt = time.Now()
		job.Message = "Done! Found 0 files in folder."
		job.mu.Unlock()

		if job.Pool != nil {
			_ = database.UpdateJobStatus(context.Background(), job.Pool, job.ID, "done", 1.0, job.Message, "", nil)
		}
		events.DefaultBus.Publish("dupfinder", "job_done", ginMap(job))
		return
	}

	// 2. Dynamic Hashing Phase (5% -> 75%)
	// Video frame extraction takes ~5x the work of image hashing
	const imageWeight = 1.0
	const videoWeight = 5.0
	totalHashingUnits := float64(totalImages)*imageWeight + float64(totalVideos)*videoWeight

	const hashBase = 0.05
	const hashSpan = 0.70

	var processedUnits float64
	var processedCount int
	var progressMu sync.Mutex

	var imgInfos map[string]*ImageInfo
	if totalImages > 0 {
		imgInfos = ProcessImages(ctx, job.Pool, imageFiles, shared.DefaultNumWorkers, func(p, total int) {
			progressMu.Lock()
			processedUnits += imageWeight
			processedCount++
			currentUnits := processedUnits
			currentProcessed := processedCount
			progressMu.Unlock()

			job.mu.Lock()
			job.TotalProcessed = currentProcessed
			job.Progress = hashBase + (currentUnits/totalHashingUnits)*hashSpan
			job.Message = fmt.Sprintf("Hashing images: %d / %d processed", p, total)
			job.mu.Unlock()
			publishProgress(job)
		})
	}

	select {
	case <-ctx.Done():
		markCancelled(job)
		return
	default:
	}

	var vidInfos map[string]*VideoInfo
	if totalVideos > 0 {
		vidInfos = ProcessVideos(ctx, job.Pool, videoFiles, shared.DefaultNumFrames, shared.DefaultNumWorkers, func(p, total int) {
			progressMu.Lock()
			processedUnits += videoWeight
			processedCount++
			currentUnits := processedUnits
			currentProcessed := processedCount
			progressMu.Unlock()

			job.mu.Lock()
			job.TotalProcessed = currentProcessed
			job.Progress = hashBase + (currentUnits/totalHashingUnits)*hashSpan
			job.Message = fmt.Sprintf("Hashing videos: %d / %d processed", p, total)
			job.mu.Unlock()
			publishProgress(job)
		})
	}

	select {
	case <-ctx.Done():
		markCancelled(job)
		return
	default:
	}

	// 3. Duplicate Detection & Comparison Phase (75% -> 95%)
	const compareBase = 0.75
	const compareSpan = 0.20

	var imgCompareSpan, vidCompareSpan float64
	if totalImages > 0 && totalVideos > 0 {
		imgCompareSpan = compareSpan * 0.5
		vidCompareSpan = compareSpan * 0.5
	} else if totalImages > 0 {
		imgCompareSpan = compareSpan
	} else {
		vidCompareSpan = compareSpan
	}

	if totalImages > 0 && len(imgInfos) > 0 {
		job.mu.Lock()
		job.Progress = compareBase
		job.Message = fmt.Sprintf("Comparing %d images for duplicates...", len(imgInfos))
		job.mu.Unlock()
		publishProgress(job)

		groups := FindImageDuplicates(ctx, imgInfos, job.ImageThreshold, func(p float64) {
			job.mu.Lock()
			job.Progress = compareBase + p*imgCompareSpan
			job.Message = fmt.Sprintf("Comparing images: %d%%", int(p*100))
			job.mu.Unlock()
			publishProgress(job)
		})

		job.mu.Lock()
		job.ImageGroups = formatImageGroups(groups, imgInfos)
		job.Progress = compareBase + imgCompareSpan
		job.mu.Unlock()
		publishProgress(job)
	}

	select {
	case <-ctx.Done():
		markCancelled(job)
		return
	default:
	}

	if totalVideos > 0 && len(vidInfos) > 0 {
		vidBase := compareBase + imgCompareSpan
		job.mu.Lock()
		job.Progress = vidBase
		job.Message = fmt.Sprintf("Comparing %d videos for duplicates...", len(vidInfos))
		job.mu.Unlock()
		publishProgress(job)

		groups := FindVideoDuplicates(ctx, vidInfos, job.VideoThreshold, func(p float64) {
			job.mu.Lock()
			job.Progress = vidBase + p*vidCompareSpan
			job.Message = fmt.Sprintf("Comparing videos: %d%%", int(p*100))
			job.mu.Unlock()
			publishProgress(job)
		})

		job.mu.Lock()
		job.VideoGroups = formatVideoGroups(groups, vidInfos)
		job.Progress = compareBase + compareSpan
		job.mu.Unlock()
		publishProgress(job)
	}

	select {
	case <-ctx.Done():
		markCancelled(job)
		return
	default:
	}

	// 4. Finalise Phase (95% -> 100%)
	job.mu.Lock()
	job.Progress = 0.98
	job.Message = "Calculating space savings..."
	job.mu.Unlock()
	publishProgress(job)

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

	if job.Pool != nil {
		_ = database.UpdateJobStatus(context.Background(), job.Pool, job.ID, "done", 1.0, job.Message, "", map[string]interface{}{
			"image_groups":  job.ImageGroups,
			"video_groups":  job.VideoGroups,
			"space_savings": job.SpaceSavings,
		})
	}
	events.DefaultBus.Publish("dupfinder", "job_done", ginMap(job))
}

func markCancelled(job *ScanJob) {
	job.mu.Lock()
	job.Status = shared.JobStatusCancelled
	job.DoneAt = time.Now()
	job.Message = "Scan was interrupted"
	job.mu.Unlock()

	if job.Pool != nil {
		_ = database.UpdateJobStatus(context.Background(), job.Pool, job.ID, "cancelled", job.Progress, job.Message, "", nil)
	}
	events.DefaultBus.Publish("dupfinder", "job_cancelled", ginMap(job))
}

func publishProgress(job *ScanJob) {
	job.mu.Lock()
	payload := ginMap(job)
	job.mu.Unlock()

	events.DefaultBus.Publish("dupfinder", "progress", payload)
}

func ginMap(job *ScanJob) map[string]interface{} {
	elapsed := 0.0
	if !job.StartedAt.IsZero() {
		end := job.DoneAt
		if end.IsZero() {
			end = time.Now()
		}
		elapsed = end.Sub(job.StartedAt).Seconds()
	}

	return map[string]interface{}{
		"id":                    job.ID,
		"status":                job.Status,
		"progress":              job.Progress,
		"message":               job.Message,
		"error":                 job.Error,
		"total_files_found":     job.TotalFound,
		"total_files_processed": job.TotalProcessed,
		"elapsed_seconds":       float64(int(elapsed*10)) / 10,
		"image_groups":          job.ImageGroups,
		"video_groups":          job.VideoGroups,
		"space_savings":         job.SpaceSavings,
	}
}

func formatImageGroups(groups [][]DuplicateEntry, infos map[string]*ImageInfo) []DuplicateGroup {
	var result []DuplicateGroup
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
		var nonRefSum float64
		var nonRefCount int
		for _, entry := range group {
			info := infos[entry.Path]
			if info == nil {
				continue
			}
			sim := float64(int(entry.Similarity*1000)) / 10
			if entry.Similarity < 0.9999 {
				nonRefSum += sim
				nonRefCount++
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
				Similarity:        sim,
				Type:              "image",
			})
		}
		if len(formatted) > 1 {
			groupSim := 100.0
			if nonRefCount > 0 {
				groupSim = float64(int((nonRefSum/float64(nonRefCount))*10)) / 10
			}
			result = append(result, DuplicateGroup{
				Items:           formatted,
				GroupSimilarity: groupSim,
			})
		}
	}
	return result
}

func formatVideoGroups(groups [][]DuplicateEntry, infos map[string]*VideoInfo) []DuplicateGroup {
	var result []DuplicateGroup
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
		var nonRefSum float64
		var nonRefCount int
		for _, entry := range group {
			info := infos[entry.Path]
			if info == nil {
				continue
			}
			sim := float64(int(entry.Similarity*1000)) / 10
			if entry.Similarity < 0.9999 {
				nonRefSum += sim
				nonRefCount++
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
				Similarity:        sim,
				Type:              "video",
			})
		}
		if len(formatted) > 1 {
			groupSim := 100.0
			if nonRefCount > 0 {
				groupSim = float64(int((nonRefSum/float64(nonRefCount))*10)) / 10
			}
			result = append(result, DuplicateGroup{
				Items:           formatted,
				GroupSimilarity: groupSim,
			})
		}
	}
	return result
}

func calculateSpaceSavings(job *ScanJob) int64 {
	var total int64
	for _, group := range job.ImageGroups {
		total += groupSavings(group.Items)
	}
	for _, group := range job.VideoGroups {
		total += groupSavings(group.Items)
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
