package organizer_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andreassag/morphic/internal/organizer"
)

// assetsDir returns the abs path of assets/test from internal/organizer/.
func assetsDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "assets", "test"))
	if err != nil {
		t.Fatalf("cannot resolve assets dir: %v", err)
	}
	return dir
}

// tempCopy copies src into a new temp dir and returns the copy's path.
func tempCopy(t *testing.T, src string) string {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("tempCopy read %s: %v", src, err)
	}
	dir := t.TempDir()
	dst := filepath.Join(dir, filepath.Base(src))
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("tempCopy write %s: %v", dst, err)
	}
	return dst
}

// ── RenderName ──────────────────────────────────────────────────────────────

func TestRenderName_dateToken(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "photo.jpg")
	os.WriteFile(f, []byte{0xFF, 0xD8, 0xFF}, 0o644)

	name := organizer.RenderName("{date}_{original}", f, 1)
	if !strings.Contains(name, "_photo") {
		t.Errorf("expected {original} → 'photo', got %q", name)
	}
	// {date} should be 8 digits: YYYYMMDD
	parts := strings.SplitN(name, "_", 2)
	if len(parts[0]) != 8 {
		t.Errorf("expected 8-digit date, got %q", parts[0])
	}
}

func TestRenderName_seqPadded(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "img.jpg")
	os.WriteFile(f, []byte{}, 0o644)

	name := organizer.RenderName("{seq:4}", f, 7)
	if !strings.HasPrefix(name, "0007") {
		t.Errorf("expected zero-padded seq '0007', got %q", name)
	}
}

func TestRenderName_seqPlain(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "img.jpg")
	os.WriteFile(f, []byte{}, 0o644)

	name := organizer.RenderName("{seq}", f, 42)
	if !strings.HasPrefix(name, "42") {
		t.Errorf("expected seq '42', got %q", name)
	}
}

func TestRenderName_extToken(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "clip.mp4")
	os.WriteFile(f, []byte{}, 0o644)

	name := organizer.RenderName("video_{seq}.{ext}", f, 1)
	if !strings.Contains(name, ".mp4") {
		t.Errorf("expected '.mp4' in output, got %q", name)
	}
}

func TestRenderName_datetimeToken(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "img.jpg")
	os.WriteFile(f, []byte{}, 0o644)

	name := organizer.RenderName("{datetime}", f, 1)
	base := strings.TrimSuffix(name, ".jpg")
	if len(base) != 15 {
		t.Errorf("expected 15-char datetime (YYYYMMDD_HHMMSS), got %q (%d chars)", base, len(base))
	}
}

// ── RenderTemplate ──────────────────────────────────────────────────────────

func TestRenderTemplate_basic(t *testing.T) {
	date := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	got := organizer.RenderTemplate("{year}/{month}/{day}", date)
	if got != "2024/06/15" {
		t.Errorf("RenderTemplate = %q, want %q", got, "2024/06/15")
	}
}

func TestRenderTemplate_zeropadded(t *testing.T) {
	date := time.Date(2024, 1, 5, 9, 3, 0, 0, time.UTC)
	got := organizer.RenderTemplate("{year}-{month}-{day}", date)
	if got != "2024-01-05" {
		t.Errorf("RenderTemplate = %q, want %q", got, "2024-01-05")
	}
}

// ── PlanSort ────────────────────────────────────────────────────────────────

func TestPlanSort_producesPendingEntries(t *testing.T) {
	tmp := t.TempDir()
	files := make([]string, 3)
	for i := range files {
		f := filepath.Join(tmp, fmt.Sprintf("img%d.jpg", i))
		os.WriteFile(f, []byte{0xFF, 0xD8, 0xFF}, 0o644)
		files[i] = f
	}

	dest := filepath.Join(tmp, "sorted")
	plan := organizer.PlanSort(files, "{year}/{month}/{day}", dest)

	if len(plan) != 3 {
		t.Fatalf("expected 3 plan entries, got %d", len(plan))
	}
	for _, e := range plan {
		if e.Status != "pending" {
			t.Errorf("expected 'pending', got %q for %s", e.Status, e.Source)
		}
		if !strings.HasPrefix(e.Destination, dest) {
			t.Errorf("destination %q does not start with %q", e.Destination, dest)
		}
	}
}

// ── ExecuteSort (copy) ──────────────────────────────────────────────────────

func TestExecuteSort_copy(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "photo.jpg")
	os.WriteFile(src, []byte{0xFF, 0xD8, 0xFF}, 0o644)

	dest := filepath.Join(tmp, "out")
	plan := organizer.PlanSort([]string{src}, "{year}/{month}", dest)
	organizer.ExecuteSort(context.Background(), plan, "copy")

	for _, e := range plan {
		if e.Status == "error" {
			t.Errorf("ExecuteSort copy error for %s: %s", e.Source, e.Error)
		}
		if _, err := os.Stat(e.Destination); err != nil {
			t.Errorf("expected destination file to exist: %v", err)
		}
		// Original must still exist (copy, not move)
		if _, err := os.Stat(src); err != nil {
			t.Error("original file was removed after copy")
		}
	}
}

func TestExecuteSort_move(t *testing.T) {
	src := tempCopy(t, filepath.Join(assetsDir(t), "sample1.jpg"))
	if _, err := os.Stat(filepath.Join(assetsDir(t), "sample1.jpg")); os.IsNotExist(err) {
		src2 := filepath.Join(filepath.Dir(src), "dummy.jpg")
		os.WriteFile(src2, []byte{0xFF, 0xD8, 0xFF}, 0o644)
		src = src2
	}

	dest := filepath.Join(filepath.Dir(src), "moved")
	plan := organizer.PlanSort([]string{src}, "{year}/{month}", dest)
	organizer.ExecuteSort(context.Background(), plan, "move")

	for _, e := range plan {
		if e.Status == "error" {
			t.Errorf("ExecuteSort move error: %s", e.Error)
			continue
		}
		if _, err := os.Stat(e.Destination); err != nil {
			t.Errorf("destination file missing after move: %v", err)
		}
		if _, err := os.Stat(e.Source); err == nil {
			t.Error("source file still exists after move")
		}
	}
}

// ── PlanRename ──────────────────────────────────────────────────────────────

func TestPlanRename_noConflicts(t *testing.T) {
	tmp := t.TempDir()
	files := []string{}
	for i := 0; i < 5; i++ {
		f := filepath.Join(tmp, fmt.Sprintf("raw%d.jpg", i))
		os.WriteFile(f, []byte{}, 0o644)
		files = append(files, f)
	}

	plan := organizer.PlanRename(files, "photo_{seq:3}", "move", 1)
	if len(plan) != 5 {
		t.Fatalf("expected 5 plan entries, got %d", len(plan))
	}
	for _, e := range plan {
		if e.Status == "conflict" {
			t.Errorf("unexpected conflict for %s → %s", e.Source, e.Destination)
		}
	}
}

func TestPlanRename_conflictDetected(t *testing.T) {
	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "a.jpg")
	f2 := filepath.Join(tmp, "b.jpg")
	os.WriteFile(f1, []byte{}, 0o644)
	os.WriteFile(f2, []byte{}, 0o644)
	now := time.Now()
	os.Chtimes(f1, now, now)
	os.Chtimes(f2, now, now)

	plan := organizer.PlanRename([]string{f1, f2}, "samename", "move", 1)
	conflicts := 0
	for _, e := range plan {
		if e.Status == "conflict" {
			conflicts++
		}
	}
	if conflicts < 2 {
		t.Errorf("expected ≥2 conflict entries, got %d", conflicts)
	}
}
