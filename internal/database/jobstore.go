package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// JobRecord represents a persistent job entry in PostgreSQL.
type JobRecord struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Status    string          `json:"status"`
	Progress  float64         `json:"progress"`
	Message   string          `json:"message,omitempty"`
	Error     string          `json:"error,omitempty"`
	Payload   json.RawMessage `json:"payload"`
	Result    json.RawMessage `json:"result,omitempty"`
	StartedAt time.Time       `json:"started_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	DoneAt    *time.Time      `json:"done_at,omitempty"`
}

// CreateJob records a new job in the database.
func CreateJob(ctx context.Context, pool *pgxpool.Pool, job JobRecord) error {
	if pool == nil {
		return nil
	}

	payloadBytes := job.Payload
	if len(payloadBytes) == 0 {
		payloadBytes = []byte("{}")
	}
	resultBytes := job.Result
	if len(resultBytes) == 0 {
		resultBytes = []byte("{}")
	}

	query := `
		INSERT INTO jobs (id, type, status, progress, message, error, payload, result, started_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW());
	`

	jobID, err := uuid.Parse(job.ID)
	if err != nil {
		jobID = uuid.New()
	}

	_, err = pool.Exec(ctx, query,
		jobID,
		job.Type,
		job.Status,
		job.Progress,
		job.Message,
		job.Error,
		payloadBytes,
		resultBytes,
		job.StartedAt,
	)
	if err != nil {
		return fmt.Errorf("creating job %s: %w", job.ID, err)
	}
	return nil
}

// UpdateJobProgress updates the live progress and status message of a job.
func UpdateJobProgress(ctx context.Context, pool *pgxpool.Pool, id string, progress float64, message string) error {
	if pool == nil {
		return nil
	}

	jobID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid job id %s: %w", id, err)
	}

	query := `
		UPDATE jobs
		SET progress = $1, message = $2, updated_at = NOW()
		WHERE id = $3;
	`

	_, err = pool.Exec(ctx, query, progress, message, jobID)
	if err != nil {
		return fmt.Errorf("updating job progress for %s: %w", id, err)
	}
	return nil
}

// UpdateJobStatus updates the status, progress, message, and outcome result of a job.
func UpdateJobStatus(ctx context.Context, pool *pgxpool.Pool, id string, status string, progress float64, message string, errorMsg string, result interface{}) error {
	if pool == nil {
		return nil
	}

	jobID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid job id %s: %w", id, err)
	}

	var resultBytes []byte
	if result != nil {
		resultBytes, err = json.Marshal(result)
		if err != nil {
			return fmt.Errorf("marshaling job result: %w", err)
		}
	} else {
		resultBytes = []byte("{}")
	}

	var query string
	if status == "done" || status == "failed" || status == "cancelled" {
		query = `
			UPDATE jobs
			SET status = $1, progress = $2, message = $3, error = $4, result = $5, updated_at = NOW(), done_at = NOW()
			WHERE id = $6;
		`
	} else {
		query = `
			UPDATE jobs
			SET status = $1, progress = $2, message = $3, error = $4, result = $5, updated_at = NOW()
			WHERE id = $6;
		`
	}

	_, err = pool.Exec(ctx, query, status, progress, message, errorMsg, resultBytes, jobID)
	if err != nil {
		return fmt.Errorf("updating job status for %s: %w", id, err)
	}
	return nil
}

// GetJob retrieves a job record by UUID string.
func GetJob(ctx context.Context, pool *pgxpool.Pool, id string) (*JobRecord, error) {
	if pool == nil {
		return nil, nil
	}

	jobID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid job id %s: %w", id, err)
	}

	query := `
		SELECT id, type, status, progress, message, error, payload, result, started_at, updated_at, done_at
		FROM jobs
		WHERE id = $1
		LIMIT 1;
	`

	var r JobRecord
	var idVal uuid.UUID

	err = pool.QueryRow(ctx, query, jobID).Scan(
		&idVal,
		&r.Type,
		&r.Status,
		&r.Progress,
		&r.Message,
		&r.Error,
		&r.Payload,
		&r.Result,
		&r.StartedAt,
		&r.UpdatedAt,
		&r.DoneAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting job %s: %w", id, err)
	}

	r.ID = idVal.String()
	return &r, nil
}

// ListUnfinishedJobs returns jobs that were in 'running' or 'pending' state (e.g. before server restart).
func ListUnfinishedJobs(ctx context.Context, pool *pgxpool.Pool) ([]JobRecord, error) {
	if pool == nil {
		return nil, nil
	}

	query := `
		SELECT id, type, status, progress, message, error, payload, result, started_at, updated_at, done_at
		FROM jobs
		WHERE status IN ('running', 'pending')
		ORDER BY updated_at ASC;
	`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying unfinished jobs: %w", err)
	}
	defer rows.Close()

	var jobs []JobRecord
	for rows.Next() {
		var r JobRecord
		var idVal uuid.UUID
		if err := rows.Scan(
			&idVal,
			&r.Type,
			&r.Status,
			&r.Progress,
			&r.Message,
			&r.Error,
			&r.Payload,
			&r.Result,
			&r.StartedAt,
			&r.UpdatedAt,
			&r.DoneAt,
		); err != nil {
			return nil, fmt.Errorf("scanning unfinished job: %w", err)
		}
		r.ID = idVal.String()
		jobs = append(jobs, r)
	}
	return jobs, nil
}
