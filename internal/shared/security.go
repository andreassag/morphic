package shared

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	ErrEmptyPath        = errors.New("path cannot be empty")
	ErrNullByte         = errors.New("path contains null byte")
	ErrProtectedSystem  = errors.New("access to system directory is forbidden")
	ErrNotADirectory    = errors.New("path is not a directory")
	ErrNotARegularFile  = errors.New("path is not a regular file")
	ErrUnsupportedMedia = errors.New("file format is not a supported media type")
)

// protectedPrefixesUnix defines root-level OS directories that should never be accessed by Morphic.
var protectedPrefixesUnix = []string{
	"/bin",
	"/boot",
	"/dev",
	"/etc",
	"/lib",
	"/lib64",
	"/lost+found",
	"/opt",
	"/proc",
	"/root",
	"/run",
	"/sbin",
	"/sys",
	"/usr/bin",
	"/usr/sbin",
	"/var/lock",
	"/var/run",
}

// protectedPrefixesWindows defines Windows system paths that should never be accessed.
var protectedPrefixesWindows = []string{
	`c:\windows`,
	`c:\program files`,
	`c:\program files (x86)`,
	`c:\programdata`,
}

// IsProtectedSystemPath checks whether an absolute path falls into a forbidden OS system directory.
func IsProtectedSystemPath(cleanAbsPath string) bool {
	if runtime.GOOS == "windows" {
		lower := strings.ToLower(cleanAbsPath)
		for _, prefix := range protectedPrefixesWindows {
			if lower == prefix || strings.HasPrefix(lower, prefix+`\`) {
				return true
			}
		}
		return false
	}

	// Unix / Linux / macOS
	if cleanAbsPath == "/" {
		return true
	}
	for _, prefix := range protectedPrefixesUnix {
		if cleanAbsPath == prefix || strings.HasPrefix(cleanAbsPath, prefix+"/") {
			return true
		}
	}
	return false
}

// ValidateSafePath cleans and canonicalizes a path, rejecting null bytes, relative traversal,
// and protected system directories.
func ValidateSafePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrEmptyPath
	}
	if strings.Contains(raw, "\x00") {
		return "", ErrNullByte
	}

	// Expand leading tilde
	if raw == "~" || strings.HasPrefix(raw, "~/") || strings.HasPrefix(raw, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			if raw == "~" {
				raw = home
			} else {
				raw = filepath.Join(home, raw[2:])
			}
		}
	}

	clean := filepath.Clean(raw)
	abs, err := filepath.Abs(clean)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	if IsProtectedSystemPath(abs) {
		return "", ErrProtectedSystem
	}

	// If file or directory exists, check real path via EvalSymlinks to prevent symlink traversal
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		if IsProtectedSystemPath(resolved) {
			return "", ErrProtectedSystem
		}
	}

	return abs, nil
}

// ValidateSafeDirPath verifies that raw resolves to a safe, existing directory.
func ValidateSafeDirPath(raw string) (string, error) {
	safePath, err := ValidateSafePath(raw)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(safePath)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", ErrNotADirectory
	}

	return safePath, nil
}

// ValidateMediaFilePath verifies that raw resolves to a safe, existing regular file
// with an allowed media extension.
func ValidateMediaFilePath(raw string) (string, error) {
	safePath, err := ValidateSafePath(raw)
	if err != nil {
		return "", err
	}

	ext := NormaliseExt(filepath.Ext(safePath))
	_, isImg := ImageExtensions[ext]
	_, isVid := VideoExtensions[ext]
	if !isImg && !isVid {
		return "", ErrUnsupportedMedia
	}

	info, err := os.Stat(safePath)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", ErrNotARegularFile
	}

	return safePath, nil
}
