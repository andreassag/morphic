package web

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"

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

	sourceOrigin := "dupfinder"
	if len(origin) > 0 && origin[0] != "" {
		sourceOrigin = origin[0]
	}

	for _, fp := range files {
		if !isAbsPath(fp) {
			results = append(results, DeleteResult{Path: fp, Status: "not_found", Error: "path must be absolute"})
			continue
		}
		info, err := os.Stat(fp)
		if err != nil {
			results = append(results, DeleteResult{Path: fp, Status: "not_found", Error: "file not found"})
			continue
		}
		if info.IsDir() {
			results = append(results, DeleteResult{Path: fp, Status: "not_found", Error: "cannot delete directory"})
			continue
		}

		auditID, _, size, err := trash.MoveToTrash(ctx, pool, fp, sourceOrigin)
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

	return DeleteFilesResponse{
		Results:             results,
		TotalFreed:          totalFreed,
		TotalFreedFormatted: shared.FormatFileSize(totalFreed),
	}
}
