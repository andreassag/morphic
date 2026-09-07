package organizer

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/exterex/morphic/internal/database"
	"github.com/exterex/morphic/internal/events"
	"github.com/exterex/morphic/internal/shared"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UnifiedPlanEntry represents an organizer plan entry matching the API response.
type UnifiedPlanEntry struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Conflict    bool   `json:"conflict,omitempty"`
}

// ExecutionResult represents execution statistics.
type ExecutionResult struct {
	Completed int `json:"completed"`
	Errors    int `json:"errors"`
	Skipped   int `json:"skipped"`
}

// ScanJob represents an organizer scan/plan/execute job.
type ScanJob struct {
	shared.Job
	mu sync.Mutex

	Folder      string            `json:"folder"`
	Phase       string            `json:"phase"`
	Template    string            `json:"template"`
	Destination string            `json:"destination"`
	Operation   string            `json:"operation"`
	Mode        string            `json:"mode"`
	StartSeq    int               `json:"start_seq"`
	Files       []string          `json:"files,omitempty"`
	SortPlan    []SortPlanEntry   `json:"sort_plan,omitempty"`
	RenamePlan  []RenamePlanEntry `json:"rename_plan,omitempty"`
	Total       int               `json:"total"`
	Processed   int               `json:"processed"`
	Pool        *pgxpool.Pool     `json:"-"`
}

var (
	store  = shared.NewJobStore[ScanJob]()
	dbPool *pgxpool.Pool
)

// SetDBPool sets the database connection pool for organizer.
func SetDBPool(pool *pgxpool.Pool) {
	dbPool = pool
}

// StartCleanup starts the background cleanup goroutine for expired organizer jobs.
func StartCleanup(ctx context.Context, ttl time.Duration) {
	store.StartCleanup(ctx, ttl, func(j *ScanJob) time.Time {
		return j.DoneAt
	})
}

