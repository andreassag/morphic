package trash_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/exterex/morphic/internal/trash"
)

func TestTrash_MoveAndRestore(t *testing.T) {
	tmp := t.TempDir()
	testFile := filepath.Join(tmp, "document.txt")
	content := []byte("important user document")
	if err := os.WriteFile(testFile, content, 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	ctx := context.Background()
	_, trashPath, size, err := trash.MoveToTrash(ctx, nil, testFile)
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
