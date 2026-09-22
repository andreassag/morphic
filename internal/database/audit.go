package database

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AuditEntry represents an action logged in the audit_log table.
type AuditEntry struct {
	ID              int64           `json:"id"`
	Operation       string          `json:"operation"` // 'delete', 'convert', 'rename', 'sort'
	Source          string          `json:"source,omitempty"`
	OriginalPath    string          `json:"original_path"`
	DestinationPath string          `json:"destination_path,omitempty"`
	TrashPath       string          `json:"trash_path,omitempty"`
	FileSize        int64           `json:"file_size"`
	Metadata        json.RawMessage `json:"metadata"`
	Reversible      bool            `json:"reversible"`
	ReversedAt      *time.Time      `json:"reversed_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
}

// LogAction logs an action (e.g. deletion to trash, rename, conversion) in the audit_log.
func LogAction(ctx context.Context, pool *pgxpool.Pool, entry AuditEntry) (int64, error) {
	if pool == nil {
		return 0, nil
	}

	metadataBytes := entry.Metadata
	if len(metadataBytes) == 0 {
		metadataBytes = []byte("{}")
	}

	query := `
		INSERT INTO audit_log (operation, original_path, destination_path, trash_path, file_size, metadata, reversible, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		RETURNING id;
	`

	var id int64
	err := pool.QueryRow(ctx, query,
		entry.Operation,
		entry.OriginalPath,
		entry.DestinationPath,
		entry.TrashPath,
		entry.FileSize,
		metadataBytes,
		entry.Reversible,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("logging audit action %s on %s: %w", entry.Operation, entry.OriginalPath, err)
	}
	return id, nil
}

// GetAuditEntry retrieves a single audit entry by ID.
func GetAuditEntry(ctx context.Context, pool *pgxpool.Pool, id int64) (*AuditEntry, error) {
	if pool == nil {
		return nil, nil
	}

	query := `
		SELECT id, operation, original_path, COALESCE(destination_path, ''), COALESCE(trash_path, ''), COALESCE(file_size, 0), metadata, reversible, reversed_at, created_at
		FROM audit_log
		WHERE id = $1
		LIMIT 1;
	`

	var e AuditEntry
	err := pool.QueryRow(ctx, query, id).Scan(
		&e.ID,
		&e.Operation,
		&e.OriginalPath,
		&e.DestinationPath,
		&e.TrashPath,
		&e.FileSize,
		&e.Metadata,
		&e.Reversible,
		&e.ReversedAt,
		&e.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("getting audit entry %d: %w", id, err)
	}
	e.Source = extractSource(e.Metadata, e.Operation)
	return &e, nil
}

// MarkActionReversed sets reversed_at timestamp on an audit log item.
func MarkActionReversed(ctx context.Context, pool *pgxpool.Pool, id int64) error {
	if pool == nil {
		return nil
	}

	query := `
		UPDATE audit_log
		SET reversed_at = NOW()
		WHERE id = $1;
	`

	_, err := pool.Exec(ctx, query, id)
	return err
}

// ListAuditHistory lists recent audit events with optional operation filtering and pagination.
func ListAuditHistory(ctx context.Context, pool *pgxpool.Pool, operation string, limit, offset int) ([]AuditEntry, int64, error) {
	if pool == nil {
		return nil, 0, nil
	}

	if limit <= 0 {
		limit = 50
	}

	var countQuery string
	var query string
	var rows pgx.Rows
	var err error
	var total int64

	if operation != "" {
		countQuery = `SELECT COUNT(*) FROM audit_log WHERE operation = $1;`
		if err := pool.QueryRow(ctx, countQuery, operation).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("counting audit log: %w", err)
		}

		query = `
			SELECT id, operation, original_path, COALESCE(destination_path, ''), COALESCE(trash_path, ''), COALESCE(file_size, 0), metadata, reversible, reversed_at, created_at
			FROM audit_log
			WHERE operation = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3;
		`
		rows, err = pool.Query(ctx, query, operation, limit, offset)
	} else {
		countQuery = `SELECT COUNT(*) FROM audit_log;`
		if err := pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("counting audit log: %w", err)
		}

		query = `
			SELECT id, operation, original_path, COALESCE(destination_path, ''), COALESCE(trash_path, ''), COALESCE(file_size, 0), metadata, reversible, reversed_at, created_at
			FROM audit_log
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2;
		`
		rows, err = pool.Query(ctx, query, limit, offset)
	}

	if err != nil {
		return nil, 0, fmt.Errorf("querying audit history: %w", err)
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(
			&e.ID,
			&e.Operation,
			&e.OriginalPath,
			&e.DestinationPath,
			&e.TrashPath,
			&e.FileSize,
			&e.Metadata,
			&e.Reversible,
			&e.ReversedAt,
			&e.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning audit entry: %w", err)
		}
		e.Source = extractSource(e.Metadata, e.Operation)
		entries = append(entries, e)
	}

	return entries, total, nil
}

func extractSource(metadata json.RawMessage, operation string) string {
	if len(metadata) > 0 {
		var m struct {
			Source string `json:"source"`
		}
		if err := json.Unmarshal(metadata, &m); err == nil && m.Source != "" {
			return m.Source
		}
	}
	switch operation {
	case "delete":
		return "dupfinder"
	case "convert":
		return "converter"
	case "rename", "sort":
		return "organizer"
	default:
		return "manual"
	}
}

// AuditOperation represents a high-level bulk operation record (convert, delete, sort, rename, etc.).
type AuditOperation struct {
	ID        int64           `json:"id"`
	Operation string          `json:"operation"` // 'convert', 'delete', 'sort', 'rename', 'restore', 'purge'
	Source    string          `json:"source"`    // 'converter', 'dupfinder', 'organizer', 'trash'
	Summary   string          `json:"summary"`
	ItemCount int             `json:"item_count"`
	TotalSize int64           `json:"total_size"`
	Status    string          `json:"status"` // 'completed', 'partial', 'failed'
	Metadata  json.RawMessage `json:"metadata"`
	CreatedAt time.Time       `json:"created_at"`
}

// LogOperation inserts a bulk operation record into audit_operations.
func LogOperation(ctx context.Context, pool *pgxpool.Pool, op AuditOperation) (int64, error) {
	if pool == nil {
		return 0, nil
	}

	metadataBytes := op.Metadata
	if len(metadataBytes) == 0 {
		metadataBytes = []byte("{}")
	}
	if op.Status == "" {
		op.Status = "completed"
	}
	if op.ItemCount <= 0 {
		op.ItemCount = 1
	}

	query := `
		INSERT INTO audit_operations (operation, source, summary, item_count, total_size, status, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		RETURNING id;
	`

	var id int64
	err := pool.QueryRow(ctx, query,
		op.Operation,
		op.Source,
		op.Summary,
		op.ItemCount,
		op.TotalSize,
		op.Status,
		metadataBytes,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("logging audit operation %s: %w", op.Operation, err)
	}
	return id, nil
}

// ListAuditOperations retrieves bulk operation logs with optional operation filtering and pagination.
func ListAuditOperations(ctx context.Context, pool *pgxpool.Pool, operation string, limit, offset int) ([]AuditOperation, int64, error) {
	if pool == nil {
		return nil, 0, nil
	}

	if limit <= 0 {
		limit = 50
	}

	var countQuery string
	var query string
	var rows pgx.Rows
	var err error
	var total int64

	if operation != "" {
		countQuery = `SELECT COUNT(*) FROM audit_operations WHERE operation = $1;`
		if err := pool.QueryRow(ctx, countQuery, operation).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("counting audit operations: %w", err)
		}

		query = `
			SELECT id, operation, source, summary, item_count, total_size, status, metadata, created_at
			FROM audit_operations
			WHERE operation = $1
			ORDER BY created_at DESC
			LIMIT $2 OFFSET $3;
		`
		rows, err = pool.Query(ctx, query, operation, limit, offset)
	} else {
		countQuery = `SELECT COUNT(*) FROM audit_operations;`
		if err := pool.QueryRow(ctx, countQuery).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("counting audit operations: %w", err)
		}

		query = `
			SELECT id, operation, source, summary, item_count, total_size, status, metadata, created_at
			FROM audit_operations
			ORDER BY created_at DESC
			LIMIT $1 OFFSET $2;
		`
		rows, err = pool.Query(ctx, query, limit, offset)
	}

	if err != nil {
		return nil, 0, fmt.Errorf("querying audit operations: %w", err)
	}
	defer rows.Close()

	var ops []AuditOperation
	for rows.Next() {
		var o AuditOperation
		if err := rows.Scan(
			&o.ID,
			&o.Operation,
			&o.Source,
			&o.Summary,
			&o.ItemCount,
			&o.TotalSize,
			&o.Status,
			&o.Metadata,
			&o.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning audit operation: %w", err)
		}
		ops = append(ops, o)
	}

	return ops, total, nil
}
