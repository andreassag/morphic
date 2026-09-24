package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/exterex/morphic/internal/database"
	"github.com/exterex/morphic/internal/events"
	"github.com/exterex/morphic/internal/shared"
	"github.com/exterex/morphic/internal/trash"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DeleteResult describes the deletion outcome for a single file.
type DeleteResult struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	AuditID   int64  `json:"audit_id,omitempty"`
	Error     string `json:"error,omitempty"`
	SizeFreed int64  `json:"size_freed,omitempty"`
}

// DeleteFilesResponse contains the aggregate results of a bulk file deletion.
type DeleteFilesResponse struct {
	Results             []DeleteResult `json:"results"`
	TotalFreed          int64          `json:"total_freed"`
	TotalFreedFormatted string         `json:"total_freed_formatted"`
}

// executeDeleteFiles executes safe-trash file deletions across a list of file paths.
func executeDeleteFiles(ctx context.Context, pool *pgxpool.Pool, files []string, origin ...string) DeleteFilesResponse {
	var results []DeleteResult
	totalFreed := int64(0)
	deletedCount := 0

	sourceOrigin := "dupfinder"
	if len(origin) > 0 && origin[0] != "" {
		sourceOrigin = origin[0]
	}

	for _, fp := range files {
		safePath, err := shared.ValidateSafePath(fp)
		if err != nil {
			results = append(results, DeleteResult{Path: fp, Status: "not_found", Error: "invalid or protected path: " + err.Error()})
			continue
		}
		info, err := os.Stat(safePath)
		if err != nil {
			results = append(results, DeleteResult{Path: safePath, Status: "not_found", Error: "file not found"})
			continue
		}
		if info.IsDir() {
			results = append(results, DeleteResult{Path: safePath, Status: "not_found", Error: "cannot delete directory"})
			continue
		}

		auditID, _, size, err := trash.MoveToTrash(ctx, pool, safePath, sourceOrigin)
		if err != nil {
			slog.Error("failed to move file to trash", "path", fp, "err", err)
			isPerm := errors.Is(err, os.ErrPermission) || os.IsPermission(err) || strings.Contains(strings.ToLower(err.Error()), "permission denied")
			if isPerm {
				results = append(results, DeleteResult{
					Path:   fp,
					Status: "permission_denied",
					Error:  "permission denied: check folder/file write permissions",
				})
			} else {
				results = append(results, DeleteResult{
					Path:   fp,
					Status: "error",
					Error:  err.Error(),
				})
			}
		} else {
			totalFreed += size
			deletedCount++
			results = append(results, DeleteResult{
				Path:      fp,
				Status:    "deleted",
				AuditID:   auditID,
				SizeFreed: size,
			})
			events.DefaultBus.Publish("trash", "file_trashed", map[string]interface{}{
				"id":     auditID,
				"path":   fp,
				"size":   size,
				"source": sourceOrigin,
			})
		}
	}

	// Record bulk operation for audit history
	if len(files) > 0 {
		status := "completed"
		if deletedCount == 0 {
			status = "failed"
		} else if deletedCount < len(files) {
			status = "partial"
		}

		summary := fmt.Sprintf("Safe-trashed %d file(s) (freed %s)", deletedCount, shared.FormatFileSize(totalFreed))
		if deletedCount < len(files) {
			summary += fmt.Sprintf(" [%d failed]", len(files)-deletedCount)
		}

		metaBytes, _ := json.Marshal(map[string]interface{}{
			"requested": len(files),
			"deleted":   deletedCount,
			"freed":     totalFreed,
			"source":    sourceOrigin,
		})

		op := database.AuditOperation{
			Operation: "delete",
			Source:    sourceOrigin,
			Summary:   summary,
			ItemCount: deletedCount,
			TotalSize: totalFreed,
			Status:    status,
			Metadata:  metaBytes,
			CreatedAt: time.Now(),
		}

		if pool != nil {
			_, _ = database.LogOperation(ctx, pool, op)
		} else {
			_ = trash.LogStandaloneOperation(op)
		}
		events.DefaultBus.Publish("history", "operation_logged", op)
	}

	return DeleteFilesResponse{
		Results:             results,
		TotalFreed:          totalFreed,
		TotalFreedFormatted: shared.FormatFileSize(totalFreed),
	}
}
