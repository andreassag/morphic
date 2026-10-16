package gallery_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/andreassag/morphic/internal/gallery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func assetsDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", "..", "assets", "test"))
	require.NoError(t, err)
	return dir
}

func TestListMedia_FilteringAndSorting(t *testing.T) {
	ctx := context.Background()
	assets := assetsDir(t)

	tests := []struct {
		name      string
		query     gallery.GalleryQuery
		wantMin   int
		checkItem func(t *testing.T, res *gallery.GalleryResult)
	}{
		{
			name: "All media sorted by name asc",
			query: gallery.GalleryQuery{
				Folder:    assets,
				SortBy:    "name",
				SortOrder: "asc",
				Limit:     10,
			},
			wantMin: 5,
			checkItem: func(t *testing.T, res *gallery.GalleryResult) {
				if len(res.Items) >= 2 {
					assert.True(t, res.Items[0].Name <= res.Items[1].Name)
				}
			},
		},
		{
			name: "Filter only videos",
			query: gallery.GalleryQuery{
				Folder:     assets,
				FilterType: "video",
				Limit:      10,
			},
			wantMin: 1,
			checkItem: func(t *testing.T, res *gallery.GalleryResult) {
				for _, it := range res.Items {
					assert.Equal(t, "video", it.Type)
				}
			},
		},
		{
			name: "Filter only images",
			query: gallery.GalleryQuery{
				Folder:     assets,
				FilterType: "image",
				Limit:      10,
			},
			wantMin: 3,
			checkItem: func(t *testing.T, res *gallery.GalleryResult) {
				for _, it := range res.Items {
					assert.Equal(t, "image", it.Type)
				}
			},
		},
		{
			name: "Search query substring",
			query: gallery.GalleryQuery{
				Folder: assets,
				Query:  "sample1",
				Limit:  10,
			},
			wantMin: 1,
			checkItem: func(t *testing.T, res *gallery.GalleryResult) {
				for _, it := range res.Items {
					assert.Contains(t, it.Name, "sample1")
				}
			},
		},
		{
			name: "Pagination offset and limit",
			query: gallery.GalleryQuery{
				Folder: assets,
				Offset: 0,
				Limit:  2,
			},
			wantMin: 2,
			checkItem: func(t *testing.T, res *gallery.GalleryResult) {
				assert.Equal(t, 2, len(res.Items))
				assert.True(t, res.HasMore)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := gallery.ListMedia(ctx, tt.query)
			require.NoError(t, err)
			assert.GreaterOrEqual(t, res.Total, tt.wantMin)
			if tt.checkItem != nil {
				tt.checkItem(t, res)
			}
		})
	}
}

func TestListMedia_InvalidDirectory(t *testing.T) {
	ctx := context.Background()
	_, err := gallery.ListMedia(ctx, gallery.GalleryQuery{
		Folder: "/nonexistent/invalid/dir",
	})
	assert.Error(t, err)
}
