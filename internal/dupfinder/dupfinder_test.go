package dupfinder_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/exterex/morphic/internal/dupfinder"
	"github.com/exterex/morphic/internal/shared"
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
	info := dupfinder.ComputeImageHashes(context.Background(), path)
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
	info := dupfinder.ComputeImageHashes(context.Background(), path)
	if !info.HasHash {
		t.Error("expected HasHash=true for a valid PNG")
	}
}

func TestComputeImageHashes_nonexistent(t *testing.T) {
	info := dupfinder.ComputeImageHashes(context.Background(), "/nonexistent/file.jpg")
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
	a := dupfinder.ComputeImageHashes(context.Background(), path)
	b := dupfinder.ComputeImageHashes(context.Background(), path)
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
	info := dupfinder.ComputeImageHashes(context.Background(), path)
	sim := dupfinder.ComputeSimilarity(&info, &info)
	if sim < 0.999 {
		t.Errorf("self-similarity expected ≈1.0, got %f", sim)
	}
}

func TestComputeSimilarity_differentImages(t *testing.T) {
	dir := assetsDir(t)
	pathA := filepath.Join(dir, "gradient.png")
	pathB := filepath.Join(dir, "checkerboard.png")
	if _, e := os.Stat(pathA); os.IsNotExist(e) {
		t.Skip("gradient.png not present")
	}
	if _, e := os.Stat(pathB); os.IsNotExist(e) {
		t.Skip("checkerboard.png not present")
	}
	a := dupfinder.ComputeImageHashes(context.Background(), pathA)
	b := dupfinder.ComputeImageHashes(context.Background(), pathB)
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
	var existing []shared.FileInfo
	for _, f := range files {
		if _, err := os.Stat(f.Path); err == nil {
			existing = append(existing, f)
		}
	}
	if len(existing) == 0 {
		t.Skip("no test images available")
	}

	result := dupfinder.ProcessImages(context.Background(), existing, 2, nil)
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

	result := dupfinder.ProcessImages(ctx, files, 2, nil)
	_ = result // partial/empty results expected
}

// ── FindImageDuplicates ─────────────────────────────────────────────────────

func TestFindImageDuplicates_exactDuplicate(t *testing.T) {
	path := filepath.Join(assetsDir(t), "sample1.jpg")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("sample1.jpg not present")
	}

	info := dupfinder.ComputeImageHashes(context.Background(), path)
	infos := map[string]*dupfinder.ImageInfo{
		"/fake/a.jpg": &info,
		"/fake/b.jpg": &info,
	}
	groups := dupfinder.FindImageDuplicates(context.Background(), infos, 0.9, nil)
	if len(groups) == 0 {
		t.Error("expected at least one duplicate group for identical hashes")
	}
}

func TestFindImageDuplicates_noFalsePositives(t *testing.T) {
	dir := assetsDir(t)
	pathA := filepath.Join(dir, "gradient.png")
	pathB := filepath.Join(dir, "checkerboard.png")
	if _, e := os.Stat(pathA); os.IsNotExist(e) {
		t.Skip("gradient.png not present")
	}
	if _, e := os.Stat(pathB); os.IsNotExist(e) {
		t.Skip("checkerboard.png not present")
	}

	infoA := dupfinder.ComputeImageHashes(context.Background(), pathA)
	infoB := dupfinder.ComputeImageHashes(context.Background(), pathB)
	infos := map[string]*dupfinder.ImageInfo{
		pathA: &infoA,
		pathB: &infoB,
	}
	groups := dupfinder.FindImageDuplicates(context.Background(), infos, 0.99, nil)
	for _, g := range groups {
		if len(g) > 1 {
			t.Errorf("distinct images grouped as duplicates at threshold 0.99: %v", g)
		}
	}
}

// ── StartJob / GetJob ───────────────────────────────────────────────────────

func TestStartJob_createsJob(t *testing.T) {
	dir := assetsDir(t)
	id := dupfinder.StartJob(context.Background(), dir, "images", 0.9, 0.85)
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
