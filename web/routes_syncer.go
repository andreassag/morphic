package web

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/andreassag/morphic/internal/database"
	"github.com/andreassag/morphic/internal/shared"
	"github.com/andreassag/morphic/internal/syncer"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerSyncerRoutes(r *gin.Engine, pool *pgxpool.Pool) {
	g := r.Group("/api/syncer")
	{
		g.POST("/plan", handleSyncerPlan)
		g.POST("/execute", func(c *gin.Context) { handleSyncerExecute(c, pool) })
		g.GET("/status/:id", handleSyncerStatus)
		g.POST("/cancel/:id", handleSyncerCancel)
	}
}

func handleSyncerPlan(c *gin.Context) {
	var req syncer.SyncPlanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	plan, err := syncer.PlanSync(c.Request.Context(), req)
	if err != nil {
		respondError(c, http.StatusBadRequest, "PLAN_FAILED", err.Error())
		return
	}

	c.JSON(http.StatusOK, plan)
}

func handleSyncerExecute(c *gin.Context, pool *pgxpool.Pool) {
	var req struct {
		PlanID string `json:"plan_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	plan, ok := syncer.GetPlan(req.PlanID)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Plan not found")
		return
	}

	job := &syncer.SyncerJob{
		Job:  shared.NewJob(),
		Plan: plan,
	}
	job.Status = shared.JobStatusRunning
	syncer.SetJob(job.ID, job)

	jobCtx, cancel := context.WithCancel(context.Background())
	syncer.RegisterCancel(job.ID, cancel)

	go func() {
		defer cancel()
		err := syncer.ExecuteSync(jobCtx, plan, func(done, total int) {
			job.SetProgress(done, total)
		})
		if err != nil {
			job.SetFailed(err.Error())
		} else {
			job.SetDone()
			if pool != nil {
				_, _ = database.LogOperation(context.Background(), pool, database.AuditOperation{
					Operation: "sync_directory",
					Source:    "syncer",
					Summary:   fmt.Sprintf("Synchronized %d files from %s to %s", len(plan.Actions), plan.SourceDir, plan.TargetDir),
					ItemCount: len(plan.Actions),
					Status:    "completed",
				})
			}
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{"job_id": job.ID})
}

func handleSyncerStatus(c *gin.Context) {
	id := c.Param("id")
	job, ok := syncer.GetJob(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}
	c.JSON(http.StatusOK, job.Snapshot())
}

func handleSyncerCancel(c *gin.Context) {
	id := c.Param("id")
	if syncer.CancelJob(id) {
		c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
		return
	}
	respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found or already completed")
}
