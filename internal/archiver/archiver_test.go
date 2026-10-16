package archiver_test

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andreassag/morphic/internal/archiver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateArchive_ZipAndTarGz(t *testing.T) {
	ctx := context.Background()
	srcDir := t.TempDir()
	destDir := t.TempDir()

	f1 := filepath.Join(srcDir, "doc1.txt")
	f2 := filepath.Join(srcDir, "doc2.txt")
	require.NoError(t, os.WriteFile(f1, []byte("Content of document 1"), 0o644))
	require.NoError(t, os.WriteFile(f2, []byte("Content of document 2"), 0o644))

	tests := []struct {
		name   string
		format string
		level  string
	}{
		{name: "Create ZIP normal", format: "zip", level: "normal"},
		{name: "Create ZIP fast", format: "zip", level: "fast"},
		{name: "Create TAR.GZ normal", format: "tar.gz", level: "normal"},
		{name: "Create TAR.GZ best", format: "tar.gz", level: "best"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := archiver.ArchiveRequest{
				SourcePaths:    []string{srcDir},
				DestinationDir: destDir,
				ArchiveName:    "test_" + tt.format,
				Format:         tt.format,
				Level:          tt.level,
			}

			files, err := archiver.CreateArchive(ctx, req, nil)
			require.NoError(t, err)
			require.NotEmpty(t, files)

			info, err := os.Stat(files[0])
			require.NoError(t, err)
			assert.Greater(t, info.Size(), int64(0))

			if tt.format == "zip" {
				r, err := zip.OpenReader(files[0])
				require.NoError(t, err)
				defer r.Close()
				assert.GreaterOrEqual(t, len(r.File), 2)
			}
		})
	}
}

func TestSplitWriter_MultiPart(t *testing.T) {
	tmpDir := t.TempDir()
	baseArchive := filepath.Join(tmpDir, "split_test.bin")

	// Split threshold = 1 MB (or test SplitWriter with custom byte size)
	sw, err := archiver.NewSplitWriter(baseArchive, 1) // 1 MB split
	require.NoError(t, err)

	// Write 2.5 MB of data
	chunk := make([]byte, 512*1024) // 512 KB
	for i := 0; i < 5; i++ {
		_, err := sw.Write(chunk)
		require.NoError(t, err)
	}
	require.NoError(t, sw.Close())

	created := sw.CreatedFiles()
	assert.GreaterOrEqual(t, len(created), 3)

	for _, f := range created {
		info, err := os.Stat(f)
		require.NoError(t, err)
		assert.Greater(t, info.Size(), int64(0))
	}
}

func TestArchiverJobLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := &archiver.ArchiverJob{
		Job: archiver.ArchiverJob{}.Job,
	}
	job.ID = "test-job-archiver-1"
	archiver.SetJob(job.ID, job)
	archiver.RegisterCancel(job.ID, cancel)

	retrieved, ok := archiver.GetJob(job.ID)
	assert.True(t, ok)
	assert.Equal(t, "test-job-archiver-1", retrieved.ID)

	assert.True(t, archiver.CancelJob(job.ID))
	archiver.StartCleanup(ctx, 10*time.Millisecond)
}
