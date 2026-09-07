package web

import (
	"net/http"

	"github.com/exterex/morphic/internal/organizer"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func registerOrganizerRoutes(r *gin.Engine, pool *pgxpool.Pool) {
	g := r.Group("/api/organizer")
	{
		g.POST("/plan", func(c *gin.Context) { handleOrganizerPlan(c, pool) })
		g.POST("/execute", handleOrganizerExecute)
		g.GET("/status/:id", handleOrganizerStatus)
		g.POST("/cancel/:id", handleOrganizerCancel)
	}
}

func handleOrganizerPlan(c *gin.Context, pool *pgxpool.Pool) {
	var req struct {
		Folder      string `json:"folder"`
		Mode        string `json:"mode"`
		Template    string `json:"template"`
		Destination string `json:"destination"`
		Operation   string `json:"operation"`
		StartSeq    *int   `json:"start_seq"`
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
	if req.Destination != "" {
		req.Destination = expandPath(req.Destination)
		if !isAbsPath(req.Destination) || !isDir(req.Destination) {
			respondError(c, http.StatusBadRequest, "INVALID_DESTINATION", "Invalid destination: "+req.Destination)
			return
		}
	}
	if req.Mode != "sort" && req.Mode != "rename" {
		req.Mode = "sort"
	}
	if req.Operation != "copy" && req.Operation != "move" {
		req.Operation = "copy"
	}
	startSeq := 1
	if req.StartSeq != nil && *req.StartSeq > 0 {
		startSeq = *req.StartSeq
	}

	jobID := organizer.StartPlanJob(c.Request.Context(), pool, req.Folder, req.Mode, req.Template, req.Destination, req.Operation, startSeq)
	c.JSON(http.StatusAccepted, gin.H{"job_id": jobID})
}

func handleOrganizerExecute(c *gin.Context) {
	var req struct {
		JobID string `json:"job_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.JobID == "" {
		respondError(c, http.StatusBadRequest, "MISSING_JOB_ID", "job_id is required")
		return
	}

	if !organizer.ExecuteJob(c.Request.Context(), req.JobID) {
		respondError(c, http.StatusBadRequest, "EXECUTE_FAILED", "Job not found or not in planned state")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "executing"})
}

func handleOrganizerStatus(c *gin.Context) {
	id := c.Param("id")
	job, ok := organizer.GetJob(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}

	resp := gin.H{
		"id":        job.ID,
		"status":    job.Status,
		"phase":     job.Phase,
		"mode":      job.Mode,
		"operation": job.Operation,
		"progress":  job.Progress,
		"message":   job.Message,
		"error":     job.Error,
	}

	if job.Phase == "planned" || job.Phase == "executing" || job.Phase == "done" {
		plan := organizer.GetUnifiedPlan(job)
		conflicts := 0
		for _, e := range plan {
			if e.Conflict {
				conflicts++
			}
		}
		resp["plan"] = plan
		resp["plan_count"] = len(plan)
		resp["conflicts"] = conflicts
	}

	if job.Phase == "done" {
		resp["execution"] = organizer.GetExecutionResult(job)
	}

	c.JSON(http.StatusOK, resp)
}

func handleOrganizerCancel(c *gin.Context) {
	id := c.Param("id")
	if !organizer.CancelJob(id) {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelling"})
}
