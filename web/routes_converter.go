package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/exterex/morphic/internal/converter"
	"github.com/exterex/morphic/internal/database"
	"github.com/exterex/morphic/internal/events"
	"github.com/exterex/morphic/internal/shared"
	"github.com/exterex/morphic/internal/trash"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
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
	Total       int                `json:"total"`
	Completed   int                `json:"completed"`
	CurrentFile string             `json:"current_file"`
	Results     []ConversionResult `json:"results"`
	Pool        *pgxpool.Pool      `json:"-"`
}

func newConversionJob(total int, pool *pgxpool.Pool) *conversionJob {
	return &conversionJob{
		Job:     shared.NewJob(),
		Total:   total,
		Results: make([]ConversionResult, 0, total),
		Pool:    pool,
	}
}

// StartConverterCleanup starts background cleanup for conversion jobs.
func StartConverterCleanup(ctx context.Context, ttl time.Duration) {
	conversionStore.StartCleanup(ctx, ttl, func(j *conversionJob) time.Time {
		j.mu.RLock()
		defer j.mu.RUnlock()
		return j.DoneAt
	})
}

func registerConverterRoutes(r *gin.Engine, pool *pgxpool.Pool) {
	g := r.Group("/api/converter")
	{
		g.POST("/scan", handleConverterScan)
		g.GET("/formats", handleConverterFormats)
		g.POST("/convert", func(c *gin.Context) { handleConverterConvert(c, pool) })
		g.GET("/progress/:id", handleConverterProgress)
		g.POST("/progress/:id/cancel", handleConverterCancel)
		g.POST("/delete", func(c *gin.Context) { handleConverterDelete(c, pool) })
	}
}

