package dupfinder_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/andreassag/morphic/internal/dupfinder"
	"github.com/andreassag/morphic/internal/shared"
)

// assetsDir returns the path to assets/test from internal/dupfinder/.
func assetsDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "assets", "test"))
	if err != nil {
		t.Fatalf("cannot resolve assets dir: %v", err)
	}
	return dir
}

// ── ComputeImageHashes ──────────────────────────────────────────────────────

func TestComputeImageHashes_jpg(t *testing.T) {
	path := filepath.Join(assetsDir(t), "sample1.jpg")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("sample1.jpg not present")
	}
	info := dupfinder.ComputeImageHashes(path)
	if !info.HasHash {
		t.Error("expected HasHash=true for a valid JPEG")
	}
	if info.PHash == 0 && info.AHash == 0 && info.DHash == 0 {
		t.Error("all hashes are zero for a valid JPEG")
	}
	if info.Width == 0 || info.Height == 0 {
		t.Errorf("unexpected zero dimensions: %dx%d", info.Width, info.Height)
	}
}

func TestComputeImageHashes_png(t *testing.T) {
	path := filepath.Join(assetsDir(t), "sample2.png")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("sample2.png not present")
	}
	info := dupfinder.ComputeImageHashes(path)
	if !info.HasHash {
		t.Error("expected HasHash=true for a valid PNG")
	}
}

func TestComputeImageHashes_nonexistent(t *testing.T) {
	info := dupfinder.ComputeImageHashes("/nonexistent/file.jpg")
	if info.HasHash {
		t.Error("expected HasHash=false for a missing file")
	}
	if info.FileSize != 0 {
		t.Error("expected zero FileSize for a missing file")
	}
}

func TestComputeImageHashes_sameFileSameHash(t *testing.T) {
	path := filepath.Join(assetsDir(t), "sample1.jpg")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("sample1.jpg not present")
	}
	a := dupfinder.ComputeImageHashes(path)
	b := dupfinder.ComputeImageHashes(path)
	if a.PHash != b.PHash || a.AHash != b.AHash || a.DHash != b.DHash {
		t.Error("same file produced different hashes on two reads")
	}
}

// ── ComputeSimilarity ───────────────────────────────────────────────────────

func TestComputeSimilarity_identicalImage(t *testing.T) {
	path := filepath.Join(assetsDir(t), "sample1.jpg")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("sample1.jpg not present")
	}
	info := dupfinder.ComputeImageHashes(path)
	sim := dupfinder.ComputeSimilarity(&info, &info)
	if sim < 0.999 {
		t.Errorf("self-similarity expected ≈1.0, got %f", sim)
	}
}

func TestComputeSimilarity_differentImages(t *testing.T) {
	dir := assetsDir(t)
	// Use visually distinct images: smooth gradient vs high-frequency checkerboard
	pathA := filepath.Join(dir, "gradient.png")
	pathB := filepath.Join(dir, "checkerboard.png")
	if _, e := os.Stat(pathA); os.IsNotExist(e) {
		t.Skip("gradient.png not present")
	}
	if _, e := os.Stat(pathB); os.IsNotExist(e) {
		t.Skip("checkerboard.png not present")
	}
	a := dupfinder.ComputeImageHashes(pathA)
	b := dupfinder.ComputeImageHashes(pathB)
	// Different images should be less than 99% similar
	sim := dupfinder.ComputeSimilarity(&a, &b)
	if sim > 0.99 {
		t.Errorf("distinct images have unexpectedly high similarity: %f", sim)
	}
}

// ── ProcessImages ───────────────────────────────────────────────────────────

func TestProcessImages_basic(t *testing.T) {
	dir := assetsDir(t)
	files := []shared.FileInfo{
		{Path: filepath.Join(dir, "sample1.jpg"), Ext: ".jpg"},
		{Path: filepath.Join(dir, "sample2.png"), Ext: ".png"},
	}
	// Filter to only existing files
	var existing []shared.FileInfo
	for _, f := range files {
		if _, err := os.Stat(f.Path); err == nil {
			existing = append(existing, f)
		}
	}
	if len(existing) == 0 {
		t.Skip("no test images available")
	}

	result := dupfinder.ProcessImages(context.Background(), existing, 2)
	if len(result) == 0 {
		t.Error("ProcessImages returned empty result for valid images")
	}
	for path, info := range result {
		if !info.HasHash {
			t.Errorf("image %s has no hash after processing", path)
		}
	}
}

func TestProcessImages_cancelledContext(t *testing.T) {
	dir := assetsDir(t)
	files := []shared.FileInfo{
		{Path: filepath.Join(dir, "sample1.jpg"), Ext: ".jpg"},
		{Path: filepath.Join(dir, "sample2.png"), Ext: ".png"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	result := dupfinder.ProcessImages(ctx, files, 2)
	// With cancelled context, we should get 0 results (cancelled before first work)
	_ = result // either 0 or partial — just must not panic
}

// ── FindImageDuplicates ─────────────────────────────────────────────────────

func TestFindImageDuplicates_exactDuplicate(t *testing.T) {
	path := filepath.Join(assetsDir(t), "sample1.jpg")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("sample1.jpg not present")
	}

	info := dupfinder.ComputeImageHashes(path)
	// Register the same image under two different paths
	infos := map[string]*dupfinder.ImageInfo{
		"/fake/a.jpg": &info,
		"/fake/b.jpg": &info,
	}
	groups := dupfinder.FindImageDuplicates(infos, 0.9)
	if len(groups) == 0 {
		t.Error("expected at least one duplicate group for identical hashes")
	}
}

func TestFindImageDuplicates_noFalsePositives(t *testing.T) {
	dir := assetsDir(t)
	// Use visually distinct images: smooth gradient vs high-frequency checkerboard
	pathA := filepath.Join(dir, "gradient.png")
	pathB := filepath.Join(dir, "checkerboard.png")
	if _, e := os.Stat(pathA); os.IsNotExist(e) {
		t.Skip("gradient.png not present")
	}
	if _, e := os.Stat(pathB); os.IsNotExist(e) {
		t.Skip("checkerboard.png not present")
	}

	infoA := dupfinder.ComputeImageHashes(pathA)
	infoB := dupfinder.ComputeImageHashes(pathB)
	infos := map[string]*dupfinder.ImageInfo{
		pathA: &infoA,
		pathB: &infoB,
	}
	// High threshold — visually dissimilar images must not be grouped
	groups := dupfinder.FindImageDuplicates(infos, 0.99)
	for _, g := range groups {
		if len(g) > 1 {
			t.Errorf("distinct images grouped as duplicates at threshold 0.99: %v", g)
		}
	}
}

// ── StartJob / GetJob ───────────────────────────────────────────────────────

func TestStartJob_createsJob(t *testing.T) {
	dir := assetsDir(t)
	id := dupfinder.StartJob(dir, "images", 0.9, 0.85)
	if id == "" {
		t.Fatal("StartJob returned empty job ID")
	}

	job, ok := dupfinder.GetJob(id)
	if !ok {
		t.Fatal("GetJob returned false for a just-created job")
	}
	if job.ID != id {
		t.Errorf("job ID mismatch: got %q want %q", job.ID, id)
	}
}

func TestGetJob_unknownID(t *testing.T) {
	_, ok := dupfinder.GetJob("00000000-0000-0000-0000-000000000000")
	if ok {
		t.Error("expected false for unknown job ID")
	}
}
