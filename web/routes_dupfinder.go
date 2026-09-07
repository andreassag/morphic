package web

import (
	"net/http"
	"time"

	"github.com/exterex/morphic/internal/dupfinder"
	"github.com/exterex/morphic/internal/shared"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerDupfinderRoutes(r *gin.Engine, pool *pgxpool.Pool) {
	g := r.Group("/api/dupfinder")
	{
		g.POST("/scan", func(c *gin.Context) { handleDupfinderScan(c, pool) })
		g.GET("/scan/:id/status", handleDupfinderStatus)
		g.GET("/scan/:id/results", handleDupfinderResults)
		g.POST("/scan/:id/cancel", handleDupfinderCancel)
		g.POST("/auto-select", handleDupfinderAutoSelect)
		g.POST("/delete", func(c *gin.Context) { handleDupfinderDelete(c, pool) })
	}
}

func handleDupfinderScan(c *gin.Context, pool *pgxpool.Pool) {
	var req struct {
		Folder         string  `json:"folder"`
		Type           string  `json:"type"`
		ImageThreshold float64 `json:"image_threshold"`
		VideoThreshold float64 `json:"video_threshold"`
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
	if req.Type == "" {
		req.Type = "both"
	}
	if req.Type != "images" && req.Type != "videos" && req.Type != "both" {
		respondError(c, http.StatusBadRequest, "INVALID_TYPE", "type must be images, videos, or both")
		return
	}
	if req.ImageThreshold == 0 {
		req.ImageThreshold = shared.DefaultImageThreshold
	}
	if req.VideoThreshold == 0 {
		req.VideoThreshold = shared.DefaultVideoThreshold
	}

	jobID := dupfinder.StartJob(c.Request.Context(), pool, req.Folder, req.Type, req.ImageThreshold, req.VideoThreshold)
	c.JSON(http.StatusAccepted, gin.H{"job_id": jobID})
}

func handleDupfinderStatus(c *gin.Context) {
	id := c.Param("id")
	job, ok := dupfinder.GetJob(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}

	elapsed := 0.0
	if !job.StartedAt.IsZero() {
		end := job.DoneAt
		if end.IsZero() {
			end = time.Now()
		}
		elapsed = end.Sub(job.StartedAt).Seconds()
	}

	c.JSON(http.StatusOK, gin.H{
		"id":                    job.ID,
		"status":                job.Status,
		"progress":              job.Progress,
		"message":               job.Message,
		"error":                 job.Error,
		"total_files_found":     job.TotalFound,
		"total_files_processed": job.TotalProcessed,
		"elapsed_seconds":       round1(elapsed),
	})
}

func handleDupfinderResults(c *gin.Context) {
	id := c.Param("id")
	job, ok := dupfinder.GetJob(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}

	if job.Status != shared.JobStatusDone && job.Status != shared.JobStatusFailed {
		respondError(c, http.StatusConflict, "JOB_RUNNING", "Scan not finished yet")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"image_groups":            job.ImageGroups,
		"video_groups":            job.VideoGroups,
		"space_savings":           job.SpaceSavings,
		"space_savings_formatted": shared.FormatFileSize(job.SpaceSavings),
	})
}

func handleDupfinderAutoSelect(c *gin.Context) {
	var req struct {
		JobID string                `json:"job_id"`
		Rule  dupfinder.CullingRule `json:"rule"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	job, ok := dupfinder.GetJob(req.JobID)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}

	var allGroups []dupfinder.DuplicateGroup
	allGroups = append(allGroups, job.ImageGroups...)
	allGroups = append(allGroups, job.VideoGroups...)

	result := dupfinder.ApplyCullingRule(allGroups, req.Rule)
	c.JSON(http.StatusOK, result)
}

func handleDupfinderCancel(c *gin.Context) {
	id := c.Param("id")
	if !dupfinder.CancelJob(id) {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelling"})
}

func handleDupfinderDelete(c *gin.Context, pool *pgxpool.Pool) {
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

	c.JSON(http.StatusOK, executeDeleteFiles(c.Request.Context(), pool, req.Files))
}
