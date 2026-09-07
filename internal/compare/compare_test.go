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
