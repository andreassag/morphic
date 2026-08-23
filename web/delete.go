package web

import (
	"os"

	"github.com/exterex/morphic/internal/shared"
)

// DeleteResult describes the deletion outcome for a single file.
type DeleteResult struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	SizeFreed int64  `json:"size_freed,omitempty"`
}

// DeleteFilesResponse contains the aggregate results of a bulk file deletion.
type DeleteFilesResponse struct {
	Results             []DeleteResult `json:"results"`
	TotalFreed          int64          `json:"total_freed"`
	TotalFreedFormatted string         `json:"total_freed_formatted"`
}

// executeDeleteFiles executes file deletions across a list of file paths.
func executeDeleteFiles(files []string) DeleteFilesResponse {
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
		size := info.Size()
		if err := os.Remove(fp); err != nil {
			if os.IsPermission(err) {
				results = append(results, DeleteResult{Path: fp, Status: "permission_denied"})
			} else {
				results = append(results, DeleteResult{Path: fp, Status: "error", Error: err.Error()})
			}
		} else {
			totalFreed += size
			results = append(results, DeleteResult{Path: fp, Status: "deleted", SizeFreed: size})
		}
	}

	return DeleteFilesResponse{
		Results:             results,
		TotalFreed:          totalFreed,
		TotalFreedFormatted: shared.FormatFileSize(totalFreed),
	}
}
