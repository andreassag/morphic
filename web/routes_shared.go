package web

import (
	"math"
	"net/http"
	"os/exec"
	"runtime"
	"strings"

	"github.com/andreassag/morphic/internal/converter"
	"github.com/andreassag/morphic/internal/shared"
	"github.com/gin-gonic/gin"
)

func registerSharedRoutes(r *gin.Engine) {
	registerBrowseRoutes(r)
	r.POST("/api/browse/native", handleBrowseNative)
	r.GET("/api/thumbnail", handleThumbnail)
	r.GET("/api/system_info", handleSystemInfo)
	r.GET("/api/media", handleMedia)
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

// isAbsPath verifies that p is a valid, clean absolute path and not in a protected system directory.
func isAbsPath(p string) bool {
	safe, err := shared.ValidateSafePath(p)
	return err == nil && safe != ""
}

// round1 rounds f to one decimal place.
func round1(f float64) float64 {
	return math.Round(f*10) / 10
}
