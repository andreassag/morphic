package converter

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/andreassag/morphic/internal/shared"
)

// ScanResult holds the output of a folder scan.
type ScanResult struct {
	Folder             string         `json:"folder"`
	Summary            map[string]int `json:"summary"`
	Files              []FileEntry    `json:"files"`
	Total              int            `json:"total"`
	TotalCount         int            `json:"total_count"`
	TotalSize          int64          `json:"total_size"`
	TotalSizeFormatted string         `json:"total_size_formatted"`
}

// FileEntry describes one file found during scanning.
type FileEntry struct {
	Path          string   `json:"path"`
	Name          string   `json:"name"`
	Filename      string   `json:"filename"` // Alias for frontend compatibility
	Ext           string   `json:"ext"`
	Size          int64    `json:"size"`
	SizeFormatted string   `json:"size_formatted"`
	Type          string   `json:"type"`
	Targets       []string `json:"targets"`
}

// ScanFolder walks folder and returns an inventory of convertible media files.
func ScanFolder(folder string, includeSubfolders bool, filterType string, excludeFolders ...string) (*ScanResult, error) {
	allowed := make(map[string]struct{})
	if filterType == "images" || filterType == "both" {
		for k, v := range shared.ImageExtensions {
			allowed[k] = v
		}
	}
	if filterType == "videos" || filterType == "both" {
		for k, v := range shared.VideoExtensions {
			allowed[k] = v
		}
	}

	// Build exclude map
	excluded := make(map[string]struct{})
	for k := range shared.ExcludedFolders {
		excluded[k] = struct{}{}
	}
	for _, ef := range excludeFolders {
		for _, part := range strings.Split(ef, ",") {
			p := strings.ToLower(strings.TrimSpace(part))
			if p != "" {
				excluded[p] = struct{}{}
			}
		}
	}

	summary := make(map[string]int)
	var files []FileEntry
	var totalBytes int64

	if includeSubfolders {
		err := filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil // Skip unreadable entries
			}
			if d.IsDir() {
				name := strings.ToLower(d.Name())
				if _, excl := excluded[name]; excl {
					return filepath.SkipDir
				}
				return nil
			}
			addFileEntry(path, d, allowed, summary, &files, &totalBytes)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walking folder %s: %w", folder, err)
		}
	} else {
		entries, err := os.ReadDir(folder)
		if err != nil {
			return nil, fmt.Errorf("reading directory %s: %w", folder, err)
		}
		for _, d := range entries {
			if d.IsDir() {
				continue
			}
			path := filepath.Join(folder, d.Name())
			addFileEntry(path, d, allowed, summary, &files, &totalBytes)
		}
	}

	// Sort files by name
	sort.Slice(files, func(i, j int) bool {
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	return &ScanResult{
		Folder:             folder,
		Summary:            summary,
		Files:              files,
		Total:              len(files),
		TotalCount:         len(files),
		TotalSize:          totalBytes,
		TotalSizeFormatted: shared.FormatFileSize(totalBytes),
	}, nil
}

func addFileEntry(path string, d os.DirEntry, allowed map[string]struct{}, summary map[string]int, files *[]FileEntry, totalBytes *int64) {
	ext := shared.NormaliseExt(filepath.Ext(path))
	if _, ok := allowed[ext]; !ok {
		return
	}

	var size int64
	if info, err := d.Info(); err == nil {
		size = info.Size()
	}

	ftype := "image"
	if _, ok := shared.VideoExtensions[ext]; ok {
		ftype = "video"
	}

	targets := GetCompatibleTargets(path)
	summary[ext]++
	*totalBytes += size

	*files = append(*files, FileEntry{
		Path:          path,
		Name:          d.Name(),
		Filename:      d.Name(),
		Ext:           ext,
		Size:          size,
		SizeFormatted: shared.FormatFileSize(size),
		Type:          ftype,
		Targets:       targets,
	})
}
