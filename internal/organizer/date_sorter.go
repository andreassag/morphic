package organizer

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	exif "github.com/dsoprea/go-exif/v3"
)

// SortPlanEntry is a single planned file move/copy.
type SortPlanEntry struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
}

// GetFileDate extracts the date from EXIF data or falls back to modification time.
func GetFileDate(path string) time.Time {
	if t, err := getExifDate(path); err == nil {
		return t
	}

	info, err := os.Stat(path)
	if err != nil {
		return time.Now()
	}
	return info.ModTime()
}

func getExifDate(path string) (time.Time, error) {
	rawExif, err := exif.SearchFileAndExtractExif(path)
	if err != nil {
		return time.Time{}, err
	}

	entries, _, err := exif.GetFlatExifData(rawExif, nil)
	if err != nil {
		return time.Time{}, err
	}

	// Look for DateTimeOriginal (0x9003) or DateTime (0x0132)
	for _, entry := range entries {
		if entry.TagId == 0x9003 || entry.TagId == 0x0132 {
			if s, ok := entry.Value.(string); ok {
				s = strings.TrimRight(strings.TrimSpace(s), "\x00")
				if s != "" && s != "0000:00:00 00:00:00" {
					t, err := time.Parse("2006:01:02 15:04:05", s)
					if err == nil {
						return t, nil
					}
				}
			}
		}
	}

	return time.Time{}, fmt.Errorf("no date tag found")
}

// RenderTemplate replaces date tokens in a template string.
func RenderTemplate(tmpl string, date time.Time) string {
	r := strings.NewReplacer(
		"{year}", fmt.Sprintf("%04d", date.Year()),
		"{month}", fmt.Sprintf("%02d", date.Month()),
		"{day}", fmt.Sprintf("%02d", date.Day()),
		"{hour}", fmt.Sprintf("%02d", date.Hour()),
		"{minute}", fmt.Sprintf("%02d", date.Minute()),
	)
	return r.Replace(tmpl)
}

// PlanSort creates a plan for sorting files into date-based folders.
func PlanSort(files []string, template string, destination string) []SortPlanEntry {
	var plan []SortPlanEntry

	for _, path := range files {
		date := GetFileDate(path)
		subDir := RenderTemplate(template, date)
		destDir := filepath.Join(destination, subDir)
		destPath := filepath.Join(destDir, filepath.Base(path))

		plan = append(plan, SortPlanEntry{
			Source:      path,
			Destination: destPath,
			Status:      "pending",
		})
	}

	return plan
}

// ExecuteSort executes the sort plan using the given operation (move or copy) with context cancellation.
func ExecuteSort(ctx context.Context, plan []SortPlanEntry, operation string) {
	for i := range plan {
		if ctx.Err() != nil {
			return
		}

		destDir := filepath.Dir(plan[i].Destination)
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			plan[i].Status = "error"
			plan[i].Error = err.Error()
			continue
		}

		var err error
		switch operation {
		case "move":
			err = os.Rename(plan[i].Source, plan[i].Destination)
			if err != nil {
				// Cross-device move: copy + remove
				err = copyFile(plan[i].Source, plan[i].Destination)
				if err == nil {
					if rmErr := os.Remove(plan[i].Source); rmErr != nil {
						slog.Warn("organizer: failed to remove source after cross-device copy", "src", plan[i].Source, "err", rmErr)
					}
				}
			}
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

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}

	// Preserve modification time
	info, err := os.Stat(src)
	if err == nil {
		if err := os.Chtimes(dst, info.ModTime(), info.ModTime()); err != nil {
			// Intentionally log and continue if mtime update fails
			slog.Debug("organizer: chtimes failed", "dst", dst, "err", err)
		}
	}

	return out.Close()
}
