package web

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/andreassag/morphic/internal/database"
	"github.com/andreassag/morphic/internal/events"
	"github.com/andreassag/morphic/internal/importer"
	"github.com/andreassag/morphic/internal/shared"
	"github.com/andreassag/morphic/internal/trash"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerImporterRoutes(r *gin.Engine, pool *pgxpool.Pool) {
	g := r.Group("/api/importer")
	{
		g.POST("/url", func(c *gin.Context) { handleImporterURL(c, pool) })
		g.POST("/s3", func(c *gin.Context) { handleImporterS3(c, pool) })
		g.GET("/devices", handleImporterDevices)
		g.POST("/device", func(c *gin.Context) { handleImporterDevice(c, pool) })
		g.GET("/status/:id", handleImporterStatus)
		g.POST("/cancel/:id", handleImporterCancel)
	}
}

func handleImporterURL(c *gin.Context, pool *pgxpool.Pool) {
	var req importer.URLImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	job := &importer.ImporterJob{
		Job:       shared.NewJob(),
		Type:      "url",
		TargetDir: req.DestinationDir,
	}
	job.Status = shared.JobStatusRunning
	importer.SetJob(job.ID, job)

	jobCtx, cancel := context.WithCancel(context.Background())
	importer.RegisterCancel(job.ID, cancel)

	go func() {
		defer cancel()
		f, err := importer.ImportFromURL(jobCtx, req)
		if err != nil {
			job.SetFailed(err.Error())
		} else {
			job.SetDone([]string{f})
			op := database.AuditOperation{
				Operation: "import_url",
				Source:    "importer",
				Summary:   fmt.Sprintf("Imported file from %s", req.URL),
				ItemCount: 1,
				Status:    "completed",
				CreatedAt: time.Now(),
			}
			if pool != nil {
				_, _ = database.LogOperation(context.Background(), pool, op)
			} else {
				_ = trash.LogStandaloneOperation(op)
			}
			events.DefaultBus.Publish("history", "operation_logged", op)
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID})
}

func handleImporterS3(c *gin.Context, pool *pgxpool.Pool) {
	var req importer.S3ImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	job := &importer.ImporterJob{
		Job:       shared.NewJob(),
		Type:      "s3",
		TargetDir: req.DestinationDir,
	}
	job.Status = shared.JobStatusRunning
	importer.SetJob(job.ID, job)

	jobCtx, cancel := context.WithCancel(context.Background())
	importer.RegisterCancel(job.ID, cancel)

	go func() {
		defer cancel()
		files, err := importer.ImportFromS3(jobCtx, req)
		if err != nil {
			job.SetFailed(err.Error())
		} else {
			job.SetDone(files)
			op := database.AuditOperation{
				Operation: "import_s3",
				Source:    "importer",
				Summary:   fmt.Sprintf("Imported %d files from s3://%s/%s", len(files), req.Bucket, req.Prefix),
				ItemCount: len(files),
				Status:    "completed",
				CreatedAt: time.Now(),
			}
			if pool != nil {
				_, _ = database.LogOperation(context.Background(), pool, op)
			} else {
				_ = trash.LogStandaloneOperation(op)
			}
			events.DefaultBus.Publish("history", "operation_logged", op)
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID})
}

func handleImporterDevices(c *gin.Context) {
	devs, err := importer.DetectMountedDevices(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "DETECT_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"devices": devs})
}

func handleImporterDevice(c *gin.Context, pool *pgxpool.Pool) {
	var req importer.DeviceImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	job := &importer.ImporterJob{
		Job:       shared.NewJob(),
		Type:      "device",
		TargetDir: req.DestinationDir,
	}
	job.Status = shared.JobStatusRunning
	importer.SetJob(job.ID, job)

	jobCtx, cancel := context.WithCancel(context.Background())
	importer.RegisterCancel(job.ID, cancel)

	go func() {
		defer cancel()
		files, err := importer.ImportFromDevice(jobCtx, req)
		if err != nil {
			job.SetFailed(err.Error())
		} else {
			job.SetDone(files)
			op := database.AuditOperation{
				Operation: "import_device",
				Source:    "importer",
				Summary:   fmt.Sprintf("Imported %d files from device %s", len(files), req.SourceDir),
				ItemCount: len(files),
				Status:    "completed",
				CreatedAt: time.Now(),
			}
			if pool != nil {
				_, _ = database.LogOperation(context.Background(), pool, op)
			} else {
				_ = trash.LogStandaloneOperation(op)
			}
			events.DefaultBus.Publish("history", "operation_logged", op)
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID})
}

func handleImporterStatus(c *gin.Context) {
	id := c.Param("id")
	job, ok := importer.GetJob(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}
	c.JSON(http.StatusOK, job.Snapshot())
}

func handleImporterCancel(c *gin.Context) {
	id := c.Param("id")
	if importer.CancelJob(id) {
		c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
		return
	}
	respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found or already completed")
}
