package syncer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/andreassag/morphic/internal/shared"
	"github.com/google/uuid"
)

// SyncPlanRequest specifies directories and behavior for sync planning.
type SyncPlanRequest struct {
	SourceDir        string `json:"source_dir"`
	TargetDir        string `json:"target_dir"`
	Mode             string `json:"mode"` // "one-way" (default) or "two-way"
	DeleteExtraneous bool   `json:"delete_extraneous"`
}

// SyncAction defines an individual planned synchronization operation.
type SyncAction struct {
	Type       string `json:"type"` // "copy_new", "update_changed", "delete_extra", "conflict"
	SourcePath string `json:"source_path,omitempty"`
	TargetPath string `json:"target_path"`
	Size       int64  `json:"size"`
	Status     string `json:"status"` // "pending", "completed", "failed", "skipped"
	Error      string `json:"error,omitempty"`
}

// SyncSummary aggregates totals for the planned actions.
type SyncSummary struct {
	NewCount      int   `json:"new_count"`
	UpdatedCount  int   `json:"updated_count"`
	DeletedCount  int   `json:"deleted_count"`
	ConflictCount int   `json:"conflict_count"`
	TotalBytes    int64 `json:"total_bytes"`
}

// SyncPlan holds the generated diff actions and overall summary.
type SyncPlan struct {
	ID               string       `json:"id"`
	SourceDir        string       `json:"source_dir"`
	TargetDir        string       `json:"target_dir"`
	Mode             string       `json:"mode"`
	DeleteExtraneous bool         `json:"delete_extraneous"`
	Actions          []SyncAction `json:"actions"`
	Summary          SyncSummary  `json:"summary"`
}

// SyncerJob tracks execution progress of a sync operation.
type SyncerJob struct {
	shared.Job
	Plan *SyncPlan `json:"plan"`
}

var (
	plansMu sync.RWMutex
	plans   = make(map[string]*SyncPlan)

	jobs = shared.NewJobStore[SyncerJob]()
)

// GetPlan retrieves a stored sync plan by ID.
func GetPlan(id string) (*SyncPlan, bool) {
	plansMu.RLock()
	defer plansMu.RUnlock()
	p, ok := plans[id]
	return p, ok
}

// SetPlan stores a sync plan.
func SetPlan(p *SyncPlan) {
	plansMu.Lock()
	defer plansMu.Unlock()
	plans[p.ID] = p
}

// GetJob retrieves a syncer job by ID.
func GetJob(id string) (*SyncerJob, bool) {
	return jobs.Get(id)
}

// SetJob stores a syncer job by ID.
func SetJob(id string, job *SyncerJob) {
	jobs.Set(id, job)
}

// RegisterCancel registers a cancel func.
func RegisterCancel(id string, cancel context.CancelFunc) {
	jobs.RegisterCancel(id, cancel)
}

// CancelJob cancels a syncer job.
func CancelJob(id string) bool {
	return jobs.Cancel(id)
}

// StartCleanup periodically cleans up expired jobs.
func StartCleanup(ctx context.Context, ttl time.Duration) {
	jobs.StartCleanup(ctx, ttl, func(j *SyncerJob) time.Time {
		return j.DoneAt
	})
}

type fileEntry struct {
	fullPath string
	relPath  string
	size     int64
	modTime  time.Time
}

func scanFolder(ctx context.Context, root string) (map[string]fileEntry, error) {
	m := make(map[string]fileEntry)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = d.Name()
		}

		cleanRel := filepath.ToSlash(rel)
		m[cleanRel] = fileEntry{
			fullPath: path,
			relPath:  cleanRel,
			size:     info.Size(),
			modTime:  info.ModTime(),
		}
		return nil
	})
	return m, err
}

