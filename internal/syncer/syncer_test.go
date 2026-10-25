package syncer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andreassag/morphic/internal/syncer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanSync_OneWay(t *testing.T) {
	ctx := context.Background()
	srcDir := t.TempDir()
	targetDir := t.TempDir()

	// Source has file1 and file2
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("data 1"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file2.txt"), []byte("data 2"), 0o644))

	// Target has file1 (different data) and file3 (extra)
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "file1.txt"), []byte("data 1 old"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "file3.txt"), []byte("extra data"), 0o644))

	tests := []struct {
		name             string
		deleteExtraneous bool
		wantNew          int
		wantUpdated      int
		wantDeleted      int
	}{
		{
			name:             "One-way without extraneous deletion",
			deleteExtraneous: false,
			wantNew:          1, // file2
			wantUpdated:      1, // file1
			wantDeleted:      0,
		},
		{
			name:             "One-way with extraneous deletion",
			deleteExtraneous: true,
			wantNew:          1, // file2
			wantUpdated:      1, // file1
			wantDeleted:      1, // file3
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := syncer.PlanSync(ctx, syncer.SyncPlanRequest{
				SourceDir:        srcDir,
				TargetDir:        targetDir,
				Mode:             "one-way",
				DeleteExtraneous: tt.deleteExtraneous,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantNew, plan.Summary.NewCount)
			assert.Equal(t, tt.wantUpdated, plan.Summary.UpdatedCount)
			assert.Equal(t, tt.wantDeleted, plan.Summary.DeletedCount)
		})
	}
}

func TestExecuteSync(t *testing.T) {
	ctx := context.Background()
	srcDir := t.TempDir()
	targetDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "file_new.txt"), []byte("new file content"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "file_extra.txt"), []byte("to delete"), 0o644))

	plan, err := syncer.PlanSync(ctx, syncer.SyncPlanRequest{
		SourceDir:        srcDir,
		TargetDir:        targetDir,
		Mode:             "one-way",
		DeleteExtraneous: true,
	})
	require.NoError(t, err)

	err = syncer.ExecuteSync(ctx, plan, nil)
	require.NoError(t, err)

	// file_new.txt should exist in target
	_, err = os.Stat(filepath.Join(targetDir, "file_new.txt"))
	assert.NoError(t, err)

	// file_extra.txt should be deleted
	_, err = os.Stat(filepath.Join(targetDir, "file_extra.txt"))
	assert.True(t, os.IsNotExist(err))
}

func TestSyncerJobLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := &syncer.SyncerJob{
		Job: syncer.SyncerJob{}.Job,
	}
	job.ID = "test-job-syncer-1"
	syncer.SetJob(job.ID, job)
	syncer.RegisterCancel(job.ID, cancel)

	retrieved, ok := syncer.GetJob(job.ID)
	assert.True(t, ok)
	assert.Equal(t, "test-job-syncer-1", retrieved.ID)

	assert.True(t, syncer.CancelJob(job.ID))
	syncer.StartCleanup(ctx, 10*time.Millisecond)
}
