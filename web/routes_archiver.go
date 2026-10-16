package web

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/andreassag/morphic/internal/archiver"
	"github.com/andreassag/morphic/internal/database"
	"github.com/andreassag/morphic/internal/shared"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerArchiverRoutes(r *gin.Engine, pool *pgxpool.Pool) {
	g := r.Group("/api/archiver")
	{
		g.POST("/create", func(c *gin.Context) { handleArchiverCreate(c, pool) })
		g.GET("/status/:id", handleArchiverStatus)
		g.POST("/cancel/:id", handleArchiverCancel)
		g.GET("/download/:id", handleArchiverDownload)
	}
}

func handleArchiverCreate(c *gin.Context, pool *pgxpool.Pool) {
	var req archiver.ArchiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	safeDest, err := shared.ValidateSafeDirPath(req.DestinationDir)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_DESTINATION", err.Error())
		return
	}
	req.DestinationDir = safeDest

	job := &archiver.ArchiverJob{
		Job: shared.NewJob(),
	}
	job.Status = shared.JobStatusRunning
	archiver.SetJob(job.ID, job)

	jobCtx, cancel := context.WithCancel(context.Background())
	archiver.RegisterCancel(job.ID, cancel)

	go func() {
		defer cancel()
		files, err := archiver.CreateArchive(jobCtx, req, func(written, total int64) {
			job.TotalBytes = written
		})
		if err != nil {
			job.Status = shared.JobStatusFailed
			job.Error = err.Error()
		} else {
			job.Status = shared.JobStatusDone
			job.Progress = 1.0
			job.CreatedFiles = files
			if pool != nil {
				_, _ = database.LogOperation(context.Background(), pool, database.AuditOperation{
					Operation: "create_archive",
					Source:    "archiver",
					Summary:   fmt.Sprintf("Created %s archive with %d parts", req.Format, len(files)),
					ItemCount: len(files),
					Status:    "completed",
				})
			}
		}
		job.DoneAt = time.Now()
	}()

	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID})
}

func handleArchiverStatus(c *gin.Context) {
	id := c.Param("id")
	job, ok := archiver.GetJob(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}
	c.JSON(http.StatusOK, job)
}

func handleArchiverCancel(c *gin.Context) {
	id := c.Param("id")
	if archiver.CancelJob(id) {
		c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
		return
	}
	respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found or already completed")
}

func handleArchiverDownload(c *gin.Context) {
	id := c.Param("id")
	job, ok := archiver.GetJob(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}
	if job.Status != shared.JobStatusDone || len(job.CreatedFiles) == 0 {
		respondError(c, http.StatusBadRequest, "NOT_READY", "Archive is not ready for download")
		return
	}

	target := job.CreatedFiles[0]
	if _, err := os.Stat(target); err != nil {
		respondError(c, http.StatusNotFound, "FILE_NOT_FOUND", "Archive file missing on disk")
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(target)))
	c.File(target)
}