func handleConverterScan(c *gin.Context) {
	var req struct {
		Folder            string   `json:"folder"`
		IncludeSubfolders *bool    `json:"include_subfolders"`
		FilterType        string   `json:"filter_type"`
		ExcludeFolders    []string `json:"exclude_folders"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	req.Folder = expandPath(req.Folder)
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

	result, err := converter.ScanFolder(req.Folder, includeSub, filterType, req.ExcludeFolders...)
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
		"hwaccels": converter.DetectAvailableHWAccels(c.Request.Context()),
	})
}

func handleConverterConvert(c *gin.Context, pool *pgxpool.Pool) {
	var req struct {
		Files          []string `json:"files"`
		TargetExt      string   `json:"target_ext"`
		Codec          string   `json:"codec"`
		HWAccel        string   `json:"hwaccel"`
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

	job := newConversionJob(len(req.Files), pool)
	job.Status = shared.JobStatusRunning
	conversionStore.Set(job.ID, job)

	if pool != nil {
		_ = database.CreateJob(c.Request.Context(), pool, database.JobRecord{
			ID:        job.ID,
			Type:      "conversion",
			Status:    string(job.Status),
			Progress:  0,
			Message:   "Starting batch conversion...",
			StartedAt: job.StartedAt,
		})
	}

	bgCtx, bgCancel := context.WithCancel(context.Background())
	conversionStore.RegisterCancel(job.ID, bgCancel)

	go runConversion(bgCtx, job, req.Files, req.TargetExt, req.Codec, req.HWAccel, req.DeleteOriginal, av1CRF)

	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID})
}

func runConversion(ctx context.Context, job *conversionJob, files []string, targetExt, codec, hwaccel string, deleteOriginal bool, av1CRF int) {
	defer func() {
		job.mu.Lock()
		job.CurrentFile = ""
		if job.DoneAt.IsZero() {
			job.DoneAt = time.Now()
		}
		job.mu.Unlock()
	}()

	total := len(files)

	for i, source := range files {
		select {
		case <-ctx.Done():
			job.mu.Lock()
			job.Status = shared.JobStatusCancelled
			job.CurrentFile = ""
			job.DoneAt = time.Now()
			job.mu.Unlock()

			if job.Pool != nil {
				_ = database.UpdateJobStatus(context.Background(), job.Pool, job.ID, "cancelled", job.Progress, "Cancelled", "", nil)
			}
			events.DefaultBus.Publish("converter", "job_cancelled", convProgressMap(job))
			return
		default:
		}

		job.mu.Lock()
		job.CurrentFile = source
		job.Progress = float64(i) / float64(total)
		job.mu.Unlock()

		events.DefaultBus.Publish("converter", "progress", convProgressMap(job))

		result := ConversionResult{
			Source:        source,
			SourceDeleted: false,
		}

		origSize := int64(0)
		if info, err := os.Stat(source); err == nil {
			origSize = info.Size()
		}

		dest, err := converter.ConvertFile(ctx, source, targetExt, codec, hwaccel, "", av1CRF)
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

			// Log conversion to audit history
			auditMetaBytes, _ := json.Marshal(map[string]interface{}{
				"source":        "converter",
				"target_ext":    targetExt,
				"codec":         codec,
				"original_size": origSize,
				"new_size":      newSize,
			})
			auditEntry := database.AuditEntry{
				Operation:       "convert",
				Source:          "converter",
				OriginalPath:    source,
				DestinationPath: dest,
				FileSize:        newSize,
				Metadata:        auditMetaBytes,
				Reversible:      false,
				CreatedAt:       time.Now(),
			}
			if job.Pool != nil {
				_, _ = database.LogAction(ctx, job.Pool, auditEntry)
			} else {
				_ = trash.LogStandaloneAudit(auditEntry)
			}
			events.DefaultBus.Publish("history", "new_entry", auditEntry)

			if deleteOriginal && dest != "" {
				absSrc, errSrc := filepath.Abs(source)
				absDest, errDest := filepath.Abs(dest)
				if errSrc == nil && errDest == nil && absSrc != absDest && newSize > 0 {
					_ = executeDeleteFiles(ctx, job.Pool, []string{source}, "converter")
					result.SourceDeleted = true
				}
			}
		}

		job.mu.Lock()
		job.Results = append(job.Results, result)
		job.Completed = i + 1
		job.Progress = float64(i+1) / float64(total)
		job.mu.Unlock()

		events.DefaultBus.Publish("converter", "progress", convProgressMap(job))
	}

	job.mu.Lock()
	job.Status = shared.JobStatusDone
	job.CurrentFile = ""
	job.Progress = 1.0
	job.DoneAt = time.Now()

	successCount := 0
	failCount := 0
	var totalOrigBytes int64
	var totalNewBytes int64
	for _, res := range job.Results {
		if res.Status == "ok" {
			successCount++
			totalOrigBytes += res.OriginalSize
			totalNewBytes += res.NewSize
		} else {
			failCount++
		}
	}
	job.mu.Unlock()

	if len(job.Results) > 0 {
		status := "completed"
		if failCount > 0 && successCount > 0 {
			status = "partial"
		} else if successCount == 0 {
			status = "failed"
		}

		var summary string
		if successCount > 0 {
			summary = fmt.Sprintf("Converted %d file(s) to %s (%s → %s)",
				successCount, targetExt,
				shared.FormatFileSize(totalOrigBytes),
				shared.FormatFileSize(totalNewBytes))
			if failCount > 0 {
				summary += fmt.Sprintf(" [%d failed]", failCount)
			}
		} else {
			summary = fmt.Sprintf("Failed converting %d file(s) to %s", len(job.Results), targetExt)
		}

		bulkMetaBytes, _ := json.Marshal(map[string]interface{}{
			"target_ext":  targetExt,
			"codec":       codec,
			"total_files": len(job.Results),
			"successful":  successCount,
			"failed":      failCount,
			"orig_bytes":  totalOrigBytes,
			"new_bytes":   totalNewBytes,
		})

		bulkOp := database.AuditOperation{
			Operation: "convert",
			Source:    "converter",
			Summary:   summary,
			ItemCount: len(job.Results),
			TotalSize: totalNewBytes,
			Status:    status,
			Metadata:  bulkMetaBytes,
			CreatedAt: time.Now(),
		}

		if job.Pool != nil {
			_, _ = database.LogOperation(context.Background(), job.Pool, bulkOp)
		} else {
			_ = trash.LogStandaloneOperation(bulkOp)
		}
		events.DefaultBus.Publish("history", "operation_logged", bulkOp)
	}

	if job.Pool != nil {
		_ = database.UpdateJobStatus(context.Background(), job.Pool, job.ID, "done", 1.0, "Conversion completed", "", job.Results)
	}

	events.DefaultBus.Publish("converter", "job_done", convProgressMap(job))
}

func convProgressMap(job *conversionJob) map[string]interface{} {
	job.mu.RLock()
	defer job.mu.RUnlock()

	results := make([]ConversionResult, len(job.Results))
	copy(results, job.Results)

	return map[string]interface{}{
		"id":           job.ID,
		"status":       job.Status,
		"total":        job.Total,
		"completed":    job.Completed,
		"progress":     job.Progress,
		"current_file": job.CurrentFile,
		"results":      results,
		"error":        job.Error,
	}
}

func handleConverterProgress(c *gin.Context) {
	id := c.Param("id")
	job, ok := conversionStore.Get(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}

	c.JSON(http.StatusOK, convProgressMap(job))
}

func handleConverterDelete(c *gin.Context, pool *pgxpool.Pool) {
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

	c.JSON(http.StatusOK, executeDeleteFiles(c.Request.Context(), pool, req.Files, "converter"))
}

func handleConverterCancel(c *gin.Context) {
	id := c.Param("id")
	if !conversionStore.Cancel(id) {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelling"})
}
