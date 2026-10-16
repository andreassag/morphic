package web

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/andreassag/morphic/internal/database"
	"github.com/andreassag/morphic/internal/shared"
	"github.com/andreassag/morphic/internal/tagger"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerTaggerRoutes(r *gin.Engine, pool *pgxpool.Pool) {
	g := r.Group("/api/tagger")
	{
		g.POST("/scan", handleTaggerScan)
		g.POST("/update", func(c *gin.Context) { handleTaggerUpdate(c, pool) })
		g.POST("/strip", func(c *gin.Context) { handleTaggerStrip(c, pool) })
		g.GET("/status/:id", handleTaggerStatus)
		g.POST("/cancel/:id", handleTaggerCancel)
	}
}

func handleTaggerScan(c *gin.Context) {
	var req struct {
		Folder    string `json:"folder"`
		Recursive bool   `json:"recursive"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	safeDir, err := shared.ValidateSafeDirPath(req.Folder)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_FOLDER", err.Error())
		return
	}

	job := &tagger.TaggerJob{
		Job:    shared.NewJob(),
		Type:   "scan",
		Folder: safeDir,
	}
	job.Status = shared.JobStatusRunning
	tagger.SetJob(job.ID, job)

	jobCtx, cancel := context.WithCancel(context.Background())
	tagger.RegisterCancel(job.ID, cancel)

	go func() {
		defer cancel()
		items, err := tagger.ScanMetadata(jobCtx, safeDir, req.Recursive)
		if err != nil {
			job.Status = shared.JobStatusFailed
			job.Error = err.Error()
		} else {
			job.Status = shared.JobStatusDone
			job.Progress = 1.0
			job.Items = items
		}
		job.DoneAt = time.Now()
	}()

	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID})
}

func handleTaggerUpdate(c *gin.Context, pool *pgxpool.Pool) {
	var req struct {
		Files []string             `json:"files"`
		Patch tagger.MetadataPatch `json:"patch"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if len(req.Files) == 0 {
		respondError(c, http.StatusBadRequest, "EMPTY_FILES", "No files provided")
		return
	}

	var validFiles []string
	for _, f := range req.Files {
		if safe, err := shared.ValidateSafePath(f); err == nil {
			validFiles = append(validFiles, safe)
		}
	}
	if len(validFiles) == 0 {
		respondError(c, http.StatusBadRequest, "INVALID_FILES", "No valid files provided")
		return
	}

	ctx := c.Request.Context()
	updated, err := tagger.BatchUpdateMetadata(ctx, validFiles, req.Patch)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "UPDATE_FAILED", err.Error())
		return
	}

	if pool != nil {
		_, _ = database.LogOperation(ctx, pool, database.AuditOperation{
			Operation: "update_tags",
			Source:    "tagger",
			Summary:   fmt.Sprintf("Updated metadata on %d files", updated),
			ItemCount: updated,
			Status:    "completed",
		})
	}

	c.JSON(http.StatusOK, gin.H{"updated": updated})
}

func handleTaggerStrip(c *gin.Context, pool *pgxpool.Pool) {
	var req struct {
		Files []string `json:"files"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if len(req.Files) == 0 {
		respondError(c, http.StatusBadRequest, "EMPTY_FILES", "No files provided")
		return
	}

	var validFiles []string
	for _, f := range req.Files {
		if safe, err := shared.ValidateSafePath(f); err == nil {
			validFiles = append(validFiles, safe)
		}
	}
	if len(validFiles) == 0 {
		respondError(c, http.StatusBadRequest, "INVALID_FILES", "No valid files provided")
		return
	}

	ctx := c.Request.Context()
	stripped, err := tagger.StripMetadata(ctx, validFiles)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "STRIP_FAILED", err.Error())
		return
	}

	if pool != nil {
		_, _ = database.LogOperation(ctx, pool, database.AuditOperation{
			Operation: "strip_metadata",
			Source:    "tagger",
			Summary:   fmt.Sprintf("Stripped metadata from %d files", stripped),
			ItemCount: stripped,
			Status:    "completed",
		})
		for _, f := range validFiles {
			_, _ = database.LogAction(ctx, pool, database.AuditEntry{
				Operation:    "strip_metadata",
				OriginalPath: f,
				Reversible:   false,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"stripped": stripped})
}

func handleTaggerStatus(c *gin.Context) {
	id := c.Param("id")
	job, ok := tagger.GetJob(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}
	c.JSON(http.StatusOK, job)
}

func handleTaggerCancel(c *gin.Context) {
	id := c.Param("id")
	if tagger.CancelJob(id) {
		c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
		return
	}
	respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found or already completed")
}