// PlanSync creates a diff between source and target directories without making modifications.
func PlanSync(ctx context.Context, req SyncPlanRequest) (*SyncPlan, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	safeSrc, err := shared.ValidateSafeDirPath(req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("validate source directory: %w", err)
	}

	safeTarget, err := shared.ValidateSafeDirPath(req.TargetDir)
	if err != nil {
		return nil, fmt.Errorf("validate target directory: %w", err)
	}

	mode := strings.ToLower(req.Mode)
	if mode != "two-way" {
		mode = "one-way"
	}

	srcFiles, err := scanFolder(ctx, safeSrc)
	if err != nil {
		return nil, fmt.Errorf("scan source directory: %w", err)
	}

	targetFiles, err := scanFolder(ctx, safeTarget)
	if err != nil {
		return nil, fmt.Errorf("scan target directory: %w", err)
	}

	plan := &SyncPlan{
		ID:               uuid.New().String(),
		SourceDir:        safeSrc,
		TargetDir:        safeTarget,
		Mode:             mode,
		DeleteExtraneous: req.DeleteExtraneous,
	}

	// 1. Process files present in source
	for rel, srcEntry := range srcFiles {
		targetEntry, existsInTarget := targetFiles[rel]
		dstPath := filepath.Join(safeTarget, filepath.FromSlash(rel))

		if !existsInTarget {
			plan.Actions = append(plan.Actions, SyncAction{
				Type:       "copy_new",
				SourcePath: srcEntry.fullPath,
				TargetPath: dstPath,
				Size:       srcEntry.size,
				Status:     "pending",
			})
			plan.Summary.NewCount++
			plan.Summary.TotalBytes += srcEntry.size
		} else {
			// File exists in both
			diffSize := srcEntry.size != targetEntry.size
			diffTime := srcEntry.modTime.Sub(targetEntry.modTime).Abs() > time.Second

			if diffSize || diffTime {
				if mode == "two-way" {
					if srcEntry.modTime.After(targetEntry.modTime) {
						plan.Actions = append(plan.Actions, SyncAction{
							Type:       "update_changed",
							SourcePath: srcEntry.fullPath,
							TargetPath: dstPath,
							Size:       srcEntry.size,
							Status:     "pending",
						})
						plan.Summary.UpdatedCount++
						plan.Summary.TotalBytes += srcEntry.size
					} else if targetEntry.modTime.After(srcEntry.modTime) {
						plan.Actions = append(plan.Actions, SyncAction{
							Type:       "update_changed",
							SourcePath: targetEntry.fullPath,
							TargetPath: srcEntry.fullPath,
							Size:       targetEntry.size,
							Status:     "pending",
						})
						plan.Summary.UpdatedCount++
						plan.Summary.TotalBytes += targetEntry.size
					} else {
						plan.Actions = append(plan.Actions, SyncAction{
							Type:       "conflict",
							SourcePath: srcEntry.fullPath,
							TargetPath: dstPath,
							Size:       srcEntry.size,
							Status:     "pending",
						})
						plan.Summary.ConflictCount++
					}
				} else {
					// one-way: source always replaces target
					plan.Actions = append(plan.Actions, SyncAction{
						Type:       "update_changed",
						SourcePath: srcEntry.fullPath,
						TargetPath: dstPath,
						Size:       srcEntry.size,
						Status:     "pending",
					})
					plan.Summary.UpdatedCount++
					plan.Summary.TotalBytes += srcEntry.size
				}
			}
		}
	}

	// 2. Process extraneous files in target (only for one-way with DeleteExtraneous, or two-way)
	for rel, targetEntry := range targetFiles {
		if _, existsInSrc := srcFiles[rel]; !existsInSrc {
			if mode == "two-way" {
				// In two-way, copy file to source
				srcPath := filepath.Join(safeSrc, filepath.FromSlash(rel))
				plan.Actions = append(plan.Actions, SyncAction{
					Type:       "copy_new",
					SourcePath: targetEntry.fullPath,
					TargetPath: srcPath,
					Size:       targetEntry.size,
					Status:     "pending",
				})
				plan.Summary.NewCount++
				plan.Summary.TotalBytes += targetEntry.size
			} else if req.DeleteExtraneous {
				plan.Actions = append(plan.Actions, SyncAction{
					Type:       "delete_extra",
					TargetPath: targetEntry.fullPath,
					Size:       targetEntry.size,
					Status:     "pending",
				})
				plan.Summary.DeletedCount++
			}
		}
	}

	SetPlan(plan)
	return plan, nil
}

// ExecuteSync performs the operations specified in a planned sync.
func ExecuteSync(ctx context.Context, plan *SyncPlan, onProgress func(done, total int)) error {
	total := len(plan.Actions)
	if total == 0 {
		return nil
	}

	for i := range plan.Actions {
		select {
		case <-ctx.Done():
			plan.Actions[i].Status = "cancelled"
			return ctx.Err()
		default:
		}

		action := &plan.Actions[i]
		switch action.Type {
		case "copy_new", "update_changed":
			if err := copySyncFile(ctx, action.SourcePath, action.TargetPath); err != nil {
				action.Status = "failed"
				action.Error = err.Error()
			} else {
				action.Status = "completed"
			}
		case "delete_extra":
			if err := os.Remove(action.TargetPath); err != nil {
				action.Status = "failed"
				action.Error = err.Error()
			} else {
				action.Status = "completed"
			}
		case "conflict":
			action.Status = "skipped"
		}

		if onProgress != nil {
			onProgress(i+1, total)
		}
	}

	return nil
}

func copySyncFile(ctx context.Context, src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	srcInfo, err := in.Stat()
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open dest: %w", err)
	}
	defer out.Close()

	buf := make([]byte, 64*1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, rErr := in.Read(buf)
		if n > 0 {
			if _, wErr := out.Write(buf[:n]); wErr != nil {
				return fmt.Errorf("write stream: %w", wErr)
			}
		}
		if rErr == io.EOF {
			break
		}
		if rErr != nil {
			return fmt.Errorf("read stream: %w", rErr)
		}
	}

	_ = out.Close()
	// Preserve modification time
	_ = os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime())

	return nil
}
