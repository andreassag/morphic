package converter

import (
	"sort"

	"github.com/exterex/morphic/internal/shared"
)

// VideoContainerConfig describes a supported output video container.
type VideoContainerConfig struct {
	Name       string   `json:"name"`
	Codecs     []string `json:"codecs"`
	Extensions []string `json:"extensions"`
}

// VideoContainers lists the three supported output video containers.
var VideoContainers = []VideoContainerConfig{
	{
		Name:       "MP4",
		Codecs:     []string{"h264", "h265", "av1"},
		Extensions: []string{".mp4", ".m4a", ".m4p", ".m4b", ".m4r", ".m4v"},
	},
	{
		Name:       "Matroska",
		Codecs:     []string{"h264", "h265", "av1", "vp9"},
		Extensions: []string{".mkv", ".mk3d", ".mka", ".mks"},
	},
	{
		Name:       "WebM",
		Codecs:     []string{"vp8", "vp9", "av1"},
		Extensions: []string{".webm"},
	},
}

// Canonical image formats we can write to.
var canonicalImage = map[string]struct{}{
	".jpg":  {},
	".png":  {},
	".tif":  {},
	".bmp":  {},
	".webp": {},
	".gif":  {},
	".ico":  {},
	".avif": {},
}

// canonicalVideo is the set of all supported output video extensions (derived from VideoContainers).
var canonicalVideo map[string]struct{}

// ImageConversions maps source image extension to list of target extensions.
var ImageConversions map[string][]string

// VideoConversions maps source video extension to list of all canonical video output extensions.
var VideoConversions map[string][]string

func init() {
	canonicalVideo = make(map[string]struct{})
	for _, c := range VideoContainers {
		for _, ext := range c.Extensions {
			canonicalVideo[ext] = struct{}{}
		}
	}

	allVideoTargets := make([]string, 0, len(canonicalVideo))
	for ext := range canonicalVideo {
		allVideoTargets = append(allVideoTargets, ext)
	}
	sort.Strings(allVideoTargets)

	ImageConversions = make(map[string][]string)
	for ext := range shared.ImageExtensions {
		norm := shared.NormaliseExt(ext)
		if _, ok := canonicalImage[norm]; !ok {
			continue
		}
		var targets []string
		for t := range canonicalImage {
			if t != norm {
				targets = append(targets, t)
			}
		}
		sort.Strings(targets)
		ImageConversions[ext] = targets
	}

	VideoConversions = make(map[string][]string)
	for ext := range shared.VideoExtensions {
		norm := shared.NormaliseExt(ext)
		var targets []string
		for _, t := range allVideoTargets {
			if t != norm {
				targets = append(targets, t)
			}
		}
		VideoConversions[ext] = targets
	}
}

// GetCompatibleTargets returns the list of extensions a source can convert to.
func GetCompatibleTargets(sourcePath string) []string {
	ext := shared.NormaliseExt(extOf(sourcePath))
	if targets, ok := ImageConversions[ext]; ok {
		return targets
	}
	if targets, ok := VideoConversions[ext]; ok {
		return targets
	}
	return nil
}

func extOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[i:]
		}
	}
	return ""
}
