package inspector_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/andreassag/morphic/internal/inspector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assetsDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "assets", "test"))
	require.NoError(t, err)
	return dir
}

func TestCategorizeFile(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{path: "photo.jpg", expected: "image"},
		{path: "photo.png", expected: "image"},
		{path: "clip.mp4", expected: "video"},
		{path: "document.pdf", expected: "other"},
		{path: "archive.zip", expected: "other"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			actual := inspector.CategorizeFile(tt.path)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestAnalyze_AssetsDir(t *testing.T) {
	ctx := context.Background()
	assets := assetsDir(t)

	res, err := inspector.Analyze(ctx, assets, 5)
	require.NoError(t, err)
	require.NotNil(t, res)

	assert.Equal(t, assets, res.RootPath)
	assert.Greater(t, res.FileCount, 0)
	assert.Greater(t, res.TotalSize, int64(0))
	assert.Greater(t, res.MediaCount, 0)

	assert.NotEmpty(t, res.LargestFiles)
	assert.LessOrEqual(t, len(res.LargestFiles), 5)

	assert.NotEmpty(t, res.OldestMediaFiles)
	assert.LessOrEqual(t, len(res.OldestMediaFiles), 5)

	assert.NotNil(t, res.Tree)
	assert.Equal(t, res.TotalSize, res.Tree.Size)
	assert.Equal(t, res.FileCount, res.Tree.FileCount)
}

func TestAnalyze_InvalidDirectory(t *testing.T) {
	ctx := context.Background()

	_, err := inspector.Analyze(ctx, "/path/that/definitely/does/not/exist", 10)
	assert.Error(t, err)
}
