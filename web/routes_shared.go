package web

import (
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/exterex/morphic/internal/converter"
	"github.com/exterex/morphic/internal/shared"
	"github.com/gin-gonic/gin"
)

func registerSharedRoutes(r *gin.Engine) {
	r.GET("/api/browse", handleBrowseDirectory)
	r.POST("/api/browse/native", handleBrowseNative)
	r.GET("/api/thumbnail", handleThumbnail)
	r.GET("/api/system_info", handleSystemInfo)
	r.GET("/api/media", handleMedia)
}

// handleBrowseDirectory lists directories for the in-page folder browser.
func handleBrowseDirectory(c *gin.Context) {
	rawPath := c.Query("path")
	if rawPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		rawPath = home
	}

	safePath, err := shared.ValidateSafePath(rawPath)
	if err != nil {
		respondError(c, http.StatusBadRequest, "INVALID_PATH", err.Error())
		return
	}

	pathExists := true
	browseDir := safePath
	filterPrefix := ""

	info, err := os.Stat(safePath)
	if err != nil || !info.IsDir() {
		pathExists = false
		// If path doesn't exist as a directory, check parent directory for autocomplete matching
		parentDir := filepath.Dir(safePath)
		parentSafe, pErr := shared.ValidateSafeDirPath(parentDir)
		if pErr == nil {
			browseDir = parentSafe
			filterPrefix = strings.ToLower(filepath.Base(safePath))
		} else {
			respondError(c, http.StatusBadRequest, "NOT_A_DIRECTORY", "Not a directory")
			return
		}
	}

	entries, err := os.ReadDir(browseDir)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "READ_DIR_FAILED", err.Error())
		return
	}

	type dirEntry struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
	}
	var dirs []dirEntry
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if filterPrefix != "" && !strings.HasPrefix(strings.ToLower(e.Name()), filterPrefix) {
			continue
		}
		if e.IsDir() {
			dirs = append(dirs, dirEntry{
				Name: e.Name(),
				Path: filepath.Join(browseDir, e.Name()),
				Type: "directory",
			})
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})

	parent := filepath.Dir(browseDir)
	var parentPtr interface{} = parent
	if parent == browseDir {
		parentPtr = nil
	}

	c.JSON(http.StatusOK, gin.H{
		"current": browseDir,
		"parent":  parentPtr,
		"entries": dirs,
		"exists":  pathExists,
	})
}

// handleBrowseNative opens the OS-native folder picker dialog.
func handleBrowseNative(c *gin.Context) {
	folder, available, err := shared.OpenNativeFolderDialog(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "NATIVE_DIALOG_ERROR", err.Error())
		return
	}
	if !available {
		c.JSON(http.StatusOK, gin.H{
			"folder":    nil,
			"available": false,
		})
		return
	}
	if folder == "" {
		c.JSON(http.StatusOK, gin.H{
			"folder":    nil,
			"available": true,
			"cancelled": true,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"folder": folder, "available": true})
}

func handleThumbnail(c *gin.Context) {
	rawPath := c.Query("path")
	if rawPath == "" {
		c.Status(http.StatusBadRequest)
		return
	}

	filePath, err := shared.ValidateMediaFilePath(rawPath)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	var data []byte
	if shared.IsVideo(filePath) {
		data, err = shared.GenerateVideoThumbnail(c.Request.Context(), filePath, shared.DefaultThumbnailSize)
	} else {
		data, err = shared.GenerateImageThumbnail(c.Request.Context(), filePath, shared.DefaultThumbnailSize)
	}

	if err != nil {
		respondError(c, http.StatusInternalServerError, "THUMBNAIL_FAILED", err.Error())
		return
	}

	c.Data(http.StatusOK, "image/jpeg", data)
}

func handleSystemInfo(c *gin.Context) {
	ffmpegInfo := gin.H{
		"installed": false,
		"encoders":  []string{},
		"profiles":  converter.DetectAvailableHWAccels(c.Request.Context()),
	}

	candidates := shared.FFmpegCandidates()
	if len(candidates) > 0 {
		bin := candidates[0]
		ffmpegInfo["installed"] = true

		if out, err := exec.CommandContext(c.Request.Context(), bin, "-hide_banner", "-encoders").
			CombinedOutput(); err == nil {
			var encoders []string
			for _, line := range strings.Split(string(out), "\n") {
				line = strings.TrimSpace(line)
				if len(line) > 0 && (line[0] == 'V' || line[0] == 'A') {
					encoders = append(encoders, line)
				}
			}
			ffmpegInfo["encoders"] = encoders
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"version":  shared.Version,
		"platform": runtime.GOOS,
		"arch":     runtime.GOARCH,
		"go":       runtime.Version(),
		"cpus":     runtime.NumCPU(),
		"ffmpeg":   ffmpegInfo,
	})
}

// handleMedia serves a media file for full-size preview.
func handleMedia(c *gin.Context) {
	rawPath := c.Query("path")
	if rawPath == "" {
		c.Status(http.StatusBadRequest)
		return
	}

	filePath, err := shared.ValidateMediaFilePath(rawPath)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	c.File(filePath)
}

// isDir returns true when path exists, is a directory, and is not a protected system directory.
func isDir(path string) bool {
	safe, err := shared.ValidateSafeDirPath(path)
	return err == nil && safe != ""
}

// expandPath cleans the path and expands a leading tilde (~) to the user's home directory.
func expandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if p == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Clean(home)
		}
	} else if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Clean(filepath.Join(home, p[2:]))
		}
	}
	return filepath.Clean(p)
}

// isAbsPath verifies that p is a valid, clean absolute path and not in a protected system directory.
func isAbsPath(p string) bool {
	safe, err := shared.ValidateSafePath(p)
	return err == nil && safe != ""
}

// round1 rounds f to one decimal place.
func round1(f float64) float64 {
	return math.Round(f*10) / 10
}
