package shared_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/andreassag/morphic/internal/shared"
)

// assetsDir returns assets/test relative to internal/shared/.
func assetsDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "assets", "test"))
	if err != nil {
		t.Fatalf("cannot resolve assets dir: %v", err)
	}
	return dir
}

// ── NormaliseExt ────────────────────────────────────────────────────────────

func TestNormaliseExt_jpeg(t *testing.T) {
	if got := shared.NormaliseExt(".jpeg"); got != ".jpg" {
		t.Errorf("NormaliseExt(.jpeg) = %q, want %q", got, ".jpg")
	}
}

func TestNormaliseExt_tiff(t *testing.T) {
	if got := shared.NormaliseExt(".tiff"); got != ".tif" {
		t.Errorf("NormaliseExt(.tiff) = %q, want %q", got, ".tif")
	}
}

func TestNormaliseExt_uppercase(t *testing.T) {
	if got := shared.NormaliseExt(".JPG"); got != ".jpg" {
		t.Errorf("NormaliseExt(.JPG) = %q, want %q", got, ".jpg")
	}
}

func TestNormaliseExt_unknownPassthrough(t *testing.T) {
	if got := shared.NormaliseExt(".xyz"); got != ".xyz" {
		t.Errorf("NormaliseExt(.xyz) = %q, want %q", got, ".xyz")
	}
}

// ── IsImage / IsVideo ───────────────────────────────────────────────────────

func TestIsImage(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"photo.jpg", true},
		{"photo.JPEG", true},
		{"clip.mp4", false},
		{"document.pdf", false},
		{"image.png", true},
		{"image.webp", true},
		{"video.mkv", false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := shared.IsImage(tc.path); got != tc.want {
				t.Errorf("IsImage(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestIsVideo(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"clip.mp4", true},
		{"clip.MOV", true},
		{"photo.jpg", false},
		{"clip.mkv", true},
		{"clip.avi", true},
		{"photo.png", false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := shared.IsVideo(tc.path); got != tc.want {
				t.Errorf("IsVideo(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// ── FormatFileSize ──────────────────────────────────────────────────────────

func TestFormatFileSize(t *testing.T) {
	cases := []struct {
		bytes int64
		want  string
	}{
		{0, "0.00 B"},
		{512, "512.00 B"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1024 * 1024, "1.00 MB"},
		{1024 * 1024 * 1024, "1.00 GB"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%d", tc.bytes), func(t *testing.T) {
			got := shared.FormatFileSize(tc.bytes)
			if got != tc.want {
				t.Errorf("FormatFileSize(%d) = %q, want %q", tc.bytes, got, tc.want)
			}
		})
	}
}

// ── FormatDuration ──────────────────────────────────────────────────────────

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		secs float64
		want string
	}{
		{0, "0s"},
		{45, "45s"},
		{90, "1m 30s"},
		{3661, "1h 1m 1s"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			got := shared.FormatDuration(tc.secs)
			if got != tc.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tc.secs, got, tc.want)
			}
		})
	}
}

// ── FindFilesByExtension ────────────────────────────────────────────────────

func TestFindFilesByExtension_findsImages(t *testing.T) {
	dir := assetsDir(t)
	exts := map[string]struct{}{".jpg": {}, ".png": {}, ".tif": {}}
	files, err := shared.FindFilesByExtension(dir, exts, shared.ExcludedFolders)
	if err != nil {
		t.Fatalf("FindFilesByExtension error: %v", err)
	}
	if len(files) == 0 {
		t.Error("expected at least one image file in assets/test")
	}
	for _, f := range files {
		if _, ok := exts[f.Ext]; !ok {
			t.Errorf("unexpected extension %q for file %s", f.Ext, f.Path)
		}
	}
}

func TestFindFilesByExtension_emptyDir(t *testing.T) {
	dir := t.TempDir()
	files, err := shared.FindFilesByExtension(dir, map[string]struct{}{".jpg": {}}, shared.ExcludedFolders)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files in empty dir, got %d", len(files))
	}
}

