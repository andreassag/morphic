package shared

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// FileInfo holds metadata about a discovered file.
type FileInfo struct {
	Path string
	Name string
	Size int64
	Ext  string
}

// FindFilesByExtension walks the folder tree once and returns all files
// matching the given extensions. This replaces the Python version which
// called rglob() twice per extension (82+ traversals).
func FindFilesByExtension(folder string, extensions map[string]struct{}, excludedFolders map[string]struct{}) ([]FileInfo, error) {
	var files []FileInfo
	seen := make(map[string]struct{})

	err := filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}

		if d.IsDir() {
			name := strings.ToLower(d.Name())
			if _, excluded := excludedFolders[name]; excluded {
				return filepath.SkipDir
			}
			return nil
		}

		ext := NormaliseExt(filepath.Ext(path))
		if _, ok := extensions[ext]; !ok {
			return nil
		}

		// Deduplicate by resolved path
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		if _, dup := seen[abs]; dup {
			return nil
		}
		seen[abs] = struct{}{}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		files = append(files, FileInfo{
			Path: abs,
			Name: d.Name(),
			Size: info.Size(),
			Ext:  ext,
		})

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking folder %s: %w", folder, err)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return files, nil
}

// IsExcludedPath checks if any component of the path is in the exclusion set.
func IsExcludedPath(path string, excludedFolders map[string]struct{}) bool {
	parts := strings.Split(filepath.ToSlash(path), "/")
	for _, part := range parts {
		if _, excluded := excludedFolders[strings.ToLower(part)]; excluded {
			return true
		}
	}
	return false
}

// FindImageFiles returns all image files in the given folder.
func FindImageFiles(folder string) ([]FileInfo, error) {
	return FindFilesByExtension(folder, ImageExtensions, ExcludedFolders)
}

// FindVideoFiles returns all video files in the given folder.
func FindVideoFiles(folder string) ([]FileInfo, error) {
	return FindFilesByExtension(folder, VideoExtensions, ExcludedFolders)
}

// FindAllMediaFiles returns all image and video files.
func FindAllMediaFiles(folder string) ([]FileInfo, error) {
	allExts := make(map[string]struct{})
	for k, v := range ImageExtensions {
		allExts[k] = v
	}
	for k, v := range VideoExtensions {
		allExts[k] = v
	}
	return FindFilesByExtension(folder, allExts, ExcludedFolders)
}

// FormatFileSize formats file size in human-readable format.
func FormatFileSize(sizeBytes int64) string {
	size := float64(sizeBytes)
	for _, unit := range []string{"B", "KB", "MB", "GB"} {
		if size < 1024 {
			return fmt.Sprintf("%.2f %s", size, unit)
		}
		size /= 1024
	}
	return fmt.Sprintf("%.2f TB", size)
}

// NormaliseExt lowercases and resolves aliases (.jpeg → .jpg).
func NormaliseExt(ext string) string {
	ext = strings.ToLower(ext)
	if alias, ok := Aliases[ext]; ok {
		return alias
	}
	return ext
}

// IsImage returns true if the file extension is a known image type.
func IsImage(path string) bool {
	ext := NormaliseExt(filepath.Ext(path))
	_, ok := ImageExtensions[ext]
	return ok
}

// IsImageFile is an alias for IsImage.
func IsImageFile(path string) bool {
	return IsImage(path)
}

// IsVideo returns true if the file extension is a known video type.
func IsVideo(path string) bool {
	ext := NormaliseExt(filepath.Ext(path))
	_, ok := VideoExtensions[ext]
	return ok
}

// IsVideoFile is an alias for IsVideo.
func IsVideoFile(path string) bool {
	return IsVideo(path)
}

// FormatDuration formats duration in human-readable format.
func FormatDuration(seconds float64) string {
	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, secs)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, secs)
	}
	return fmt.Sprintf("%ds", secs)
}

// ToWindowsPath converts a WSL /mnt/X/... path to a Windows X:\... path.
// Windows-native executables (e.g. ffmpeg.exe) cannot access /mnt/ paths directly.
func ToWindowsPath(p string) string {
	if !strings.HasPrefix(p, "/mnt/") || len(p) < 7 {
		return p
	}
	rest := p[5:] // strip "/mnt/"
	slash := strings.IndexByte(rest, '/')
	var drive, tail string
	if slash == -1 {
		drive = rest
		tail = ""
	} else {
		drive = rest[:slash]
		tail = rest[slash+1:]
	}
	if len(drive) != 1 {
		return p
	}
	return strings.ToUpper(drive) + ":\\" + strings.ReplaceAll(tail, "/", "\\")
}

// PathForBin returns the path in the format expected by the given binary.
// When bin is a Windows executable (.exe), WSL /mnt/ paths are converted.
func PathForBin(bin, p string) string {
	if strings.HasSuffix(strings.ToLower(bin), ".exe") {
		return ToWindowsPath(p)
	}
	return p
}

// FFmpegCandidates returns available ffmpeg binary names in PATH.
func FFmpegCandidates() []string {
	var bins []string
	for _, name := range []string{"ffmpeg", "ffmpeg.exe"} {
		if _, err := exec.LookPath(name); err == nil {
			bins = append(bins, name)
		}
	}
	return bins
}
