package organizer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// RenamePlanEntry is a single planned rename.
type RenamePlanEntry struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
}

var seqPaddedRegex = regexp.MustCompile(`\{seq:(\d+)\}`)

// RenderName applies the rename template to a file.
func RenderName(tmpl string, path string, seq int) string {
	date := GetFileDate(path)
	ext := filepath.Ext(path)
	original := strings.TrimSuffix(filepath.Base(path), ext)

	result := tmpl
	result = strings.ReplaceAll(result, "{date}", date.Format("20060102"))
	result = strings.ReplaceAll(result, "{datetime}", date.Format("20060102_150405"))
	result = strings.ReplaceAll(result, "{original}", original)
	result = strings.ReplaceAll(result, "{ext}", strings.TrimPrefix(ext, "."))

	// Handle {seq:N} with zero-padded sequence
	result = seqPaddedRegex.ReplaceAllStringFunc(result, func(match string) string {
		sub := seqPaddedRegex.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		width := 0
		fmt.Sscanf(sub[1], "%d", &width)
		return fmt.Sprintf("%0*d", width, seq)
	})

	// Handle plain {seq}
	result = strings.ReplaceAll(result, "{seq}", fmt.Sprintf("%d", seq))

	return result + ext
}

type fileWithDate struct {
	Path string
	Date time.Time
}

// PlanRename creates a rename plan for the given files.
func PlanRename(files []string, tmpl string, operation string, startSeq int) []RenamePlanEntry {
	// Sort by (date, path) for consistent sequencing
	fwd := make([]fileWithDate, len(files))
	for i, f := range files {
		fwd[i] = fileWithDate{Path: f, Date: GetFileDate(f)}
	}
	sort.Slice(fwd, func(i, j int) bool {
		if fwd[i].Date.Equal(fwd[j].Date) {
			return fwd[i].Path < fwd[j].Path
		}
		return fwd[i].Date.Before(fwd[j].Date)
	})

	plan := make([]RenamePlanEntry, len(fwd))
	destSet := make(map[string]int) // destination -> first index

	for i, f := range fwd {
		newName := RenderName(tmpl, f.Path, startSeq+i)
		destPath := filepath.Join(filepath.Dir(f.Path), newName)

		entry := RenamePlanEntry{
			Source:      f.Path,
			Destination: destPath,
			Status:      "pending",
		}

		// Check for conflicts
		if prevIdx, exists := destSet[destPath]; exists {
			entry.Status = "conflict"
			entry.Error = fmt.Sprintf("conflicts with file #%d", prevIdx+1)
			plan[prevIdx].Status = "conflict"
			if plan[prevIdx].Error == "" {
				plan[prevIdx].Error = fmt.Sprintf("conflicts with file #%d", i+1)
			}
		} else {
			destSet[destPath] = i
		}

		plan[i] = entry
	}

	return plan
}

// ExecuteRename executes the rename plan with context cancellation.
func ExecuteRename(ctx context.Context, plan []RenamePlanEntry, operation string) {
	for i := range plan {
		if ctx.Err() != nil {
			return
		}

		if plan[i].Status == "conflict" {
			continue
		}

		var err error
		switch operation {
		case "move":
			err = os.Rename(plan[i].Source, plan[i].Destination)
		case "copy":
			err = copyFile(plan[i].Source, plan[i].Destination)
		default:
			err = fmt.Errorf("unknown operation: %s", operation)
		}

		if err != nil {
			plan[i].Status = "error"
			plan[i].Error = err.Error()
		} else {
			plan[i].Status = "done"
		}
	}
}
