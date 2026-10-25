package resizer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andreassag/morphic/internal/resizer"
	"github.com/disintegration/imaging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assetsDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "assets", "test"))
	require.NoError(t, err)
	return dir
}

func TestResolveTargets(t *testing.T) {
	tests := []struct {
		name         string
		presets      []string
		customWidths []int
		wantMin      int
	}{
		{
			name:    "Web-optimized preset",
			presets: []string{"web-optimized"},
			wantMin: 2,
		},
		{
			name:    "Thumbnails preset",
			presets: []string{"thumbnails"},
			wantMin: 3,
		},
		{
			name:         "Custom widths",
			customWidths: []int{800, 1024},
			wantMin:      2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targets := resizer.ResolveTargets(tt.presets, tt.customWidths)
			assert.GreaterOrEqual(t, len(targets), tt.wantMin)
		})
	}
}

func TestBatchResize_GeneratesParallelTree(t *testing.T) {
	ctx := context.Background()
	assets := assetsDir(t)
	srcDir := t.TempDir()
	destDir := t.TempDir()

	// Copy a test image
	srcFile := filepath.Join(assets, "sample1.jpg")
	data, err := os.ReadFile(srcFile)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "sample1.jpg"), data, 0o644))

	req := resizer.BatchResizeRequest{
		SourceDir:      srcDir,
		DestinationDir: destDir,
		CustomWidths:   []int{120, 240},
		Quality:        80,
	}

	created, err := resizer.BatchResize(ctx, req, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, len(created))

	// Verify that each resized image has proper dimensions
	for _, f := range created {
		img, err := imaging.Open(f)
		require.NoError(t, err)
		require.NotNil(t, img)
		b := img.Bounds()
		assert.True(t, b.Dx() == 120 || b.Dx() == 240)
	}
}

func TestResizerJobLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := &resizer.ResizerJob{
		Job: resizer.ResizerJob{}.Job,
	}
	job.ID = "test-job-resizer-1"
	resizer.SetJob(job.ID, job)
	resizer.RegisterCancel(job.ID, cancel)

	retrieved, ok := resizer.GetJob(job.ID)
	assert.True(t, ok)
	assert.Equal(t, "test-job-resizer-1", retrieved.ID)

	assert.True(t, resizer.CancelJob(job.ID))
	resizer.StartCleanup(ctx, 10*time.Millisecond)
}
