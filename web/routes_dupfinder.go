package web

import (
	"net/http"
	"os"
	"time"

	"github.com/andreassag/morphic/internal/dupfinder"
	"github.com/andreassag/morphic/internal/shared"
	"github.com/gin-gonic/gin"
)

func registerDupfinderRoutes(r *gin.Engine) {
	g := r.Group("/api/dupfinder")
	{
		g.POST("/scan", handleDupfinderScan)
		g.GET("/scan/:id/status", handleDupfinderStatus)
		g.GET("/scan/:id/results", handleDupfinderResults)
		g.POST("/scan/:id/cancel", handleDupfinderCancel)
		g.POST("/delete", handleDupfinderDelete)
	}
}

func handleDupfinderScan(c *gin.Context) {
	var req struct {
		Folder         string  `json:"folder"`
		Type           string  `json:"type"`
		ImageThreshold float64 `json:"image_threshold"`
		VideoThreshold float64 `json:"video_threshold"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Folder == "" || !isAbsPath(req.Folder) || !isDir(req.Folder) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid folder: " + req.Folder})
		return
	}
	if req.Type == "" {
		req.Type = "both"
	}
	if req.Type != "images" && req.Type != "videos" && req.Type != "both" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type must be images, videos, or both"})
		return
	}
	if req.ImageThreshold == 0 {
		req.ImageThreshold = shared.DefaultImageThreshold
	}
	if req.VideoThreshold == 0 {
		req.VideoThreshold = shared.DefaultVideoThreshold
	}

	jobID := dupfinder.StartJob(req.Folder, req.Type, req.ImageThreshold, req.VideoThreshold)
	c.JSON(http.StatusAccepted, gin.H{"job_id": jobID})
}

func handleDupfinderStatus(c *gin.Context) {
	id := c.Param("id")
	job, ok := dupfinder.GetJob(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
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
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}

	if job.Status != "done" && job.Status != "failed" {
		c.JSON(http.StatusConflict, gin.H{"error": "Scan not finished yet"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"image_groups":            job.ImageGroups,
		"video_groups":            job.VideoGroups,
		"space_savings":           job.SpaceSavings,
		"space_savings_formatted": shared.FormatFileSize(job.SpaceSavings),
	})
}

func handleDupfinderCancel(c *gin.Context) {
	id := c.Param("id")
	job, ok := dupfinder.GetJob(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}
	job.Cancel()
	c.JSON(http.StatusOK, gin.H{"status": "cancelling"})
}

func handleDupfinderDelete(c *gin.Context) {
	var req struct {
		Files []string `json:"files"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No files specified"})
		return
	}

	var results []map[string]interface{}
	totalFreed := int64(0)

	for _, fp := range req.Files {
		if !isAbsPath(fp) {
			results = append(results, map[string]interface{}{"path": fp, "status": "not_found"})
			continue
		}
		info, err := os.Stat(fp)
		if err != nil {
			results = append(results, map[string]interface{}{"path": fp, "status": "not_found"})
			continue
		}
		if info.IsDir() {
			results = append(results, map[string]interface{}{"path": fp, "status": "not_found"})
			continue
		}
		size := info.Size()
		if err := os.Remove(fp); err != nil {
			if os.IsPermission(err) {
				results = append(results, map[string]interface{}{"path": fp, "status": "permission_denied"})
			} else {
				results = append(results, map[string]interface{}{"path": fp, "status": "error", "error": err.Error()})
			}
		} else {
			totalFreed += size
			results = append(results, map[string]interface{}{"path": fp, "status": "deleted", "size_freed": size})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"results":               results,
		"total_freed":           totalFreed,
		"total_freed_formatted": shared.FormatFileSize(totalFreed),
	})
}
