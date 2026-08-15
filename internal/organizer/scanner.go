package organizer

import (
	"sync"
	"time"

	"github.com/andreassag/morphic/internal/shared"
)

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
}

var store = shared.NewJobStore[ScanJob]()

func init() {
	store.StartCleanup(30*time.Minute, func(j *ScanJob) time.Time {
		return j.DoneAt
	})
}

// StartPlanJob starts a new planning job.
func StartPlanJob(folder, mode, template, destination, operation string, startSeq int) string {
	job := &ScanJob{
		Job:         shared.NewJob(),
		Folder:      folder,
		Phase:       "scanning",
		Mode:        mode,
		Template:    template,
		Destination: destination,
		Operation:   operation,
		StartSeq:    startSeq,
	}
	job.Status = shared.JobStatusRunning

	store.Set(job.ID, job)

	go runPlan(job)

	return job.ID
}

// GetJob retrieves a job by ID.
func GetJob(id string) (*ScanJob, bool) {
	return store.Get(id)
}

// ExecuteJob starts the execution phase of a planned job.
func ExecuteJob(id string) bool {
	job, ok := store.Get(id)
	if !ok || job.Phase != "planned" {
		return false
	}

	job.mu.Lock()
	job.Phase = "executing"
	job.Progress = 0
	job.Processed = 0
	job.mu.Unlock()

	go runExecute(job)
	return true
}

func runPlan(job *ScanJob) {
	files, err := shared.FindAllMediaFiles(job.Folder)

	if err != nil {
		job.mu.Lock()
		job.Status = shared.JobStatusFailed
		job.Error = err.Error()
		job.DoneAt = time.Now()
		job.mu.Unlock()
		return
	}

	// Check for cancellation after file discovery
	select {
	case <-job.Ctx().Done():
		job.mu.Lock()
		job.Status = shared.JobStatusCancelled
		job.DoneAt = time.Now()
		job.Message = "Scan was interrupted"
		job.mu.Unlock()
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

	// Check for cancellation after planning
	select {
	case <-job.Ctx().Done():
		job.mu.Lock()
		job.Status = shared.JobStatusCancelled
		job.DoneAt = time.Now()
		job.Message = "Scan was interrupted"
		job.mu.Unlock()
		return
	default:
	}

	job.mu.Lock()
	job.Phase = "planned"
	job.Progress = 1.0
	job.Message = "Plan ready for review"
	job.mu.Unlock()
}

func runExecute(job *ScanJob) {
	// Check for cancellation before starting execution
	select {
	case <-job.Ctx().Done():
		job.mu.Lock()
		job.Status = shared.JobStatusCancelled
		job.DoneAt = time.Now()
		job.Message = "Execution was interrupted"
		job.mu.Unlock()
		return
	default:
	}

	switch job.Mode {
	case "sort":
		ExecuteSort(job.SortPlan, job.Operation)
		job.mu.Lock()
		for _, e := range job.SortPlan {
			if e.Status == "done" {
				job.Processed++
			}
		}
		job.mu.Unlock()
	case "rename":
		ExecuteRename(job.RenamePlan, job.Operation)
		job.mu.Lock()
		for _, e := range job.RenamePlan {
			if e.Status == "done" {
				job.Processed++
			}
		}
		job.mu.Unlock()
	}

	job.mu.Lock()
	job.Phase = "done"
	job.Status = shared.JobStatusDone
	job.Progress = 1.0
	job.DoneAt = time.Now()
	job.mu.Unlock()
}

// GetUnifiedPlan returns the plan entries in a unified format matching the
// Python API's response (each entry has "source", "destination", "conflict").
func GetUnifiedPlan(job *ScanJob) []map[string]interface{} {
	job.mu.Lock()
	defer job.mu.Unlock()

	if job.Mode == "sort" {
		plan := make([]map[string]interface{}, len(job.SortPlan))
		for i, e := range job.SortPlan {
			entry := map[string]interface{}{
				"source":      e.Source,
				"destination": e.Destination,
			}
			if e.Status == "conflict" {
				entry["conflict"] = true
			}
			plan[i] = entry
		}
		return plan
	}

	plan := make([]map[string]interface{}, len(job.RenamePlan))
	for i, e := range job.RenamePlan {
		entry := map[string]interface{}{
			"source":      e.Source,
			"destination": e.Destination,
		}
		if e.Status == "conflict" {
			entry["conflict"] = true
		}
		plan[i] = entry
	}
	return plan
}

// GetExecutionResult returns execution stats matching the Python API format.
func GetExecutionResult(job *ScanJob) map[string]interface{} {
	job.mu.Lock()
	defer job.mu.Unlock()

	completed := 0
	errors := 0
	skipped := 0

	if job.Mode == "sort" {
		for _, e := range job.SortPlan {
			switch e.Status {
			case "done":
				completed++
			case "error":
				errors++
			case "conflict", "skipped":
				skipped++
			}
		}
	} else {
		for _, e := range job.RenamePlan {
			switch e.Status {
			case "done":
				completed++
			case "error":
				errors++
			case "conflict", "skipped":
				skipped++
			}
		}
	}

	return map[string]interface{}{
		"completed": completed,
		"errors":    errors,
		"skipped":   skipped,
	}
}
