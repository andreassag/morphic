package shared

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// JobStatus represents the state of a background job.
type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusDone      JobStatus = "done"
	JobStatusFailed    JobStatus = "failed"
	JobStatusPlanned   JobStatus = "planned"
	JobStatusCancelled JobStatus = "cancelled"
)

// Job is a base type embedded in module-specific jobs.
// Notice: context.Context is intentionally NOT stored here to adhere to Go guidelines.
// Context is threaded through call chains and cancellation is managed via JobStore.
type Job struct {
	ID        string    `json:"id"`
	Status    JobStatus `json:"status"`
	Progress  float64   `json:"progress"`
	Message   string    `json:"message,omitempty"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
	DoneAt    time.Time `json:"done_at,omitempty"`
}

// NewJob creates a new job with a unique ID and initial status.
func NewJob() Job {
	return Job{
		ID:        uuid.New().String(),
		Status:    JobStatusPending,
		Progress:  0,
		StartedAt: time.Now(),
	}
}

// JobStore is a thread-safe generic store for background jobs and their cancellation functions.
type JobStore[T any] struct {
	mu      sync.RWMutex
	jobs    map[string]*T
	cancels map[string]context.CancelFunc
}

// NewJobStore creates a new JobStore.
func NewJobStore[T any]() *JobStore[T] {
	return &JobStore[T]{
		jobs:    make(map[string]*T),
		cancels: make(map[string]context.CancelFunc),
	}
}

// Set stores a job.
func (s *JobStore[T]) Set(id string, job *T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[id] = job
}

// RegisterCancel associates a cancellation function with a job.
func (s *JobStore[T]) RegisterCancel(id string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cancels[id] = cancel
}

// Cancel invokes the cancel function associated with a job ID, if registered.
func (s *JobStore[T]) Cancel(id string) bool {
	s.mu.Lock()
	cancel, ok := s.cancels[id]
	if ok {
		delete(s.cancels, id)
	}
	s.mu.Unlock()

	if ok && cancel != nil {
		cancel()
		return true
	}
	return false
}

// Get retrieves a job by ID.
func (s *JobStore[T]) Get(id string) (*T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	return job, ok
}

// Delete removes a job and its cancellation function by ID.
func (s *JobStore[T]) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
	delete(s.cancels, id)
}

// StartCleanup runs a background goroutine that removes jobs older than ttl.
// It terminates when the provided context is cancelled.
func (s *JobStore[T]) StartCleanup(ctx context.Context, ttl time.Duration, getDoneAt func(*T) time.Time) {
	go func() {
		ticker := time.NewTicker(ttl / 2)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				now := time.Now()
				s.mu.Lock()
				for id, job := range s.jobs {
					doneAt := getDoneAt(job)
					if !doneAt.IsZero() && now.Sub(doneAt) > ttl {
						delete(s.jobs, id)
						delete(s.cancels, id)
					}
				}
				s.mu.Unlock()
			}
		}
	}()
}
