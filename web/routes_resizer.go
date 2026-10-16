package web

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/andreassag/morphic/internal/database"
	"github.com/andreassag/morphic/internal/resizer"
	"github.com/andreassag/morphic/internal/shared"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerResizerRoutes(r *gin.Engine, pool *pgxpool.Pool) {
	g := r.Group("/api/resizer")
	{
		g.POST("/batch", func(c *gin.Context) { handleResizerBatch(c, pool) })
		g.GET("/status/:id", handleResizerStatus)
		g.POST("/cancel/:id", handleResizerCancel)
	}
}

func handleResizerBatch(c *gin.Context, pool *pgxpool.Pool) {
	var req resizer.BatchResizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	safeSrc, err := shared.ValidateSafeDirPath(req.SourceDir)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_SOURCE", err.Error())
		return
	}
	req.SourceDir = safeSrc

	safeDest, err := shared.ValidateSafeDirPath(req.DestinationDir)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_DESTINATION", err.Error())
		return
	}
	req.DestinationDir = safeDest

	job := &resizer.ResizerJob{
		Job:            shared.NewJob(),
		SourceDir:      safeSrc,
		DestinationDir: safeDest,
	}
	job.Status = shared.JobStatusRunning
	resizer.SetJob(job.ID, job)

	jobCtx, cancel := context.WithCancel(context.Background())
	resizer.RegisterCancel(job.ID, cancel)

	go func() {
		defer cancel()
		files, err := resizer.BatchResize(jobCtx, req, func(done, total int) {
			job.TotalImages = total
			job.ProcessedCount = done
			if total > 0 {
				job.Progress = float64(done) / float64(total)
			}
		})
		if err != nil {
			job.Status = shared.JobStatusFailed
			job.Error = err.Error()
		} else {
			job.Status = shared.JobStatusDone
			job.Progress = 1.0
			job.GeneratedFiles = files
			if pool != nil {
				_, _ = database.LogOperation(context.Background(), pool, database.AuditOperation{
					Operation: "batch_resize",
					Source:    "resizer",
					Summary:   fmt.Sprintf("Batch resized into %d images", len(files)),
					ItemCount: len(files),
					Status:    "completed",
				})
			}
		}
		job.DoneAt = time.Now()
	}()

	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID})
}

func handleResizerStatus(c *gin.Context) {
	id := c.Param("id")
	job, ok := resizer.GetJob(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}
	c.JSON(http.StatusOK, job)
}

func handleResizerCancel(c *gin.Context) {
	id := c.Param("id")
	if resizer.CancelJob(id) {
		c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
		return
	}
	respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found or already completed")
}
