package web

import (
	"context"
	"os"

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
func executeDeleteFiles(ctx context.Context, pool *pgxpool.Pool, files []string) DeleteFilesResponse {
	var results []DeleteResult
	totalFreed := int64(0)

	for _, fp := range files {
		if !isAbsPath(fp) {
			results = append(results, DeleteResult{Path: fp, Status: "not_found"})
			continue
		}
		info, err := os.Stat(fp)
		if err != nil {
			results = append(results, DeleteResult{Path: fp, Status: "not_found"})
			continue
		}
		if info.IsDir() {
			results = append(results, DeleteResult{Path: fp, Status: "not_found"})
			continue
		}

		auditID, _, size, err := trash.MoveToTrash(ctx, pool, fp)
		if err != nil {
			if os.IsPermission(err) {
				results = append(results, DeleteResult{Path: fp, Status: "permission_denied"})
			} else {
				results = append(results, DeleteResult{Path: fp, Status: "error", Error: err.Error()})
			}
		} else {
			totalFreed += size
			results = append(results, DeleteResult{
				Path:      fp,
				Status:    "deleted",
				AuditID:   auditID,
				SizeFreed: size,
			})
		}
	}

	return DeleteFilesResponse{
		Results:             results,
		TotalFreed:          totalFreed,
		TotalFreedFormatted: shared.FormatFileSize(totalFreed),
	}
}
