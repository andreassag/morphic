package converter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andreassag/morphic/internal/converter"
)

// assetsDir returns the absolute path to assets/test relative to this file.
func assetsDir(t *testing.T) string {
	t.Helper()
	// From internal/converter/ we go up three levels to reach the repo root.
	dir, err := filepath.Abs(filepath.Join("..", "..", "assets", "test"))
	if err != nil {
		t.Fatalf("cannot resolve assets dir: %v", err)
	}
	return dir
}

// ── ScanFolder ─────────────────────────────────────────────────────────────

func TestScanFolder_basic(t *testing.T) {
	dir := assetsDir(t)
	result, err := converter.ScanFolder(dir, false, "both")
	if err != nil {
		t.Fatalf("ScanFolder returned error: %v", err)
	}
	if result.Total == 0 {
		t.Error("expected at least one file in assets/test, got 0")
	}
	if result.Folder != dir {
		t.Errorf("folder mismatch: got %q want %q", result.Folder, dir)
	}
}

func TestScanFolder_imageFilter(t *testing.T) {
	dir := assetsDir(t)
	result, err := converter.ScanFolder(dir, false, "images")
	if err != nil {
		t.Fatalf("ScanFolder(images) error: %v", err)
	}
	for _, f := range result.Files {
		if f.Type != "image" {
			t.Errorf("expected image type, got %q for %s", f.Type, f.Name)
		}
	}
}

func TestScanFolder_videoFilter(t *testing.T) {
	dir := assetsDir(t)
	result, err := converter.ScanFolder(dir, false, "videos")
	if err != nil {
		t.Fatalf("ScanFolder(videos) error: %v", err)
	}
	for _, f := range result.Files {
		if f.Type != "video" {
			t.Errorf("expected video type, got %q for %s", f.Type, f.Name)
		}
	}
}

func TestScanFolder_invalidPath(t *testing.T) {
	_, err := converter.ScanFolder("/nonexistent/path/xyz", false, "both")
	if err == nil {
		t.Error("expected error for nonexistent folder, got nil")
	}
}

func TestScanFolder_summaryMatchesTotal(t *testing.T) {
	dir := assetsDir(t)
	result, err := converter.ScanFolder(dir, true, "both")
	if err != nil {
		t.Fatalf("ScanFolder error: %v", err)
	}
	sum := 0
	for _, v := range result.Summary {
		sum += v
	}
	if sum != result.Total {
		t.Errorf("summary counts (%d) don't match total (%d)", sum, result.Total)
	}
}

// ── ConvertImage ────────────────────────────────────────────────────────────

func TestConvertImage_jpgToPng(t *testing.T) {
	src := filepath.Join(assetsDir(t), "sample1.jpg")
	if _, err := os.Stat(src); os.IsNotExist(err) {
		t.Skip("sample1.jpg not present")
	}

	tmp := t.TempDir()
	out, err := converter.ConvertImage(src, ".png", tmp)
	if err != nil {
		t.Fatalf("ConvertImage jpg→png failed: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("output file not created: %v", err)
	}
	if !strings.HasSuffix(strings.ToLower(out), ".png") {
		t.Errorf("expected .png output, got %s", out)
	}
}

func TestConvertImage_pngToWebp(t *testing.T) {
	src := filepath.Join(assetsDir(t), "sample2.png")
	if _, err := os.Stat(src); os.IsNotExist(err) {
		t.Skip("sample2.png not present")
	}

	tmp := t.TempDir()
	out, err := converter.ConvertImage(src, ".webp", tmp)
	if err != nil {
		t.Fatalf("ConvertImage png→webp failed: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("output file not created: %v", err)
	}
}

func TestConvertImage_tifToJpg(t *testing.T) {
	src := filepath.Join(assetsDir(t), "sample3.tif")
	if _, err := os.Stat(src); os.IsNotExist(err) {
		t.Skip("sample3.tif not present")
	}

	tmp := t.TempDir()
	out, err := converter.ConvertImage(src, ".jpg", tmp)
	if err != nil {
		t.Fatalf("ConvertImage tif→jpg failed: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("output file not created: %v", err)
	}
}

func TestConvertImage_rgbaPngToJpg(t *testing.T) {
	src := filepath.Join(assetsDir(t), "sample_rgba.png")
	if _, err := os.Stat(src); os.IsNotExist(err) {
		t.Skip("sample_rgba.png not present")
	}

	tmp := t.TempDir()
	out, err := converter.ConvertImage(src, ".jpg", tmp)
	if err != nil {
		t.Fatalf("ConvertImage rgba.png→jpg failed: %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("output file not created: %v", err)
	}
}

func TestConvertImage_noOverwrite(t *testing.T) {
	src := filepath.Join(assetsDir(t), "sample1.jpg")
	if _, err := os.Stat(src); os.IsNotExist(err) {
		t.Skip("sample1.jpg not present")
	}

	tmp := t.TempDir()
	// Convert twice — second call must not overwrite; it adds "_converted" suffix
	out1, err := converter.ConvertImage(src, ".png", tmp)
	if err != nil {
		t.Fatalf("first convert failed: %v", err)
	}
	out2, err := converter.ConvertImage(src, ".png", tmp)
	if err != nil {
		t.Fatalf("second convert failed: %v", err)
	}
	if out1 == out2 {
		t.Error("expected second output path to differ (no-overwrite), got same path")
	}
}

// ── ImageConversions table ──────────────────────────────────────────────────

func TestImageConversions_nonEmpty(t *testing.T) {
	if len(converter.ImageConversions) == 0 {
		t.Error("ImageConversions is empty")
	}
	for src, targets := range converter.ImageConversions {
		if len(targets) == 0 {
			t.Errorf("no targets for source format %q", src)
		}
		// Source must not appear in its own target list
		for _, tgt := range targets {
			if tgt == src {
				t.Errorf("format %q lists itself as a target", src)
			}
		}
	}
}

func TestVideoConversions_nonEmpty(t *testing.T) {
	if len(converter.VideoConversions) == 0 {
		t.Error("VideoConversions is empty")
	}
}

// ── Input validation ─────────────────────────────────────────────────────────

func TestConvertImage_relativePath(t *testing.T) {
	_, err := converter.ConvertImage("relative/path/image.jpg", ".png", "")
	if err == nil {
		t.Error("expected error for relative source path, got nil")
	}
}

func TestConvertImage_nullBytePath(t *testing.T) {
	_, err := converter.ConvertImage("/valid/path/image\x00.jpg", ".png", "")
	if err == nil {
		t.Error("expected error for source path containing NUL byte, got nil")
	}
}

func TestConvertVideo_relativePath(t *testing.T) {
	_, err := converter.ConvertVideo("relative/path/video.mp4", ".mkv", "", "", 0)
	if err == nil {
		t.Error("expected error for relative source path, got nil")
	}
}

func TestConvertVideo_nullBytePath(t *testing.T) {
	_, err := converter.ConvertVideo("/valid/path/video\x00.mp4", ".mkv", "", "", 0)
	if err == nil {
		t.Error("expected error for source path containing NUL byte, got nil")
	}
}
