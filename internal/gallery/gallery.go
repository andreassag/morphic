package gallery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/andreassag/morphic/internal/shared"
)

// MediaItem represents a single image or video item for gallery browsing.
type MediaItem struct {
	Path    string    `json:"path"`
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
	Ext     string    `json:"ext"`
	Type    string    `json:"type"` // "image" or "video"
	Width   int       `json:"width,omitempty"`
	Height  int       `json:"height,omitempty"`
}

// GalleryQuery defines sorting, filtering, and pagination parameters.
type GalleryQuery struct {
	Folder     string `json:"folder" form:"folder"`
	Recursive  bool   `json:"recursive" form:"recursive"`
	SortBy     string `json:"sort_by" form:"sort_by"`       // "date", "size", "name", "type"
	SortOrder  string `json:"sort_order" form:"sort_order"` // "asc", "desc"
	FilterType string `json:"type" form:"type"`             // "all", "image", "video"
	Ext        string `json:"ext" form:"ext"`               // specific extension e.g. ".jpg"
	MinSize    int64  `json:"min_size" form:"min_size"`
	MaxSize    int64  `json:"max_size" form:"max_size"`
	Query      string `json:"query" form:"query"` // search in filename
	Offset     int    `json:"offset" form:"offset"`
	Limit      int    `json:"limit" form:"limit"`
}

// GalleryResult holds the paginated response for media items.
type GalleryResult struct {
	Items   []MediaItem `json:"items"`
	Total   int         `json:"total"`
	Offset  int         `json:"offset"`
	Limit   int         `json:"limit"`
	HasMore bool        `json:"has_more"`
}

// ListMedia enumerates media files matching the query criteria.
func ListMedia(ctx context.Context, query GalleryQuery) (*GalleryResult, error) {
	safeDir, err := shared.ValidateSafeDirPath(query.Folder)
	if err != nil {
		return nil, fmt.Errorf("validate directory: %w", err)
	}

	if query.Limit <= 0 {
		query.Limit = 50
	}
	if query.Limit > 200 {
		query.Limit = 200
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	if query.SortBy == "" {
		query.SortBy = "date"
	}
	if query.SortOrder == "" {
		query.SortOrder = "desc"
	}
	if query.FilterType == "" {
		query.FilterType = "all"
	}
	filterExt := strings.ToLower(strings.TrimSpace(query.Ext))
	if filterExt != "" && !strings.HasPrefix(filterExt, ".") {
		filterExt = "." + filterExt
	}
	searchQuery := strings.ToLower(strings.TrimSpace(query.Query))

	var collected []MediaItem

	err = filepath.WalkDir(safeDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			if !query.Recursive && path != safeDir {
				return filepath.SkipDir
			}
			return nil
		}

		name := d.Name()
		if searchQuery != "" && !strings.Contains(strings.ToLower(name), searchQuery) {
			return nil
		}

		ext := shared.NormaliseExt(filepath.Ext(path))
		if filterExt != "" && ext != filterExt {
			return nil
		}

		isImg := shared.IsImage(path)
		isVid := shared.IsVideo(path)
		if !isImg && !isVid {
			return nil
		}

		mediaType := "image"
		if isVid {
			mediaType = "video"
		}
		if query.FilterType == "image" && mediaType != "image" {
			return nil
		}
		if query.FilterType == "video" && mediaType != "video" {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		size := info.Size()
		if query.MinSize > 0 && size < query.MinSize {
			return nil
		}
		if query.MaxSize > 0 && size > query.MaxSize {
			return nil
		}

		collected = append(collected, MediaItem{
			Path:    path,
			Name:    name,
			Size:    size,
			ModTime: info.ModTime(),
			Ext:     ext,
			Type:    mediaType,
		})
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking gallery %s: %w", safeDir, err)
	}

	// Sort
	sort.Slice(collected, func(i, j int) bool {
		switch query.SortBy {
		case "name":
			if query.SortOrder == "asc" {
				return collected[i].Name < collected[j].Name
			}
			return collected[i].Name > collected[j].Name
		case "size":
			if query.SortOrder == "asc" {
				return collected[i].Size < collected[j].Size
			}
			return collected[i].Size > collected[j].Size
		case "type":
			if collected[i].Type != collected[j].Type {
				if query.SortOrder == "asc" {
					return collected[i].Type < collected[j].Type
				}
				return collected[i].Type > collected[j].Type
			}
			return collected[i].Name < collected[j].Name
		case "date":
			fallthrough
		default:
			if query.SortOrder == "asc" {
				return collected[i].ModTime.Before(collected[j].ModTime)
			}
			return collected[i].ModTime.After(collected[j].ModTime)
		}
	})

	total := len(collected)
	start := query.Offset
	if start > total {
		start = total
	}
	end := start + query.Limit
	if end > total {
		end = total
	}

	paged := collected[start:end]
	hasMore := end < total

	return &GalleryResult{
		Items:   paged,
		Total:   total,
		Offset:  query.Offset,
		Limit:   query.Limit,
		HasMore: hasMore,
	}, nil
}
