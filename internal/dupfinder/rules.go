package dupfinder

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/andreassag/morphic/internal/shared"
)

// CullingRule specifies the strategy for choosing which duplicate to keep.
type CullingRule string

const (
	RuleKeepLargest         CullingRule = "largest"
	RuleKeepHighestRes      CullingRule = "highest_resolution"
	RuleKeepShortestName    CullingRule = "shortest_name"
	RuleKeepPreferredFormat CullingRule = "preferred_format"
)

// AutoSelectResult returns the paths marked for deletion based on the chosen culling rule.
type AutoSelectResult struct {
	Rule           CullingRule `json:"rule"`
	SelectedFiles  []string    `json:"selected_files"`
	KeptFiles      []string    `json:"kept_files"`
	TotalFiles     int         `json:"total_files"`
	TotalSelected  int         `json:"total_selected"`
	PotentialFreed int64       `json:"potential_freed"`
	FreedFormatted string      `json:"freed_formatted"`
}

func getFilename(item MediaEntry) string {
	if item.Filename != "" {
		return item.Filename
	}
	return filepath.Base(item.Path)
}

// ApplyCullingRule applies an auto-culling strategy to duplicate groups, returning files marked for deletion.
func ApplyCullingRule(groups []DuplicateGroup, rule CullingRule) AutoSelectResult {
	var selectedFiles []string
	var keptFiles []string
	var freedBytes int64
	totalCount := 0

	for _, grp := range groups {
		group := grp.Items
		if len(group) <= 1 {
			continue
		}
		totalCount += len(group)

		// Make a copy of group slice to sort
		sorted := make([]MediaEntry, len(group))
		copy(sorted, group)

		switch rule {
		case RuleKeepHighestRes:
			sort.Slice(sorted, func(i, j int) bool {
				resI := sorted[i].Width * sorted[i].Height
				resJ := sorted[j].Width * sorted[j].Height
				if resI != resJ {
					return resI > resJ
				}
				return sorted[i].FileSize > sorted[j].FileSize
			})
		case RuleKeepShortestName:
			sort.Slice(sorted, func(i, j int) bool {
				nameI := getFilename(sorted[i])
				nameJ := getFilename(sorted[j])
				if len(nameI) != len(nameJ) {
					return len(nameI) < len(nameJ)
				}
				return sorted[i].FileSize > sorted[j].FileSize
			})
		case RuleKeepPreferredFormat:
			rank := func(ext string) int {
				ext = strings.ToLower(ext)
				switch ext {
				case ".avif", ".webp":
					return 10
				case ".png":
					return 8
				case ".jpg", ".jpeg":
					return 6
				case ".mp4", ".webm", ".mkv":
					return 10
				default:
					return 1
				}
			}
			sort.Slice(sorted, func(i, j int) bool {
				nameI := getFilename(sorted[i])
				nameJ := getFilename(sorted[j])
				extI := filepath.Ext(nameI)
				extJ := filepath.Ext(nameJ)
				rI := rank(extI)
				rJ := rank(extJ)
				if rI != rJ {
					return rI > rJ
				}
				return sorted[i].FileSize > sorted[j].FileSize
			})
		default: // RuleKeepLargest
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].FileSize > sorted[j].FileSize
			})
		}

		// Kept is index 0 (top-ranked), all other entries in group are selected for deletion
		keptFiles = append(keptFiles, sorted[0].Path)
		for _, item := range sorted[1:] {
			selectedFiles = append(selectedFiles, item.Path)
			freedBytes += item.FileSize
		}
	}

	return AutoSelectResult{
		Rule:           rule,
		SelectedFiles:  selectedFiles,
		KeptFiles:      keptFiles,
		TotalFiles:     totalCount,
		TotalSelected:  len(selectedFiles),
		PotentialFreed: freedBytes,
		FreedFormatted: shared.FormatFileSize(freedBytes),
	}
}
