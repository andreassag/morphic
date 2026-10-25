package importer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andreassag/morphic/internal/importer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSSRF(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "Disallowed file scheme", url: "file:///etc/passwd", wantErr: true},
		{name: "Disallowed ftp scheme", url: "ftp://example.com/file.jpg", wantErr: true},
		{name: "Blocked localhost string", url: "http://localhost:8080/image.png", wantErr: true},
		{name: "Blocked IPv4 loopback", url: "http://127.0.0.1:9000/test.jpg", wantErr: true},
		{name: "Blocked IPv4 private 10.x", url: "http://10.0.0.1/test.jpg", wantErr: true},
		{name: "Blocked IPv4 private 192.168.x", url: "http://192.168.1.1/test.jpg", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := importer.ValidateSSRF(tt.url)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestImportFromDevice(t *testing.T) {
	ctx := context.Background()
	srcDir := t.TempDir()
	destDir := t.TempDir()

	img := filepath.Join(srcDir, "photo.jpg")
	vid := filepath.Join(srcDir, "video.mp4")
	txt := filepath.Join(srcDir, "notes.txt")

	require.NoError(t, os.WriteFile(img, []byte{0xFF, 0xD8, 0xFF, 0xE0}, 0o644))
	require.NoError(t, os.WriteFile(vid, []byte("fake mp4 video"), 0o644))
	require.NoError(t, os.WriteFile(txt, []byte("not media"), 0o644))

	imported, err := importer.ImportFromDevice(ctx, importer.DeviceImportRequest{
		SourceDir:      srcDir,
		DestinationDir: destDir,
		Recursive:      true,
	})
	require.NoError(t, err)

	// Should import only photo.jpg and video.mp4, skipping notes.txt
	assert.Equal(t, 2, len(imported))
	for _, f := range imported {
		info, err := os.Stat(f)
		require.NoError(t, err)
		assert.Greater(t, info.Size(), int64(0))
	}
}

func TestDetectMountedDevices(t *testing.T) {
	ctx := context.Background()
	devs, err := importer.DetectMountedDevices(ctx)
	require.NoError(t, err)
	// Even if empty on minimal test container, should not error
	assert.NotNil(t, devs)
}

func TestImporterJobLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	job := &importer.ImporterJob{
		Job:  importer.ImporterJob{}.Job,
		Type: "device",
	}
	job.ID = "test-job-importer-1"
	importer.SetJob(job.ID, job)
	importer.RegisterCancel(job.ID, cancel)

	retrieved, ok := importer.GetJob(job.ID)
	assert.True(t, ok)
	assert.Equal(t, "test-job-importer-1", retrieved.ID)

	assert.True(t, importer.CancelJob(job.ID))
	importer.StartCleanup(ctx, 10*time.Millisecond)
}
