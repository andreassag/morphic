package hasher_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andreassag/morphic/internal/hasher"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeFileHashes(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	fPath := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(fPath, []byte("hello morphic"), 0o644)
	require.NoError(t, err)

	item, err := hasher.ComputeFileHashes(ctx, fPath, tmpDir, true, true, true)
	require.NoError(t, err)

	assert.Equal(t, "test.txt", item.RelPath)
	assert.NotEmpty(t, item.MD5)
	assert.NotEmpty(t, item.SHA256)
	assert.NotEmpty(t, item.XXHash64)
}

func TestHashDirectory_And_ExportManifest(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	err := os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("data 1"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("data 2"), 0o644)
	require.NoError(t, err)

	items, err := hasher.HashDirectory(ctx, tmpDir, true, true, true, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, len(items))

	tests := []struct {
		format string
		substr string
	}{
		{format: "sha256", substr: "file1.txt"},
		{format: "md5", substr: "file1.txt"},
		{format: "xxh64", substr: "file1.txt"},
		{format: "json", substr: `"rel_path": "file1.txt"`},
	}

	for _, tt := range tests {
		t.Run("Export_"+tt.format, func(t *testing.T) {
			data, err := hasher.ExportManifest(items, tt.format)
			require.NoError(t, err)
			assert.Contains(t, string(data), tt.substr)
		})
	}
}

func TestVerifyManifest_BitRotDetection(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "ok.txt")
	file2 := filepath.Join(tmpDir, "corrupted.txt")
	err := os.WriteFile(file1, []byte("original content 1"), 0o644)
	require.NoError(t, err)
	err = os.WriteFile(file2, []byte("original content 2"), 0o644)
	require.NoError(t, err)

	items, err := hasher.HashDirectory(ctx, tmpDir, true, true, true, nil)
	require.NoError(t, err)

	manifest, err := hasher.ExportManifest(items, "sha256")
	require.NoError(t, err)

	// Simulate bit-rot by mutating 1 byte in corrupted.txt
	err = os.WriteFile(file2, []byte("coriginal content 2"), 0o644)
	require.NoError(t, err)

	// Also simulate adding a new untracked file
	err = os.WriteFile(filepath.Join(tmpDir, "new_file.txt"), []byte("new"), 0o644)
	require.NoError(t, err)

	res, err := hasher.VerifyManifest(ctx, tmpDir, manifest, "sha256")
	require.NoError(t, err)

	assert.Equal(t, 1, res.OKCount)
	assert.Equal(t, 1, res.CorruptCount)
	assert.Equal(t, 1, res.NewCount)
	assert.Equal(t, 0, res.MissingCount)
	assert.Equal(t, "corrupted.txt", res.Corrupted[0].RelPath)
}

func TestHasherJobStore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := &hasher.HasherJob{
		Job:       hasher.HasherJob{}.Job,
		TargetDir: "/tmp",
	}
	job.ID = "test-job-hasher-1"
	hasher.SetJob(job.ID, job)
	hasher.RegisterCancel(job.ID, cancel)

	retrieved, ok := hasher.GetJob(job.ID)
	assert.True(t, ok)
	assert.Equal(t, "test-job-hasher-1", retrieved.ID)

	assert.True(t, hasher.CancelJob(job.ID))
	hasher.StartCleanup(ctx, 10*time.Millisecond)
}
