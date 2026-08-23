package web

import (
	"context"
	"net/http"

	"github.com/exterex/morphic/internal/organizer"
	"github.com/gin-gonic/gin"
)

func registerOrganizerRoutes(r *gin.Engine) {
	g := r.Group("/api/organizer")
	{
		g.POST("/plan", handleOrganizerPlan)
		g.POST("/execute", handleOrganizerExecute)
		g.GET("/status/:id", handleOrganizerStatus)
		g.GET("/status/:id/stream", handleOrganizerStream)
		g.POST("/cancel/:id", handleOrganizerCancel)
	}
}

func handleOrganizerPlan(c *gin.Context) {
	var req struct {
		Folder      string `json:"folder"`
		Mode        string `json:"mode"`
		Template    string `json:"template"`
		Destination string `json:"destination"`
		Operation   string `json:"operation"`
		StartSeq    int    `json:"start_seq"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	if req.Folder == "" || !isAbsPath(req.Folder) || !isDir(req.Folder) {
		respondError(c, http.StatusBadRequest, "INVALID_FOLDER", "Valid folder is required")
		return
	}
	if req.StartSeq <= 0 {
		req.StartSeq = 1
	}

	jobID := organizer.StartPlanJob(
		context.Background(),
		req.Folder, req.Mode, req.Template,
		req.Destination, req.Operation, req.StartSeq,
	)

	c.JSON(http.StatusAccepted, gin.H{"job_id": jobID})
}

func handleOrganizerExecute(c *gin.Context) {
	var req struct {
		JobID string `json:"job_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.JobID == "" {
		respondError(c, http.StatusBadRequest, "MISSING_JOB_ID", "job_id required")
		return
	}

	if !organizer.ExecuteJob(context.Background(), req.JobID) {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "job not found or not in planned state")
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"status": "executing", "job_id": req.JobID})
}

func handleOrganizerStatus(c *gin.Context) {
	id := c.Param("id")
	job, ok := organizer.GetJob(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "job not found")
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

	// Include plan when planning is done
	if job.Phase == "planned" || job.Phase == "executing" || job.Phase == "done" {
		plan := organizer.GetUnifiedPlan(job)
		resp["plan"] = plan
		resp["plan_count"] = len(plan)

		conflicts := 0
		for _, entry := range plan {
			if entry.Conflict {
				conflicts++
			}
		}
		resp["conflicts"] = conflicts
	}

	// Include execution results when done
	if job.Phase == "done" {
		resp["execution"] = organizer.GetExecutionResult(job)
	}

	c.JSON(http.StatusOK, resp)
}

func handleOrganizerCancel(c *gin.Context) {
	id := c.Param("id")
	if !organizer.CancelJob(id) {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "job not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelling"})
}
