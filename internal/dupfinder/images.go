package dupfinder

import (
	"context"
	"log"
	"math/bits"
	"os"
	"sort"
	"sync"

	"github.com/corona10/goimagehash"
	"github.com/andreassag/morphic/internal/shared"
)

// ImageInfo stores information about an image file.
type ImageInfo struct {
	Path     string `json:"path"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileSize int64  `json:"file_size"`
	Format   string `json:"format"`
	PHash    uint64 `json:"-"`
	AHash    uint64 `json:"-"`
	DHash    uint64 `json:"-"`
	HasHash  bool   `json:"-"`
}

// ComputeImageHashes loads an image and computes perceptual hashes.
func ComputeImageHashes(path string) ImageInfo {
	info := ImageInfo{Path: path}

	st, err := os.Stat(path)
	if err != nil {
		return info
	}
	info.FileSize = st.Size()

	img, err := shared.OpenImageFile(path)
	if err != nil {
		log.Printf("dupfinder: cannot open image %s: %v", path, err)
		return info
	}

	bounds := img.Bounds()
	info.Width = bounds.Dx()
	info.Height = bounds.Dy()

	ph, err := goimagehash.PerceptionHash(img)
	if err == nil {
		info.PHash = ph.GetHash()
	}
	ah, err := goimagehash.AverageHash(img)
	if err == nil {
		info.AHash = ah.GetHash()
	}
	dh, err := goimagehash.DifferenceHash(img)
	if err == nil {
		info.DHash = dh.GetHash()
	}

	info.HasHash = info.PHash != 0 || info.AHash != 0 || info.DHash != 0
	return info
}

// ProcessImages hashes all images concurrently and returns successful results.
// It stops accepting new work when ctx is cancelled.
func ProcessImages(ctx context.Context, files []shared.FileInfo, numWorkers int) map[string]*ImageInfo {
	result := make(map[string]*ImageInfo)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, numWorkers)

	for _, f := range files {
		select {
		case <-ctx.Done():
			wg.Wait()
			return result
		default:
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(fi shared.FileInfo) {
			defer wg.Done()
			defer func() { <-sem }()
			info := ComputeImageHashes(fi.Path)
			if info.HasHash {
				mu.Lock()
				result[fi.Path] = &info
				mu.Unlock()
			}
		}(f)
	}
	wg.Wait()
	return result
}

// hashSimilarity returns the similarity (0-1) between two 64-bit hashes.
func hashSimilarity(a, b uint64) float64 {
	dist := bits.OnesCount64(a ^ b)
	return 1.0 - float64(dist)/64.0
}

// ComputeSimilarity computes average similarity across phash, ahash, dhash.
func ComputeSimilarity(a, b *ImageInfo) float64 {
	var total, count float64
	if a.PHash != 0 && b.PHash != 0 {
		total += hashSimilarity(a.PHash, b.PHash)
		count++
	}
	if a.AHash != 0 && b.AHash != 0 {
		total += hashSimilarity(a.AHash, b.AHash)
		count++
	}
	if a.DHash != 0 && b.DHash != 0 {
		total += hashSimilarity(a.DHash, b.DHash)
		count++
	}
	if count == 0 {
		return 0
	}
	return total / count
}

// DuplicateEntry represents one file in a duplicate group.
type DuplicateEntry struct {
	Path       string  `json:"path"`
	Similarity float64 `json:"similarity"`
}

// FindImageDuplicates finds groups of duplicate images.
func FindImageDuplicates(infos map[string]*ImageInfo, threshold float64) [][]DuplicateEntry {
	// Bucket exact PHash matches first
	buckets := make(map[uint64][]string)
	for path, info := range infos {
		if info.PHash != 0 {
			buckets[info.PHash] = append(buckets[info.PHash], path)
		}
	}

	var groups [][]DuplicateEntry
	assigned := make(map[string]bool)

	// Exact hash groups
	for _, paths := range buckets {
		if len(paths) > 1 {
			sort.Strings(paths)
			var group []DuplicateEntry
			for _, p := range paths {
				group = append(group, DuplicateEntry{Path: p, Similarity: 1.0})
				assigned[p] = true
			}
			groups = append(groups, group)
		}
	}

	// Near-duplicate detection on remaining images
	var remaining []string
	for path := range infos {
		if !assigned[path] {
			remaining = append(remaining, path)
		}
	}
	sort.Strings(remaining)

	for i := 0; i < len(remaining); i++ {
		if assigned[remaining[i]] {
			continue
		}
		group := []DuplicateEntry{{Path: remaining[i], Similarity: 1.0}}

		for j := i + 1; j < len(remaining); j++ {
			if assigned[remaining[j]] {
				continue
			}
			sim := ComputeSimilarity(infos[remaining[i]], infos[remaining[j]])
			if sim >= threshold {
				group = append(group, DuplicateEntry{Path: remaining[j], Similarity: sim})
				assigned[remaining[j]] = true
			}
		}

		if len(group) > 1 {
			assigned[remaining[i]] = true
			groups = append(groups, group)
		}
	}

	return groups
}
