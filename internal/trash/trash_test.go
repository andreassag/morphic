package trash_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/andreassag/morphic/internal/trash"
)

func TestTrash_MoveAndRestore(t *testing.T) {
	trashDir := t.TempDir()
	t.Setenv("MORPHIC_TRASH_DIR", trashDir)

	tmp := t.TempDir()
	testFile := filepath.Join(tmp, "document.txt")
	content := []byte("important user document")
	if err := os.WriteFile(testFile, content, 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	auditID, trashPath, size, err := trash.MoveToTrash(ctx, nil, testFile)
	if err != nil {
		t.Fatalf("MoveToTrash failed: %v", err)
	}

	if size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), size)
	}

	// Verify original file is gone
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("expected original file to be moved")
	}

	// Verify file exists in trash
	if _, err := os.Stat(trashPath); err != nil {
		t.Errorf("expected file in trash at %s, got err: %v", trashPath, err)
	}

	// Verify it shows in ListStandaloneTrash
	entries, total, err := trash.ListStandaloneTrash("delete", 10, 0)
	if err != nil {
		t.Fatalf("ListStandaloneTrash failed: %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("expected 1 entry, got total=%d, len=%d", total, len(entries))
	}
	if entries[0].OriginalPath != testFile {
		t.Errorf("expected original path %s, got %s", testFile, entries[0].OriginalPath)
	}
	if entries[0].ID != auditID {
		t.Errorf("expected audit ID %d, got %d", auditID, entries[0].ID)
	}

	// Restore file
	if err := trash.Restore(ctx, nil, auditID); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	// Verify file restored
	restoredContent, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read restored file: %v", err)
	}
	if string(restoredContent) != string(content) {
		t.Errorf("restored content mismatch: expected %q, got %q", content, restoredContent)
	}

	// Second restore should fail because it is already reversed
	if err := trash.Restore(ctx, nil, auditID); err == nil {
		t.Error("expected second restore to fail because already reversed")
	}
}

func TestTrash_RejectsInvalidPaths(t *testing.T) {
	ctx := context.Background()

	_, _, _, err := trash.MoveToTrash(ctx, nil, "relative/path.txt")
	if err == nil {
		t.Error("expected error for relative path")
	}

	_, _, _, err = trash.MoveToTrash(ctx, nil, "/nonexistent/path/file.txt")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestTrash_DeleteItem(t *testing.T) {
	trashDir := t.TempDir()
	t.Setenv("MORPHIC_TRASH_DIR", trashDir)

	tmp := t.TempDir()
	testFile := filepath.Join(tmp, "item_to_delete.txt")
	content := []byte("temporary item")
	if err := os.WriteFile(testFile, content, 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	auditID, trashPath, size, err := trash.MoveToTrash(ctx, nil, testFile)
	if err != nil {
		t.Fatalf("MoveToTrash failed: %v", err)
	}

	freedBytes, err := trash.DeleteItem(ctx, nil, auditID)
	if err != nil {
		t.Fatalf("DeleteItem failed: %v", err)
	}
	if freedBytes != size {
		t.Errorf("expected freedBytes %d, got %d", size, freedBytes)
	}

	// Verify file is gone from trash
	if _, err := os.Stat(trashPath); !os.IsNotExist(err) {
		t.Errorf("expected trashed file to be deleted from %s", trashPath)
	}

	// Verify not in list
	entries, total, err := trash.ListSafeTrash(ctx, nil, 10, 0)
	if err != nil {
		t.Fatalf("ListSafeTrash failed: %v", err)
	}
	if total != 0 || len(entries) != 0 {
		t.Errorf("expected 0 entries after delete, got total=%d, len=%d", total, len(entries))
	}
}

func TestTrash_EmptyTrash(t *testing.T) {
	trashDir := t.TempDir()
	t.Setenv("MORPHIC_TRASH_DIR", trashDir)

	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "file1.txt")
	f2 := filepath.Join(tmp, "file2.txt")
	_ = os.WriteFile(f1, []byte("hello"), 0o644)
	_ = os.WriteFile(f2, []byte("world"), 0o644)

	ctx := context.Background()
	_, _, _, err1 := trash.MoveToTrash(ctx, nil, f1)
	_, _, _, err2 := trash.MoveToTrash(ctx, nil, f2)
	if err1 != nil || err2 != nil {
		t.Fatalf("failed to move files to trash: %v, %v", err1, err2)
	}

	count, freed, err := trash.EmptyTrash(ctx, nil)
	if err != nil {
		t.Fatalf("EmptyTrash failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 deleted files, got %d", count)
	}
	if freed != 10 { // 5 + 5 bytes
		t.Errorf("expected 10 freed bytes, got %d", freed)
	}

	entries, total, err := trash.ListSafeTrash(ctx, nil, 10, 0)
	if err != nil {
		t.Fatalf("ListSafeTrash failed: %v", err)
	}
	if total != 0 || len(entries) != 0 {
		t.Errorf("expected 0 entries after empty, got total=%d, len=%d", total, len(entries))
	}
}

func TestTrash_PurgeExpired_DaysZeroAndMetaJsonExcluded(t *testing.T) {
	trashDir := t.TempDir()
	t.Setenv("MORPHIC_TRASH_DIR", trashDir)

	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "purge_file.txt")
	_ = os.WriteFile(f1, []byte("contents to purge"), 0o644)

	ctx := context.Background()
	_, _, size, err := trash.MoveToTrash(ctx, nil, f1)
	if err != nil {
		t.Fatalf("failed to move to trash: %v", err)
	}

	purged, freed, err := trash.PurgeExpired(ctx, nil, 0)
	if err != nil {
		t.Fatalf("PurgeExpired failed: %v", err)
	}
	if purged != 1 {
		t.Errorf("expected 1 purged file, got %d", purged)
	}
	if freed != size {
		t.Errorf("expected %d freed bytes (excluding meta.json), got %d", size, freed)
	}
}

func TestTrash_RestoreRemovesFromActiveTrash(t *testing.T) {
	trashDir := t.TempDir()
	t.Setenv("MORPHIC_TRASH_DIR", trashDir)

	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "restore_file.txt")
	_ = os.WriteFile(f1, []byte("restore me"), 0o644)

	ctx := context.Background()
	auditID, _, _, err := trash.MoveToTrash(ctx, nil, f1)
	if err != nil {
		t.Fatalf("failed to move to trash: %v", err)
	}

	entriesBefore, totalBefore, _ := trash.ListSafeTrash(ctx, nil, 10, 0)
	if totalBefore != 1 || len(entriesBefore) != 1 {
		t.Fatalf("expected 1 active trash item before restore")
	}

	if err := trash.Restore(ctx, nil, auditID); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	entriesAfter, totalAfter, _ := trash.ListSafeTrash(ctx, nil, 10, 0)
	if totalAfter != 0 || len(entriesAfter) != 0 {
		t.Errorf("expected 0 active trash items after restore, got total=%d", totalAfter)
	}
}