func TestFindFilesByExtension_noDuplicates(t *testing.T) {
	dir := assetsDir(t)
	files, err := shared.FindFilesByExtension(dir, shared.ImageExtensions, shared.ExcludedFolders)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	seen := make(map[string]int)
	for _, f := range files {
		seen[f.Path]++
	}
	for path, count := range seen {
		if count > 1 {
			t.Errorf("file %s appeared %d times (expected 1)", path, count)
		}
	}
}

func TestFindImageFiles_basic(t *testing.T) {
	files, err := shared.FindImageFiles(assetsDir(t))
	if err != nil {
		t.Fatalf("FindImageFiles error: %v", err)
	}
	if len(files) == 0 {
		t.Error("expected images in assets/test")
	}
}

func TestFindAllMediaFiles_includesBoth(t *testing.T) {
	files, err := shared.FindAllMediaFiles(assetsDir(t))
	if err != nil {
		t.Fatalf("FindAllMediaFiles error: %v", err)
	}
	hasImage, hasVideo := false, false
	for _, f := range files {
		if _, ok := shared.ImageExtensions[f.Ext]; ok {
			hasImage = true
		}
		if _, ok := shared.VideoExtensions[f.Ext]; ok {
			hasVideo = true
		}
	}
	if !hasImage {
		t.Error("expected at least one image")
	}
	if !hasVideo {
		t.Error("expected at least one video")
	}
}

// ── IsExcludedPath ──────────────────────────────────────────────────────────

func TestIsExcludedPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/home/user/photos/img.jpg", false},
		{"/home/user/$Recycle.Bin/img.jpg", true},
		{"/home/user/.Trash/img.jpg", true},
		{"/home/user/node_modules/img.jpg", true}, // node_modules is excluded
		{"/home/user/Pictures/img.jpg", false},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := shared.IsExcludedPath(tc.path, shared.ExcludedFolders)
			if got != tc.want {
				t.Errorf("IsExcludedPath(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// ── GenerateImageThumbnail ──────────────────────────────────────────────────

func TestGenerateImageThumbnail_jpg(t *testing.T) {
	path := filepath.Join(assetsDir(t), "sample1.jpg")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("sample1.jpg not present")
	}
	data, err := shared.GenerateImageThumbnail(path, 64)
	if err != nil {
		t.Fatalf("GenerateImageThumbnail error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty thumbnail bytes")
	}
	// JPEG starts with FF D8
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		t.Error("thumbnail is not a valid JPEG (missing FF D8 header)")
	}
}

func TestGenerateImageThumbnail_png(t *testing.T) {
	path := filepath.Join(assetsDir(t), "sample2.png")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skip("sample2.png not present")
	}
	data, err := shared.GenerateImageThumbnail(path, 64)
	if err != nil {
		t.Fatalf("GenerateImageThumbnail png error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty thumbnail bytes")
	}
}

// ── JobStore ────────────────────────────────────────────────────────────────

func TestJobStore_setAndGet(t *testing.T) {
	type item struct{ shared.Job }
	store := shared.NewJobStore[item]()

	job := item{Job: shared.NewJob()}
	store.Set(job.ID, &job)

	got, ok := store.Get(job.ID)
	if !ok {
		t.Fatal("Get returned false after Set")
	}
	if got.ID != job.ID {
		t.Errorf("ID mismatch: got %q want %q", got.ID, job.ID)
	}
}

func TestJobStore_missingKey(t *testing.T) {
	type item struct{ shared.Job }
	store := shared.NewJobStore[item]()

	_, ok := store.Get("does-not-exist")
	if ok {
		t.Error("expected false for missing key")
	}
}

// ── Job context / cancel ────────────────────────────────────────────────────

func TestJob_cancelStopsContext(t *testing.T) {
	job := shared.NewJob()
	ctx := job.Ctx()

	select {
	case <-ctx.Done():
		t.Fatal("context should not be done before Cancel()")
	default:
	}

	job.Cancel()

	select {
	case <-ctx.Done():
		// expected
	default:
		t.Error("context should be done after Cancel()")
	}
}

func TestJob_doubleCancelNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("double Cancel() panicked: %v", r)
		}
	}()
	job := shared.NewJob()
	job.Cancel()
	job.Cancel() // must not panic
}
