package compare_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/exterex/morphic/internal/compare"
)

func assetsDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "assets", "test"))
	if err != nil {
		t.Fatalf("cannot resolve assets dir: %v", err)
	}
	return dir
}

func TestCompareMetadata_basic(t *testing.T) {
	dir := assetsDir(t)
	img1 := filepath.Join(dir, "sample1.jpg")
	img2 := filepath.Join(dir, "sample2.png")

	if _, err := os.Stat(img1); os.IsNotExist(err) {
		t.Skip("sample1.jpg not present")
	}
	if _, err := os.Stat(img2); os.IsNotExist(err) {
		t.Skip("sample2.png not present")
	}

	result, err := compare.CompareMetadata(context.Background(), img1, img2)
	if err != nil {
		t.Fatalf("CompareMetadata failed: %v", err)
	}

	if result.Left.Filename != "sample1.jpg" {
		t.Errorf("expected left filename sample1.jpg, got %s", result.Left.Filename)
	}
	if result.Right.Filename != "sample2.png" {
		t.Errorf("expected right filename sample2.png, got %s", result.Right.Filename)
	}
}

func TestGenerateDiffImage_differentImages(t *testing.T) {
	dir := assetsDir(t)
	img1 := filepath.Join(dir, "sample1.jpg")
	img2 := filepath.Join(dir, "sample2.png")

	if _, err := os.Stat(img1); os.IsNotExist(err) {
		t.Skip("sample1.jpg not present")
	}
	if _, err := os.Stat(img2); os.IsNotExist(err) {
		t.Skip("sample2.png not present")
	}

	data, err := compare.GenerateDiffImage(context.Background(), img1, img2)
	if err != nil {
		t.Fatalf("GenerateDiffImage failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("expected non-empty JPEG diff image data")
	}
}

func TestGenerateDiffImage_videoRejection(t *testing.T) {
	tmpDir := t.TempDir()
	v1 := filepath.Join(tmpDir, "test1.mp4")
	v2 := filepath.Join(tmpDir, "test2.mp4")
	_ = os.WriteFile(v1, []byte("fake video content 1"), 0644)
	_ = os.WriteFile(v2, []byte("fake video content 2"), 0644)

	_, err := compare.GenerateDiffImage(context.Background(), v1, v2)
	if err == nil {
		t.Error("expected error when running GenerateDiffImage on video files")
	}
}

func TestCompareMetadata_videoFiles(t *testing.T) {
	tmpDir := t.TempDir()
	v1 := filepath.Join(tmpDir, "sample.mp4")
	v2 := filepath.Join(tmpDir, "sample.mkv")
	_ = os.WriteFile(v1, []byte("fake video 1"), 0644)
	_ = os.WriteFile(v2, []byte("fake video 2 larger"), 0644)

	res, err := compare.CompareMetadata(context.Background(), v1, v2)
	if err != nil {
		t.Fatalf("CompareMetadata failed on video files: %v", err)
	}

	if res.Left.Type != "video" {
		t.Errorf("expected left type 'video', got %s", res.Left.Type)
	}
	if res.Right.Type != "video" {
		t.Errorf("expected right type 'video', got %s", res.Right.Type)
	}
}
