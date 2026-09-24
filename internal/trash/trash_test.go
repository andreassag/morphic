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
