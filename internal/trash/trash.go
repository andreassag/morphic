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

// MoveToTrash moves a file into the safe-trash store and logs an audit_log record.
func MoveToTrash(ctx context.Context, pool *pgxpool.Pool, filePath string) (int64, string, int64, error) {
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

	meta := map[string]interface{}{
		"original_filename": filepath.Base(filePath),
		"trashed_at":        time.Now().Format(time.RFC3339),
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

	return auditID, targetPath, size, nil
}

// Restore moves a trashed file back to its original location.
func Restore(ctx context.Context, pool *pgxpool.Pool, auditID int64) error {
	entry, err := database.GetAuditEntry(ctx, pool, auditID)
	if err != nil {
		return fmt.Errorf("retrieving audit entry: %w", err)
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
	err = os.Rename(entry.TrashPath, entry.OriginalPath)
	if err != nil {
		if err := copyAndDelete(entry.TrashPath, entry.OriginalPath); err != nil {
			return fmt.Errorf("restoring file: %w", err)
		}
	}

	// Remove empty trash parent dir
	_ = os.Remove(filepath.Dir(entry.TrashPath))

	// Mark action as reversed in database
	if err := database.MarkActionReversed(ctx, pool, auditID); err != nil {
		return fmt.Errorf("updating audit record: %w", err)
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
		return err
	}
	_ = in.Close()
	_ = out.Close()

	return os.Remove(src)
}