// StartPlanJob starts a new planning job.
func StartPlanJob(parentCtx context.Context, pool *pgxpool.Pool, folder, mode, template, destination, operation string, startSeq int) string {
	if pool == nil {
		pool = dbPool
	}

	job := &ScanJob{
		Job:         shared.NewJob(),
		Folder:      folder,
		Phase:       "scanning",
		Mode:        mode,
		Template:    template,
		Destination: destination,
		Operation:   operation,
		StartSeq:    startSeq,
		Pool:        pool,
	}
	job.Status = shared.JobStatusRunning

	store.Set(job.ID, job)

	if pool != nil {
		_ = database.CreateJob(parentCtx, pool, database.JobRecord{
			ID:        job.ID,
			Type:      "organizer",
			Status:    string(job.Status),
			Progress:  job.Progress,
			Message:   "Scanning files...",
			StartedAt: job.StartedAt,
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	store.RegisterCancel(job.ID, cancel)

	go runPlan(ctx, job)

	return job.ID
}

// GetJob retrieves a job by ID.
func GetJob(id string) (*ScanJob, bool) {
	return store.Get(id)
}

// CancelJob cancels a job by ID.
func CancelJob(id string) bool {
	return store.Cancel(id)
}

// ExecuteJob starts the execution phase of a planned job.
func ExecuteJob(parentCtx context.Context, id string) bool {
	job, ok := store.Get(id)
	if !ok || job.Phase != "planned" {
		return false
	}

	job.mu.Lock()
	job.Phase = "executing"
	job.Progress = 0
	job.Processed = 0
	job.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	store.RegisterCancel(job.ID, cancel)

	go runExecute(ctx, job)
	return true
}

func runPlan(ctx context.Context, job *ScanJob) {
	defer func() {
		if r := recover(); r != nil {
			job.mu.Lock()
			job.Status = shared.JobStatusFailed
			job.Error = fmt.Sprintf("%v", r)
			job.DoneAt = time.Now()
			job.mu.Unlock()

			if job.Pool != nil {
				_ = database.UpdateJobStatus(context.Background(), job.Pool, job.ID, "failed", job.Progress, "Planning failed", job.Error, nil)
			}
			events.DefaultBus.Publish("organizer", "job_failed", planResponse(job))
		}
	}()

	files, err := shared.FindAllMediaFiles(job.Folder)
	if err != nil {
		slog.Error("organizer: finding media files failed", "folder", job.Folder, "err", err)
		job.mu.Lock()
		job.Status = shared.JobStatusFailed
		job.Error = err.Error()
		job.DoneAt = time.Now()
		job.mu.Unlock()
		return
	}

	select {
	case <-ctx.Done():
		markCancelled(job)
		return
	default:
	}

	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}

	job.mu.Lock()
	job.Files = paths
	job.Total = len(paths)
	job.Phase = "planning"
	job.Progress = 0.3
	job.mu.Unlock()
	publishProgress(job)

	switch job.Mode {
	case "sort":
		dest := job.Destination
		if dest == "" {
			dest = job.Folder
		}
		plan := PlanSort(paths, job.Template, dest)
		job.mu.Lock()
		job.SortPlan = plan
		job.mu.Unlock()
	case "rename":
		plan := PlanRename(paths, job.Template, job.Operation, job.StartSeq)
		job.mu.Lock()
		job.RenamePlan = plan
		job.mu.Unlock()
	}

	select {
	case <-ctx.Done():
		markCancelled(job)
		return
	default:
	}

	job.mu.Lock()
	job.Phase = "planned"
	job.Progress = 1.0
	job.Message = "Plan ready for review"
	job.mu.Unlock()

	publishProgress(job)
}

func runExecute(ctx context.Context, job *ScanJob) {
	defer func() {
		if r := recover(); r != nil {
			job.mu.Lock()
			job.Status = shared.JobStatusFailed
			job.Error = fmt.Sprintf("%v", r)
			job.DoneAt = time.Now()
			job.mu.Unlock()

			if job.Pool != nil {
				_ = database.UpdateJobStatus(context.Background(), job.Pool, job.ID, "failed", job.Progress, "Execution failed", job.Error, nil)
			}
			events.DefaultBus.Publish("organizer", "job_failed", planResponse(job))
		}
	}()

	select {
	case <-ctx.Done():
		markCancelled(job)
		return
	default:
	}

	switch job.Mode {
	case "sort":
		ExecuteSort(ctx, job.SortPlan, job.Operation)
		job.mu.Lock()
		for _, e := range job.SortPlan {
			if e.Status == "done" {
				job.Processed++
			}
		}
		job.mu.Unlock()
	case "rename":
		ExecuteRename(ctx, job.RenamePlan, job.Operation)
		job.mu.Lock()
		for _, e := range job.RenamePlan {
			if e.Status == "done" {
				job.Processed++
			}
		}
		job.mu.Unlock()
	}

	job.mu.Lock()
	if ctx.Err() != nil {
		job.Phase = "cancelled"
		job.Status = shared.JobStatusCancelled
		job.Message = "Execution was interrupted"
	} else {
		job.Phase = "done"
		job.Status = shared.JobStatusDone
		job.Progress = 1.0
		job.Message = "Execution completed"
	}
	job.DoneAt = time.Now()
	job.mu.Unlock()

	if job.Pool != nil {
		_ = database.UpdateJobStatus(context.Background(), job.Pool, job.ID, string(job.Status), job.Progress, job.Message, job.Error, map[string]interface{}{
			"execution": GetExecutionResult(job),
		})
	}

	events.DefaultBus.Publish("organizer", "job_done", planResponse(job))
}

func markCancelled(job *ScanJob) {
	job.mu.Lock()
	job.Status = shared.JobStatusCancelled
	job.DoneAt = time.Now()
	job.Message = "Execution was interrupted"
	job.mu.Unlock()

	if job.Pool != nil {
		_ = database.UpdateJobStatus(context.Background(), job.Pool, job.ID, "cancelled", job.Progress, job.Message, "", nil)
	}
	events.DefaultBus.Publish("organizer", "job_cancelled", planResponse(job))
}

func publishProgress(job *ScanJob) {
	events.DefaultBus.Publish("organizer", "progress", planResponse(job))
}

func planResponse(job *ScanJob) map[string]interface{} {
	job.mu.Lock()
	defer job.mu.Unlock()

	resp := map[string]interface{}{
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
		plan := make([]UnifiedPlanEntry, 0)
		conflicts := 0
		if job.Mode == "sort" {
			for _, e := range job.SortPlan {
				isConflict := e.Status == "conflict"
				if isConflict {
					conflicts++
				}
				plan = append(plan, UnifiedPlanEntry{
					Source:      e.Source,
					Destination: e.Destination,
					Conflict:    isConflict,
				})
			}
		} else {
			for _, e := range job.RenamePlan {
				isConflict := e.Status == "conflict"
				if isConflict {
					conflicts++
				}
				plan = append(plan, UnifiedPlanEntry{
					Source:      e.Source,
					Destination: e.Destination,
					Conflict:    isConflict,
				})
			}
		}
		resp["plan"] = plan
		resp["plan_count"] = len(plan)
		resp["conflicts"] = conflicts
	}

	if job.Phase == "done" {
		var res ExecutionResult
		if job.Mode == "sort" {
			for _, e := range job.SortPlan {
				switch e.Status {
				case "done":
					res.Completed++
				case "error":
					res.Errors++
				case "conflict", "skipped":
					res.Skipped++
				}
			}
		} else {
			for _, e := range job.RenamePlan {
				switch e.Status {
				case "done":
					res.Completed++
				case "error":
					res.Errors++
				case "conflict", "skipped":
					res.Skipped++
				}
			}
		}
		resp["execution"] = res
	}

	return resp
}

// GetUnifiedPlan returns the plan entries in a unified typed format.
func GetUnifiedPlan(job *ScanJob) []UnifiedPlanEntry {
	job.mu.Lock()
	defer job.mu.Unlock()

	if job.Mode == "sort" {
		plan := make([]UnifiedPlanEntry, len(job.SortPlan))
		for i, e := range job.SortPlan {
			plan[i] = UnifiedPlanEntry{
				Source:      e.Source,
				Destination: e.Destination,
				Conflict:    e.Status == "conflict",
			}
		}
		return plan
	}

	plan := make([]UnifiedPlanEntry, len(job.RenamePlan))
	for i, e := range job.RenamePlan {
		plan[i] = UnifiedPlanEntry{
			Source:      e.Source,
			Destination: e.Destination,
			Conflict:    e.Status == "conflict",
		}
	}
	return plan
}

// GetExecutionResult returns execution stats.
func GetExecutionResult(job *ScanJob) ExecutionResult {
	job.mu.Lock()
	defer job.mu.Unlock()

	var res ExecutionResult
	if job.Mode == "sort" {
		for _, e := range job.SortPlan {
			switch e.Status {
			case "done":
				res.Completed++
			case "error":
				res.Errors++
			case "conflict", "skipped":
				res.Skipped++
			}
		}
	} else {
		for _, e := range job.RenamePlan {
			switch e.Status {
			case "done":
				res.Completed++
			case "error":
				res.Errors++
			case "conflict", "skipped":
				res.Skipped++
			}
		}
	}

	return res
}
