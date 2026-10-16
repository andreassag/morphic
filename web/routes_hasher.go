package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/andreassag/morphic/internal/hasher"
	"github.com/andreassag/morphic/internal/shared"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerHasherRoutes(r *gin.Engine, pool *pgxpool.Pool) {
	g := r.Group("/api/hasher")
	{
		g.POST("/generate", handleHasherGenerate)
		g.GET("/status/:id", handleHasherStatus)
		g.POST("/cancel/:id", handleHasherCancel)
		g.GET("/export/:id", handleHasherExport)
		g.POST("/verify", handleHasherVerify)
	}
}

func handleHasherGenerate(c *gin.Context) {
	var req struct {
		Folder     string   `json:"folder"`
		Algorithms []string `json:"algorithms"`
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

	computeMD5 := false
	computeSHA256 := true // default
	computeXXHash := false
	if len(req.Algorithms) > 0 {
		computeSHA256 = false
		for _, alg := range req.Algorithms {
			switch strings.ToLower(alg) {
			case "md5":
				computeMD5 = true
			case "sha256":
				computeSHA256 = true
			case "xxhash", "xxh64":
				computeXXHash = true
			}
		}
	}
	if !computeMD5 && !computeSHA256 && !computeXXHash {
		computeSHA256 = true
	}

	job := &hasher.HasherJob{
		Job:       shared.NewJob(),
		TargetDir: safeDir,
	}
	job.Status = shared.JobStatusRunning
	hasher.SetJob(job.ID, job)

	jobCtx, cancel := context.WithCancel(context.Background())
	hasher.RegisterCancel(job.ID, cancel)

	go func() {
		defer cancel()
		items, err := hasher.HashDirectory(jobCtx, safeDir, computeMD5, computeSHA256, computeXXHash, func(done, total int) {
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
			job.Items = items
		}
		job.DoneAt = time.Now()
	}()

	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID})
}

func handleHasherStatus(c *gin.Context) {
	id := c.Param("id")
	job, ok := hasher.GetJob(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}
	c.JSON(http.StatusOK, job)
}

func handleHasherCancel(c *gin.Context) {
	id := c.Param("id")
	if hasher.CancelJob(id) {
		c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
		return
	}
	respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found or already completed")
}

func handleHasherExport(c *gin.Context) {
	id := c.Param("id")
	job, ok := hasher.GetJob(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}
	if job.Status != shared.JobStatusDone {
		respondError(c, http.StatusBadRequest, "JOB_NOT_READY", "Job is not completed yet")
		return
	}

	format := c.DefaultQuery("format", "sha256")
	manifest, err := hasher.ExportManifest(job.Items, format)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "EXPORT_FAILED", err.Error())
		return
	}

	contentType := "text/plain; charset=utf-8"
	if format == "json" {
		contentType = "application/json"
	}

	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="manifest.%s"`, format))
	c.Data(http.StatusOK, contentType, manifest)
}

func handleHasherVerify(c *gin.Context) {
	var req struct {
		Folder   string `json:"folder"`
		Manifest string `json:"manifest"`
		Format   string `json:"format"`
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

	if req.Manifest == "" {
		respondError(c, http.StatusBadRequest, "EMPTY_MANIFEST", "Manifest content is required")
		return
	}

	res, err := hasher.VerifyManifest(c.Request.Context(), safeDir, []byte(req.Manifest), req.Format)
	if err != nil {
		respondError(c, http.StatusBadRequest, "VERIFICATION_FAILED", err.Error())
		return
	}

	c.JSON(http.StatusOK, res)
}
