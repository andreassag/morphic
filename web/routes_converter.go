package web

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/exterex/morphic/internal/converter"
	"github.com/exterex/morphic/internal/shared"
	"github.com/gin-gonic/gin"
)

var conversionStore = shared.NewJobStore[conversionJob]()

// ConversionResult stores the conversion result of a single file.
type ConversionResult struct {
	Source          string `json:"source"`
	Destination     string `json:"destination,omitempty"`
	Status          string `json:"status"` // "ok" | "error"
	Error           string `json:"error,omitempty"`
	OriginalSize    int64  `json:"original_size,omitempty"`
	NewSize         int64  `json:"new_size,omitempty"`
	OriginalSizeFmt string `json:"original_size_fmt,omitempty"`
	NewSizeFmt      string `json:"new_size_fmt,omitempty"`
	SourceDeleted   bool   `json:"source_deleted"`
}

type conversionJob struct {
	shared.Job
	mu          sync.RWMutex
	cond        *sync.Cond
	Total       int                `json:"total"`
	Completed   int                `json:"completed"`
	CurrentFile string             `json:"current_file"`
	Results     []ConversionResult `json:"results"`
}

func newConversionJob(total int) *conversionJob {
	j := &conversionJob{
		Job:     shared.NewJob(),
		Total:   total,
		Results: make([]ConversionResult, 0, total),
	}
	j.cond = sync.NewCond(&j.mu)
	return j
}

// StartConverterCleanup starts background cleanup for conversion jobs.
func StartConverterCleanup(ctx context.Context, ttl time.Duration) {
	conversionStore.StartCleanup(ctx, ttl, func(j *conversionJob) time.Time {
		j.mu.RLock()
		defer j.mu.RUnlock()
		return j.DoneAt
	})
}

func registerConverterRoutes(r *gin.Engine) {
	g := r.Group("/api/converter")
	{
		g.POST("/scan", handleConverterScan)
		g.GET("/formats", handleConverterFormats)
		g.POST("/convert", handleConverterConvert)
		g.GET("/progress/:id", handleConverterProgress)
		g.GET("/progress/:id/poll", handleConverterPoll)
		g.GET("/progress/:id/stream", handleConverterStream)
		g.POST("/progress/:id/cancel", handleConverterCancel)
		g.POST("/delete", handleConverterDelete)
	}
}

func handleConverterScan(c *gin.Context) {
	var req struct {
		Folder            string `json:"folder"`
		IncludeSubfolders *bool  `json:"include_subfolders"`
		FilterType        string `json:"filter_type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.Folder == "" || !isAbsPath(req.Folder) || !isDir(req.Folder) {
		respondError(c, http.StatusBadRequest, "INVALID_FOLDER", "Invalid folder: "+req.Folder)
		return
	}
	includeSub := true
	if req.IncludeSubfolders != nil {
		includeSub = *req.IncludeSubfolders
	}
	filterType := req.FilterType
	if filterType != "images" && filterType != "videos" && filterType != "both" {
		filterType = "both"
	}

	result, err := converter.ScanFolder(req.Folder, includeSub, filterType)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "SCAN_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusOK, result)
}

func handleConverterFormats(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"image": converter.ImageConversions,
		"video": gin.H{
			"containers": converter.VideoContainers,
		},
	})
}

func handleConverterConvert(c *gin.Context) {
	var req struct {
		Files          []string `json:"files"`
		TargetExt      string   `json:"target_ext"`
		Codec          string   `json:"codec"`
		DeleteOriginal bool     `json:"delete_original"`
		AV1CRF         *int     `json:"av1_crf"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if len(req.Files) == 0 || req.TargetExt == "" {
		respondError(c, http.StatusBadRequest, "MISSING_PARAMETERS", "files and target_ext required")
		return
	}
	if !converter.IsValidTargetExt(req.TargetExt) {
		respondError(c, http.StatusBadRequest, "INVALID_TARGET_EXT", "Unsupported or invalid target_ext: "+req.TargetExt)
		return
	}
	for _, f := range req.Files {
		if !isAbsPath(f) {
			respondError(c, http.StatusBadRequest, "INVALID_PATH", "Invalid file path: "+f)
			return
		}
	}

	av1CRF := 0
	if req.AV1CRF != nil {
		av1CRF = *req.AV1CRF
	}

	job := newConversionJob(len(req.Files))
	job.Status = shared.JobStatusRunning
	conversionStore.Set(job.ID, job)

	bgCtx, bgCancel := context.WithCancel(context.Background())
	conversionStore.RegisterCancel(job.ID, bgCancel)

	go runConversion(bgCtx, job, req.Files, req.TargetExt, req.Codec, req.DeleteOriginal, av1CRF)

	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID})
}

