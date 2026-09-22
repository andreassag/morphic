package trash

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/exterex/morphic/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultRetentionDays is the default retention period before trash purge.
const DefaultRetentionDays = 30

// GetTrashDir returns the root trash directory path.
func GetTrashDir() string {
	if dir := os.Getenv("MORPHIC_TRASH_DIR"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".morphic_trash"
	}
	return filepath.Join(home, ".morphic", "trash")
}

// GetRetentionDays returns the configured trash retention period in days.
func GetRetentionDays() int {
	if s := os.Getenv("TRASH_RETENTION_DAYS"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			return n
		}
	}
	return DefaultRetentionDays
}

// StandaloneMeta represents the metadata stored in meta.json for standalone mode.
type StandaloneMeta struct {
	ID           int64      `json:"id"`
	Operation    string     `json:"operation"`
	Source       string     `json:"source,omitempty"`
	OriginalPath string     `json:"original_path"`
	TrashPath    string     `json:"trash_path"`
	FileSize     int64      `json:"file_size"`
	CreatedAt    time.Time  `json:"created_at"`
	Reversible   bool       `json:"reversible"`
	ReversedAt   *time.Time `json:"reversed_at,omitempty"`
}

// MoveToTrash moves a file into the safe-trash store and logs an audit_log record.
func MoveToTrash(ctx context.Context, pool *pgxpool.Pool, filePath string, origin ...string) (int64, string, int64, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return 0, "", 0, fmt.Errorf("reading file info: %w", err)
	}
	if info.IsDir() {
		return 0, "", 0, fmt.Errorf("cannot trash directories: %s", filePath)
	}

	size := info.Size()
	trashBase := GetTrashDir()
	uniqueID := uuid.New().String()
	destDir := filepath.Join(trashBase, uniqueID)

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return 0, "", 0, fmt.Errorf("creating trash directory: %w", err)
	}

	targetPath := filepath.Join(destDir, filepath.Base(filePath))

	// Try atomic rename first
	err = os.Rename(filePath, targetPath)
	if err != nil {
		// Fallback to copy + delete across mount points / devices
		if err := copyAndDelete(filePath, targetPath); err != nil {
			return 0, "", 0, fmt.Errorf("moving file to trash: %w", err)
		}
	}

	sourceOrigin := "dupfinder"
	if len(origin) > 0 && origin[0] != "" {
		sourceOrigin = origin[0]
	}

	meta := map[string]interface{}{
		"original_filename": filepath.Base(filePath),
		"trashed_at":        time.Now().Format(time.RFC3339),
		"source":            sourceOrigin,
	}
	metaBytes, _ := json.Marshal(meta)

	auditID, err := database.LogAction(ctx, pool, database.AuditEntry{
		Operation:    "delete",
		OriginalPath: filePath,
		TrashPath:    targetPath,
		FileSize:     size,
		Metadata:     metaBytes,
		Reversible:   true,
	})
	if err != nil {
		slog.Error("failed to record audit entry for trashed file", "path", filePath, "err", err)
	}
	if auditID == 0 {
		auditID = time.Now().UnixNano()
	}

	// Always write meta.json to destDir for resilience and standalone mode
	metaFile := StandaloneMeta{
		ID:           auditID,
		Operation:    "delete",
		Source:       sourceOrigin,
		OriginalPath: filePath,
		TrashPath:    targetPath,
		FileSize:     size,
		CreatedAt:    time.Now(),
		Reversible:   true,
	}
	if mb, err := json.MarshalIndent(metaFile, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(destDir, "meta.json"), mb, 0644)
	}

	return auditID, targetPath, size, nil
}

