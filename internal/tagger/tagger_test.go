package tagger_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andreassag/morphic/internal/tagger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assetsDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "assets", "test"))
	require.NoError(t, err)
	return dir
}

func copyToTemp(t *testing.T, src string) string {
	t.Helper()
	data, err := os.ReadFile(src)
	require.NoError(t, err)

	tmp := filepath.Join(t.TempDir(), filepath.Base(src))
	err = os.WriteFile(tmp, data, 0o644)
	require.NoError(t, err)
	return tmp
}

func TestScanFileMetadata(t *testing.T) {
	ctx := context.Background()
	assets := assetsDir(t)

	tests := []struct {
		name    string
		file    string
		wantErr bool
	}{
		{
			name:    "Scan sample1.jpg",
			file:    filepath.Join(assets, "sample1.jpg"),
			wantErr: false,
		},
		{
			name:    "Scan sample2.png",
			file:    filepath.Join(assets, "sample2.png"),
			wantErr: false,
		},
		{
			name:    "Scan nonexistent file",
			file:    filepath.Join(assets, "does_not_exist.jpg"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item, err := tagger.ScanFileMetadata(ctx, tt.file)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, item.Name)
				assert.Greater(t, item.Size, int64(0))
			}
		})
	}
}

func TestScanMetadata_Directory(t *testing.T) {
	ctx := context.Background()
	assets := assetsDir(t)

	items, err := tagger.ScanMetadata(ctx, assets, false)
	require.NoError(t, err)
	assert.NotEmpty(t, items)
}

func TestStripMetadata_JPEG_and_PNG(t *testing.T) {
	ctx := context.Background()
	assets := assetsDir(t)

	jpgCopy := copyToTemp(t, filepath.Join(assets, "sample1.jpg"))
	pngCopy := copyToTemp(t, filepath.Join(assets, "sample2.png"))

	tests := []struct {
		name string
		path string
	}{
		{name: "Strip JPEG", path: jpgCopy},
		{name: "Strip PNG", path: pngCopy},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tagger.StripFileMetadata(ctx, tt.path)
			assert.NoError(t, err)

			// File should still exist and be non-empty
			info, err := os.Stat(tt.path)
			require.NoError(t, err)
			assert.Greater(t, info.Size(), int64(0))

			// Rescanning should succeed
			item, err := tagger.ScanFileMetadata(ctx, tt.path)
			assert.NoError(t, err)
			assert.False(t, item.HasExif)
		})
	}
}

func TestBatchUpdateMetadata(t *testing.T) {
	ctx := context.Background()
	assets := assetsDir(t)
	jpgCopy := copyToTemp(t, filepath.Join(assets, "sample1.jpg"))

	title := "Morphic Test Title"
	desc := "Morphic Test Description"
	patch := tagger.MetadataPatch{
		Title:       &title,
		Description: &desc,
	}

	count, err := tagger.BatchUpdateMetadata(ctx, []string{jpgCopy}, patch)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestTaggerJobLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := &tagger.TaggerJob{
		Job:  tagger.TaggerJob{}.Job,
		Type: "scan",
	}
	job.ID = "test-job-tagger-1"
	tagger.SetJob(job.ID, job)
	tagger.RegisterCancel(job.ID, cancel)

	retrieved, ok := tagger.GetJob(job.ID)
	assert.True(t, ok)
	assert.Equal(t, "test-job-tagger-1", retrieved.ID)

	cancelled := tagger.CancelJob(job.ID)
	assert.True(t, cancelled)

	tagger.StartCleanup(ctx, 10*time.Millisecond)
}
