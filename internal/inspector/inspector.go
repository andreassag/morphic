package inspector

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/andreassag/morphic/internal/shared"
)

// FileStatItem describes an individual file for top-N ranking.
type FileStatItem struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Size     int64     `json:"size"`
	ModTime  time.Time `json:"mod_time"`
	Ext      string    `json:"ext"`
	Category string    `json:"category"`
}

// AgeBucketStats holds size and count distributions across file ages.
type AgeBucketStats struct {
	Count int `json:"count"`
	Bytes int64 `json:"bytes"`
}

// AgeBuckets breaks down metrics by file age.
type AgeBuckets struct {
	Under30d     AgeBucketStats `json:"under_30d"`
	From30dTo180d AgeBucketStats `json:"from_30d_to_180d"`
	From180dTo1y  AgeBucketStats `json:"from_180d_to_1y"`
	Over1y       AgeBucketStats `json:"over_1y"`
}

// TreeNode represents a node in the hierarchical folder tree.
type TreeNode struct {
	Name           string            `json:"name"`
	Path           string            `json:"path"`
	Size           int64             `json:"size"`
	FileCount      int               `json:"file_count"`
	Children       []*TreeNode       `json:"children,omitempty"`
	CategoryCounts map[string]int    `json:"category_counts,omitempty"`
	ExtCounts      map[string]int    `json:"ext_counts,omitempty"`
}

// InspectResult is the complete storage analytics report.
type InspectResult struct {
	RootPath           string            `json:"root_path"`
	TotalSize          int64             `json:"total_size"`
	FileCount          int               `json:"file_count"`
	MediaCount         int               `json:"media_count"`
	CategorySizes      map[string]int64  `json:"category_sizes"`
	CategoryCounts     map[string]int    `json:"category_counts"`
	ExtensionSizes     map[string]int64  `json:"extension_sizes"`
	ExtensionCounts    map[string]int    `json:"extension_counts"`
	AgeBreakdown       AgeBuckets        `json:"age_breakdown"`
	LargestFiles       []FileStatItem    `json:"largest_files"`
	OldestMediaFiles   []FileStatItem    `json:"oldest_media_files"`
	Tree               *TreeNode         `json:"tree"`
}

// CategorizeFile determines if a file is an image, video, or other.
func CategorizeFile(path string) string {
	if shared.IsImage(path) {
		return "image"
	}
	if shared.IsVideo(path) {
		return "video"
	}
	return "other"
}

// Analyze traverses folder and returns full storage analytics.
func Analyze(ctx context.Context, folder string, maxTopN int) (*InspectResult, error) {
	safeDir, err := shared.ValidateSafeDirPath(folder)
	if err != nil {
		return nil, fmt.Errorf("validate directory: %w", err)
	}

	if maxTopN <= 0 {
		maxTopN = 20
	}

	result := &InspectResult{
		RootPath:        safeDir,
		CategorySizes:   map[string]int64{"image": 0, "video": 0, "other": 0},
		CategoryCounts:  map[string]int{"image": 0, "video": 0, "other": 0},
		ExtensionSizes:  make(map[string]int64),
		ExtensionCounts: make(map[string]int),
	}

	// Map of absolute directory path to its TreeNode
	treeNodes := make(map[string]*TreeNode)
	rootNode := &TreeNode{
		Name:           filepath.Base(safeDir),
		Path:           safeDir,
		CategoryCounts: make(map[string]int),
		ExtCounts:      make(map[string]int),
	}
	treeNodes[safeDir] = rootNode

	var allFiles []FileStatItem
	var mediaFiles []FileStatItem
	now := time.Now()

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
			if _, exists := treeNodes[path]; !exists {
				node := &TreeNode{
					Name:           d.Name(),
					Path:           path,
					CategoryCounts: make(map[string]int),
					ExtCounts:      make(map[string]int),
				}
				treeNodes[path] = node

				// Connect to parent node if parent exists
				parent := filepath.Dir(path)
				if pNode, ok := treeNodes[parent]; ok {
					pNode.Children = append(pNode.Children, node)
				}
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		size := info.Size()
		modTime := info.ModTime()
		ext := shared.NormaliseExt(filepath.Ext(path))
		if ext == "" {
			ext = "(none)"
		}
		cat := CategorizeFile(path)

		result.TotalSize += size
		result.FileCount++
		if cat == "image" || cat == "video" {
			result.MediaCount++
		}

		result.CategorySizes[cat] += size
		result.CategoryCounts[cat]++
		result.ExtensionSizes[ext] += size
		result.ExtensionCounts[ext]++

		// Age breakdown
		age := now.Sub(modTime)
		switch {
		case age < 30*24*time.Hour:
			result.AgeBreakdown.Under30d.Count++
			result.AgeBreakdown.Under30d.Bytes += size
		case age < 180*24*time.Hour:
			result.AgeBreakdown.From30dTo180d.Count++
			result.AgeBreakdown.From30dTo180d.Bytes += size
		case age < 365*24*time.Hour:
			result.AgeBreakdown.From180dTo1y.Count++
			result.AgeBreakdown.From180dTo1y.Bytes += size
		default:
			result.AgeBreakdown.Over1y.Count++
			result.AgeBreakdown.Over1y.Bytes += size
		}

		statItem := FileStatItem{
			Path:     path,
			Name:     d.Name(),
			Size:     size,
			ModTime:  modTime,
			Ext:      ext,
			Category: cat,
		}
		allFiles = append(allFiles, statItem)
		if cat == "image" || cat == "video" {
			mediaFiles = append(mediaFiles, statItem)
		}

		// Update folder tree stats from current dir up to root
		dir := filepath.Dir(path)
		for {
			node, ok := treeNodes[dir]
			if ok {
				node.Size += size
				node.FileCount++
				node.CategoryCounts[cat]++
				node.ExtCounts[ext]++
			}
			if dir == safeDir || dir == filepath.Dir(dir) {
				break
			}
			dir = filepath.Dir(dir)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking %s for inspection: %w", safeDir, err)
	}

	// Sort largest files descending by size
	sort.Slice(allFiles, func(i, j int) bool {
		return allFiles[i].Size > allFiles[j].Size
	})
	if len(allFiles) > maxTopN {
		result.LargestFiles = allFiles[:maxTopN]
	} else {
		result.LargestFiles = allFiles
	}

	// Sort oldest media files ascending by ModTime
	sort.Slice(mediaFiles, func(i, j int) bool {
		return mediaFiles[i].ModTime.Before(mediaFiles[j].ModTime)
	})
	if len(mediaFiles) > maxTopN {
		result.OldestMediaFiles = mediaFiles[:maxTopN]
	} else {
		result.OldestMediaFiles = mediaFiles
	}

	result.Tree = rootNode
	return result, nil
}