func runConversion(ctx context.Context, job *conversionJob, files []string, targetExt, codec string, deleteOriginal bool, av1CRF int) {
	defer func() {
		job.mu.Lock()
		job.CurrentFile = ""
		if job.DoneAt.IsZero() {
			job.DoneAt = time.Now()
		}
		job.cond.Broadcast()
		job.mu.Unlock()
	}()

	for i, source := range files {
		// Check for cancellation before each file
		select {
		case <-ctx.Done():
			job.mu.Lock()
			job.Status = shared.JobStatusCancelled
			job.CurrentFile = ""
			job.DoneAt = time.Now()
			job.cond.Broadcast()
			job.mu.Unlock()
			return
		default:
		}

		job.mu.Lock()
		job.CurrentFile = source
		job.cond.Broadcast()
		job.mu.Unlock()

		result := ConversionResult{
			Source:        source,
			SourceDeleted: false,
		}

		origSize := int64(0)
		if info, err := os.Stat(source); err == nil {
			origSize = info.Size()
		}

		dest, err := converter.ConvertFile(ctx, source, targetExt, codec, "", av1CRF)
		if err != nil {
			result.Status = "error"
			result.Error = err.Error()
		} else {
			newSize := int64(0)
			if info, err := os.Stat(dest); err == nil {
				newSize = info.Size()
			}

			result.Destination = dest
			result.Status = "ok"
			result.OriginalSize = origSize
			result.NewSize = newSize
			result.OriginalSizeFmt = shared.FormatFileSize(origSize)
			result.NewSizeFmt = shared.FormatFileSize(newSize)

			// Delete original only if explicitly requested and safe
			if deleteOriginal && dest != "" {
				absSrc, errSrc := filepath.Abs(source)
				absDest, errDest := filepath.Abs(dest)
				if errSrc == nil && errDest == nil && absSrc != absDest && newSize > 0 {
					if err := os.Remove(source); err == nil {
						result.SourceDeleted = true
					}
				}
			}
		}

		job.mu.Lock()
		job.Results = append(job.Results, result)
		job.Completed = i + 1
		job.cond.Broadcast()
		job.mu.Unlock()
	}

	job.mu.Lock()
	job.Status = shared.JobStatusDone
	job.CurrentFile = ""
	job.DoneAt = time.Now()
	job.cond.Broadcast()
	job.mu.Unlock()
}

func handleConverterProgress(c *gin.Context) {
	id := c.Param("id")
	job, ok := conversionStore.Get(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}

	job.mu.RLock()
	defer job.mu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"id":           job.ID,
		"status":       job.Status,
		"total":        job.Total,
		"completed":    job.Completed,
		"current_file": job.CurrentFile,
		"results":      job.Results,
		"error":        job.Error,
	})
}

func handleConverterPoll(c *gin.Context) {
	id := c.Param("id")
	job, ok := conversionStore.Get(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}

	last := -1
	if lastStr := c.Query("last"); lastStr != "" {
		if n, err := strconv.Atoi(lastStr); err == nil {
			last = n
		}
	}

	// Fast wait on condition variable instead of busy spin
	job.mu.Lock()
	deadline := time.Now().Add(10 * time.Second)
	for job.Completed == last && job.Status == shared.JobStatusRunning && time.Now().Before(deadline) {
		// Wait with short timeout
		job.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		job.mu.Lock()
	}

	resp := gin.H{
		"id":           job.ID,
		"status":       job.Status,
		"total":        job.Total,
		"completed":    job.Completed,
		"current_file": job.CurrentFile,
		"results":      job.Results,
		"error":        job.Error,
	}
	job.mu.Unlock()

	c.JSON(http.StatusOK, resp)
}

func handleConverterDelete(c *gin.Context) {
	var req struct {
		Files []string `json:"files"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if len(req.Files) == 0 {
		respondError(c, http.StatusBadRequest, "EMPTY_FILES", "No files specified")
		return
	}

	c.JSON(http.StatusOK, executeDeleteFiles(req.Files))
}

func handleConverterCancel(c *gin.Context) {
	id := c.Param("id")
	if !conversionStore.Cancel(id) {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelling"})
}
