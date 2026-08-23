package web

import (
	"io"
	"net/http"
	"time"

	"github.com/exterex/morphic/internal/dupfinder"
	"github.com/exterex/morphic/internal/organizer"
	"github.com/exterex/morphic/internal/shared"
	"github.com/gin-gonic/gin"
)

func handleConverterStream(c *gin.Context) {
	id := c.Param("id")
	job, ok := conversionStore.Get(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	lastCompleted := -1
	lastStatus := shared.JobStatus("")

	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()

	c.Stream(func(w io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			return false
		case <-ticker.C:
			job.mu.RLock()
			completed := job.Completed
			status := job.Status
			currentFile := job.CurrentFile
			total := job.Total
			errMsg := job.Error
			results := make([]ConversionResult, len(job.Results))
			copy(results, job.Results)
			job.mu.RUnlock()

			if completed != lastCompleted || status != lastStatus {
				lastCompleted = completed
				lastStatus = status

				c.SSEvent("message", gin.H{
					"id":           id,
					"status":       status,
					"total":        total,
					"completed":    completed,
					"current_file": currentFile,
					"results":      results,
					"error":        errMsg,
				})
				w.(http.Flusher).Flush()
			}

			if status == shared.JobStatusDone || status == shared.JobStatusFailed || status == shared.JobStatusCancelled {
				return false
			}
			return true
		}
	})
}

func handleDupfinderStream(c *gin.Context) {
	id := c.Param("id")
	job, ok := dupfinder.GetJob(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	lastProgress := -1.0
	lastStatus := shared.JobStatus("")

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	c.Stream(func(w io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			return false
		case <-ticker.C:
			// Read job state
			status := job.Status
			progress := job.Progress
			message := job.Message
			errMsg := job.Error
			totalFound := job.TotalFound
			totalProcessed := job.TotalProcessed

			elapsed := 0.0
			if !job.StartedAt.IsZero() {
				end := job.DoneAt
				if end.IsZero() {
					end = time.Now()
				}
				elapsed = end.Sub(job.StartedAt).Seconds()
			}

			if progress != lastProgress || status != lastStatus {
				lastProgress = progress
				lastStatus = status

				c.SSEvent("message", gin.H{
					"id":                    job.ID,
					"status":                status,
					"progress":              progress,
					"message":               message,
					"error":                 errMsg,
					"total_files_found":     totalFound,
					"total_files_processed": totalProcessed,
					"elapsed_seconds":       round1(elapsed),
				})
				w.(http.Flusher).Flush()
			}

			if status == shared.JobStatusDone || status == shared.JobStatusFailed || status == shared.JobStatusCancelled {
				return false
			}
			return true
		}
	})
}

func handleOrganizerStream(c *gin.Context) {
	id := c.Param("id")
	job, ok := organizer.GetJob(id)
	if !ok {
		respondError(c, http.StatusNotFound, "NOT_FOUND", "Job not found")
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	lastPhase := ""
	lastStatus := shared.JobStatus("")
	lastProgress := -1.0

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	c.Stream(func(w io.Writer) bool {
		select {
		case <-c.Request.Context().Done():
			return false
		case <-ticker.C:
			status := job.Status
			phase := job.Phase
			progress := job.Progress
			message := job.Message
			errMsg := job.Error

			if phase != lastPhase || status != lastStatus || progress != lastProgress {
				lastPhase = phase
				lastStatus = status
				lastProgress = progress

				resp := gin.H{
					"id":        job.ID,
					"status":    status,
					"phase":     phase,
					"mode":      job.Mode,
					"operation": job.Operation,
					"progress":  progress,
					"message":   message,
					"error":     errMsg,
				}

				if phase == "planned" || phase == "executing" || phase == "done" {
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

				if phase == "done" {
					resp["execution"] = organizer.GetExecutionResult(job)
				}

				c.SSEvent("message", resp)
				w.(http.Flusher).Flush()
			}

			if (phase == "done" && status == shared.JobStatusDone) || status == shared.JobStatusFailed || status == shared.JobStatusCancelled {
				return false
			}
			return true
		}
	})
}
