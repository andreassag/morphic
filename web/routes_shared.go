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
	path := c.Query("path")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		path = home
	}

	path = filepath.Clean(path)
	if !isAbsPath(path) {
		respondError(c, http.StatusBadRequest, "INVALID_PATH", "Invalid path")
		return
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		respondError(c, http.StatusBadRequest, "NOT_A_DIRECTORY", "Not a directory")
		return
	}

	entries, err := os.ReadDir(path)
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
		if e.IsDir() {
			dirs = append(dirs, dirEntry{
				Name: e.Name(),
				Path: filepath.Join(path, e.Name()),
				Type: "directory",
			})
		}
	}
	sort.Slice(dirs, func(i, j int) bool {
		return strings.ToLower(dirs[i].Name) < strings.ToLower(dirs[j].Name)
	})

	parent := filepath.Dir(path)
	var parentPtr interface{} = parent
	if parent == path {
		parentPtr = nil
	}

	c.JSON(http.StatusOK, gin.H{
		"current": path,
		"parent":  parentPtr,
		"entries": dirs,
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
	path := c.Query("path")
	if path == "" {
		c.Status(http.StatusBadRequest)
		return
	}

	var data []byte
	var err error

	path = filepath.Clean(path)
	if !isAbsPath(path) {
		c.Status(http.StatusBadRequest)
		return
	}

	if shared.IsVideo(path) {
		data, err = shared.GenerateVideoThumbnail(c.Request.Context(), path, shared.DefaultThumbnailSize)
	} else {
		data, err = shared.GenerateImageThumbnail(c.Request.Context(), path, shared.DefaultThumbnailSize)
	}

	if err != nil {
		respondError(c, http.StatusInternalServerError, "THUMBNAIL_FAILED", err.Error())
		return
	}

	c.Data(http.StatusOK, "image/jpeg", data)
}

func handleSystemInfo(c *gin.Context) {
	ffmpegInfo := gin.H{
		"installed":       false,
		"hwaccels":        []string{},
		"encoders":        []string{},
		"nvenc_available": false,
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
			for _, e := range encoders {
				if strings.Contains(e, "nvenc") {
					ffmpegInfo["nvenc_available"] = true
					break
				}
			}
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
	filePath := c.Query("path")
	if filePath == "" {
		c.Status(http.StatusBadRequest)
		return
	}

	filePath = filepath.Clean(filePath)
	if !isAbsPath(filePath) {
		c.Status(http.StatusBadRequest)
		return
	}
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		c.Status(http.StatusNotFound)
		return
	}

	ext := shared.NormaliseExt(filepath.Ext(filePath))
	_, isImg := shared.ImageExtensions[ext]
	_, isVid := shared.VideoExtensions[ext]
	if !isImg && !isVid {
		c.Status(http.StatusForbidden)
		return
	}

	c.File(filePath)
}

// isDir returns true when path exists and is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// isAbsPath rejects relative paths and paths containing null bytes.
func isAbsPath(p string) bool {
	return filepath.IsAbs(p) && !strings.Contains(p, "\x00")
}

// round1 rounds f to one decimal place.
func round1(f float64) float64 {
	return math.Round(f*10) / 10
}
