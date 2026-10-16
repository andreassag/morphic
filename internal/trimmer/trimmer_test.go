package trimmer_test

import (
	"bytes"
	"context"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"github.com/andreassag/morphic/internal/trimmer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assetsDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "assets", "test"))
	require.NoError(t, err)
	return dir
}

func TestApplyImageTransform_CropAndResize(t *testing.T) {
	ctx := context.Background()
	assets := assetsDir(t)
	src := filepath.Join(assets, "sample1.jpg")

	tests := []struct {
		name       string
		opts       trimmer.ImageTransformOpts
		checkImage func(t *testing.T, w, h int)
	}{
		{
			name: "Crop center region",
			opts: trimmer.ImageTransformOpts{
				SourcePath: src,
				CropX:      10,
				CropY:      10,
				CropWidth:  50,
				CropHeight: 40,
			},
			checkImage: func(t *testing.T, w, h int) {
				assert.Equal(t, 50, w)
				assert.Equal(t, 40, h)
			},
		},
		{
			name: "Resize fixed width and height",
			opts: trimmer.ImageTransformOpts{
				SourcePath: src,
				ResizeW:    64,
				ResizeH:    48,
			},
			checkImage: func(t *testing.T, w, h int) {
				assert.Equal(t, 64, w)
				assert.Equal(t, 48, h)
			},
		},
		{
			name: "Rotate 90 degrees",
			opts: trimmer.ImageTransformOpts{
				SourcePath: src,
				Rotate:     90,
			},
			checkImage: func(t *testing.T, w, h int) {
				assert.Greater(t, w, 0)
				assert.Greater(t, h, 0)
			},
		},
		{
			name: "Flip horizontal and vertical",
			opts: trimmer.ImageTransformOpts{
				SourcePath: src,
				FlipH:      true,
				FlipV:      true,
			},
			checkImage: func(t *testing.T, w, h int) {
				assert.Greater(t, w, 0)
				assert.Greater(t, h, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img, err := trimmer.ApplyImageTransform(ctx, tt.opts)
			require.NoError(t, err)
			require.NotNil(t, img)
			b := img.Bounds()
			tt.checkImage(t, b.Dx(), b.Dy())
		})
	}
}

func TestPreviewImageTransform(t *testing.T) {
	ctx := context.Background()
	assets := assetsDir(t)
	src := filepath.Join(assets, "sample1.jpg")

	data, err := trimmer.PreviewImageTransform(ctx, trimmer.ImageTransformOpts{
		SourcePath: src,
		ResizeW:    80,
		ResizeH:    60,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Validate JPEG header
	img, err := jpeg.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, 80, img.Bounds().Dx())
	assert.Equal(t, 60, img.Bounds().Dy())
}

func TestExportMedia_Image(t *testing.T) {
	ctx := context.Background()
	assets := assetsDir(t)
	src := filepath.Join(assets, "sample1.jpg")
	out := filepath.Join(t.TempDir(), "exported.jpg")

	outPath, err := trimmer.ExportMedia(ctx, trimmer.ExportOpts{
		Type:       "image",
		SourcePath: src,
		OutputPath: out,
		ImageOpts: trimmer.ImageTransformOpts{
			ResizeW: 100,
			ResizeH: 100,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, out, outPath)

	info, err := os.Stat(outPath)
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(0))
}