// ListStandaloneTrash lists trashed files from local meta.json files when database is not available.
func ListStandaloneTrash(operation string, limit, offset int) ([]database.AuditEntry, int64, error) {
	trashDir := GetTrashDir()
	entries, err := os.ReadDir(trashDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, fmt.Errorf("reading trash dir: %w", err)
	}

	var all []database.AuditEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		metaPath := filepath.Join(trashDir, e.Name(), "meta.json")
		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var m StandaloneMeta
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		if operation != "" && m.Operation != operation {
			continue
		}

		src := m.Source
		if src == "" {
			if m.Operation == "delete" {
				src = "dupfinder"
			} else {
				src = m.Operation
			}
		}
		metaBytes, _ := json.Marshal(map[string]interface{}{
			"original_filename": filepath.Base(m.OriginalPath),
			"source":            src,
		})

		all = append(all, database.AuditEntry{
			ID:           m.ID,
			Operation:    m.Operation,
			Source:       src,
			OriginalPath: m.OriginalPath,
			TrashPath:    m.TrashPath,
			FileSize:     m.FileSize,
			Metadata:     metaBytes,
			Reversible:   m.Reversible,
			ReversedAt:   m.ReversedAt,
			CreatedAt:    m.CreatedAt,
		})
	}

	// Also read standalone non-trash audit entries if operation is not specifically "delete"
	if operation != "delete" {
		auditDir := filepath.Join(filepath.Dir(trashDir), "audit")
		if auditEntries, err := os.ReadDir(auditDir); err == nil {
			for _, e := range auditEntries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
					continue
				}
				data, err := os.ReadFile(filepath.Join(auditDir, e.Name()))
				if err != nil {
					continue
				}
				var item database.AuditEntry
				if err := json.Unmarshal(data, &item); err != nil {
					continue
				}
				if operation != "" && item.Operation != operation {
					continue
				}
				if item.Source == "" {
					item.Source = item.Operation
				}
				all = append(all, item)
			}
		}
	}

	// Sort by CreatedAt desc
	for i := 0; i < len(all)-1; i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].CreatedAt.After(all[i].CreatedAt) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}

	total := int64(len(all))
	if offset > len(all) {
		return nil, total, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(all) {
		end = len(all)
	}

	return all[offset:end], total, nil
}

// LogStandaloneAudit records a non-trash audit log event (e.g. convert, sort, rename) in standalone mode.
func LogStandaloneAudit(entry database.AuditEntry) error {
	auditDir := filepath.Join(filepath.Dir(GetTrashDir()), "audit")
	if err := os.MkdirAll(auditDir, 0755); err != nil {
		return err
	}
	if entry.ID == 0 {
		entry.ID = time.Now().UnixNano()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	filename := fmt.Sprintf("%d.json", entry.ID)
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(auditDir, filename), data, 0644)
}

// LogStandaloneOperation records a bulk operation event in standalone mode.
func LogStandaloneOperation(op database.AuditOperation) error {
	opsDir := filepath.Join(filepath.Dir(GetTrashDir()), "operations")
	if err := os.MkdirAll(opsDir, 0755); err != nil {
		return err
	}
	if op.ID == 0 {
		op.ID = time.Now().UnixNano()
	}
	if op.CreatedAt.IsZero() {
		op.CreatedAt = time.Now()
	}
	filename := fmt.Sprintf("%d.json", op.ID)
	data, err := json.MarshalIndent(op, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(opsDir, filename), data, 0644)
}

// ListStandaloneOperations lists bulk operations in standalone mode.
func ListStandaloneOperations(operation string, limit, offset int) ([]database.AuditOperation, int64, error) {
	opsDir := filepath.Join(filepath.Dir(GetTrashDir()), "operations")
	entries, err := os.ReadDir(opsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, fmt.Errorf("reading operations dir: %w", err)
	}

	var all []database.AuditOperation
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(opsDir, e.Name()))
		if err != nil {
			continue
		}
		var item database.AuditOperation
		if err := json.Unmarshal(data, &item); err != nil {
			continue
		}
		if operation != "" && item.Operation != operation {
			continue
		}
		all = append(all, item)
	}

	// Sort by CreatedAt desc
	for i := 0; i < len(all)-1; i++ {
		for j := i + 1; j < len(all); j++ {
			if all[j].CreatedAt.After(all[i].CreatedAt) {
				all[i], all[j] = all[j], all[i]
			}
		}
	}

	total := int64(len(all))
	if offset > len(all) {
		return nil, total, nil
	}
	end := offset + limit
	if limit <= 0 || end > len(all) {
		end = len(all)
	}

	return all[offset:end], total, nil
}

// ListSafeTrash returns deleted files specifically for the Safe Trash view.
func ListSafeTrash(ctx context.Context, pool *pgxpool.Pool, limit, offset int) ([]database.AuditEntry, int64, error) {
	if pool != nil {
		return database.ListAuditHistory(ctx, pool, "delete", limit, offset)
	}
	return ListStandaloneTrash("delete", limit, offset)
}

