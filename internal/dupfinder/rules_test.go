package dupfinder_test

import (
	"testing"

	"github.com/exterex/morphic/internal/dupfinder"
)

func TestApplyCullingRule_KeepLargest(t *testing.T) {
	group := []dupfinder.MediaEntry{
		{Path: "/photos/small.jpg", FileSize: 1000},
		{Path: "/photos/large.jpg", FileSize: 5000},
		{Path: "/photos/medium.jpg", FileSize: 2500},
	}

	result := dupfinder.ApplyCullingRule([]dupfinder.DuplicateGroup{{Items: group}}, dupfinder.RuleKeepLargest)
	if result.TotalSelected != 2 {
		t.Fatalf("expected 2 files selected for deletion, got %d", result.TotalSelected)
	}

	// small and medium should be selected for deletion, large should be kept
	selectedMap := make(map[string]bool)
	for _, p := range result.SelectedFiles {
		selectedMap[p] = true
	}

	if selectedMap["/photos/large.jpg"] {
		t.Error("expected /photos/large.jpg to be kept, but was marked for deletion")
	}
	if !selectedMap["/photos/small.jpg"] || !selectedMap["/photos/medium.jpg"] {
		t.Error("expected smaller files to be marked for deletion")
	}
}

func TestApplyCullingRule_KeepShortestName(t *testing.T) {
	group := []dupfinder.MediaEntry{
		{Path: "/photos/photo_copy_1.jpg", FileSize: 1000},
		{Path: "/photos/photo.jpg", FileSize: 1000},
		{Path: "/photos/photo_backup.jpg", FileSize: 1000},
	}

	result := dupfinder.ApplyCullingRule([]dupfinder.DuplicateGroup{{Items: group}}, dupfinder.RuleKeepShortestName)
	if result.TotalSelected != 2 {
		t.Fatalf("expected 2 files selected for deletion, got %d", result.TotalSelected)
	}

	for _, p := range result.SelectedFiles {
		if p == "/photos/photo.jpg" {
			t.Error("expected shortest name /photos/photo.jpg to be kept")
		}
	}
}