// Restore moves a trashed file back to its original location.
func Restore(ctx context.Context, pool *pgxpool.Pool, auditID int64) error {
	var entry *database.AuditEntry
	var metaDir string
	if pool != nil {
		var err error
		entry, err = database.GetAuditEntry(ctx, pool, auditID)
		if err != nil {
			return fmt.Errorf("retrieving audit entry: %w", err)
		}
	}

	// Fallback to standalone trash metadata if entry was not found in DB
	if entry == nil {
		trashDir := GetTrashDir()
		entries, err := os.ReadDir(trashDir)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				dir := filepath.Join(trashDir, e.Name())
				metaPath := filepath.Join(dir, "meta.json")
				data, err := os.ReadFile(metaPath)
				if err != nil {
					continue
				}
				var m StandaloneMeta
				if err := json.Unmarshal(data, &m); err != nil {
					continue
				}
				if m.ID == auditID {
					entry = &database.AuditEntry{
						ID:           m.ID,
						Operation:    m.Operation,
						OriginalPath: m.OriginalPath,
						TrashPath:    m.TrashPath,
						FileSize:     m.FileSize,
						Reversible:   m.Reversible,
						ReversedAt:   m.ReversedAt,
						CreatedAt:    m.CreatedAt,
					}
					metaDir = dir
					break
				}
			}
		}
	}

	if entry == nil {
		return fmt.Errorf("audit entry %d not found", auditID)
	}
	if !entry.Reversible || entry.ReversedAt != nil {
		return fmt.Errorf("action %d is already reversed or not reversible", auditID)
	}
	if entry.TrashPath == "" {
		return fmt.Errorf("no trash path recorded for action %d", auditID)
	}

	// Verify trash file exists
	if _, err := os.Stat(entry.TrashPath); err != nil {
		return fmt.Errorf("trashed file not found at %s: %w", entry.TrashPath, err)
	}

	// Ensure destination directory exists
	origDir := filepath.Dir(entry.OriginalPath)
	if err := os.MkdirAll(origDir, 0755); err != nil {
		return fmt.Errorf("creating destination directory: %w", err)
	}

	// Move file back
	err := os.Rename(entry.TrashPath, entry.OriginalPath)
	if err != nil {
		if err := copyAndDelete(entry.TrashPath, entry.OriginalPath); err != nil {
			return fmt.Errorf("restoring file: %w", err)
		}
	}

	now := time.Now()
	if metaDir != "" {
		metaPath := filepath.Join(metaDir, "meta.json")
		if data, err := os.ReadFile(metaPath); err == nil {
			var m StandaloneMeta
			if json.Unmarshal(data, &m) == nil {
				m.ReversedAt = &now
				if mb, err := json.MarshalIndent(m, "", "  "); err == nil {
					_ = os.WriteFile(metaPath, mb, 0644)
				}
			}
		}
	} else {
		_ = os.RemoveAll(filepath.Dir(entry.TrashPath))
	}

	// Mark action as reversed in database if pool is available
	if pool != nil {
		if err := database.MarkActionReversed(ctx, pool, auditID); err != nil {
			return fmt.Errorf("updating audit record: %w", err)
		}
	}

	return nil
}

// PurgeExpired deletes trashed files older than retentionDays.
func PurgeExpired(ctx context.Context, pool *pgxpool.Pool, retentionDays int) (int, int64, error) {
	if retentionDays <= 0 {
		retentionDays = GetRetentionDays()
	}

	trashDir := GetTrashDir()
	entries, err := os.ReadDir(trashDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("reading trash directory: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	purgedCount := 0
	freedBytes := int64(0)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		itemPath := filepath.Join(trashDir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			// Walk directory to sum size and delete
			_ = filepath.Walk(itemPath, func(path string, f os.FileInfo, err error) error {
				if err == nil && f != nil && !f.IsDir() {
					freedBytes += f.Size()
					purgedCount++
				}
				return nil
			})
			_ = os.RemoveAll(itemPath)
		}
	}

	return purgedCount, freedBytes, nil
}

// StartAutoPurge starts a daily background goroutine to clean expired trash.
func StartAutoPurge(ctx context.Context, pool *pgxpool.Pool) {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				count, freed, err := PurgeExpired(ctx, pool, GetRetentionDays())
				if err != nil {
					slog.Warn("trash autopurge error", "err", err)
				} else if count > 0 {
					slog.Info("trash autopurge cleaned files", "count", count, "freed_bytes", freed)
				}
			}
		}
	}()
}

func copyAndDelete(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		_ = os.Remove(dst)
		return err
	}
	_ = in.Close()
	_ = out.Close()

	if err := os.Remove(src); err != nil {
		_ = os.Remove(dst)
		return err
	}
	return nil
}
