package converter

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/andreassag/morphic/internal/shared"
)

// toWindowsPath converts a WSL /mnt/X/... path to a Windows X:\... path.
// Windows-native executables (e.g. ffmpeg.exe) cannot access /mnt/ paths directly.
// Paths not matching /mnt/<drive>/ are returned unchanged.
func toWindowsPath(p string) string {
	// /mnt/d/foo/bar → D:\foo\bar
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

// pathForBin returns the path in the format expected by the given binary.
// When bin is a Windows executable (.exe), WSL /mnt/ paths are converted.
func pathForBin(bin, p string) string {
	if strings.HasSuffix(strings.ToLower(bin), ".exe") {
		return toWindowsPath(p)
	}
	return p
}

// On WSL2, ffmpeg.exe (Windows build) supports AVIF output; /usr/bin/ffmpeg does not.
func ffmpegCandidates() []string {
	var bins []string
	for _, name := range []string{"ffmpeg", "ffmpeg.exe"} {
		if _, err := exec.LookPath(name); err == nil {
			bins = append(bins, name)
		}
	}
	return bins
}

// probeVideoBitrate returns the total bitrate (bits/s) of source, or 0 on failure.
// It derives the ffprobe binary from the ffmpeg binary (ffmpeg → ffprobe, ffmpeg.exe → ffprobe.exe).
func probeVideoBitrate(source, ffmpegBin string) int64 {
	probeBin := strings.Replace(ffmpegBin, "ffmpeg", "ffprobe", 1)
	if _, err := exec.LookPath(probeBin); err != nil {
		return 0
	}
	src := pathForBin(probeBin, source)
	out, err := exec.Command(probeBin,
		"-v", "quiet",
		"-show_entries", "format=bit_rate",
		"-of", "default=noprint_wrappers=1",
		src).Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "bit_rate=") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "bit_rate="))
			if n, err := strconv.ParseInt(val, 10, 64); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

// ffmpegHasEncoder checks if the given binary has a particular encoder.
func ffmpegHasEncoder(bin, encoder string) bool {
	out, err := exec.Command(bin, "-hide_banner", "-encoders").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, encoder) {
			return true
		}
	}
	return false
}

// getVideoEncoder returns the ffmpeg encoder name for the given codec ID.
// Codec IDs: h264, h265, av1, vp8, vp9.
func getVideoEncoder(codec string) (string, error) {
	bin := "ffmpeg"
	if candidates := ffmpegCandidates(); len(candidates) > 0 {
		bin = candidates[0]
	}

	switch codec {
	case "h264":
		return "libx264", nil
	case "h265":
		return "libx265", nil
	case "av1":
		for _, enc := range []string{"libsvtav1", "libaom-av1"} {
			if ffmpegHasEncoder(bin, enc) {
				return enc, nil
			}
		}
		return "", fmt.Errorf("no AV1 encoder available (libsvtav1 or libaom-av1 required)")
	case "vp8":
		return "libvpx", nil
	case "vp9":
		return "libvpx-vp9", nil
	}
	return "", fmt.Errorf("unknown codec: %s", codec)
}

func validateImageTargetExt(targetExt string) (string, error) {
	if targetExt == "" {
		return "", fmt.Errorf("invalid target extension")
	}
	if strings.Contains(targetExt, "\x00") || strings.ContainsAny(targetExt, `/\`) || strings.Contains(targetExt, "..") {
		return "", fmt.Errorf("invalid target extension")
	}
	for _, r := range targetExt {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("invalid target extension")
		}
	}

	ext := shared.NormaliseExt(normaliseTargetExt(targetExt))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".tif", ".tiff", ".webp", ".avif":
		return ext, nil
	default:
		return "", fmt.Errorf("unsupported target extension: %s", targetExt)
	}
}

func validateVideoTargetExt(targetExt string) (string, error) {
	if targetExt == "" {
		return "", fmt.Errorf("invalid target extension")
	}
	if strings.Contains(targetExt, "\x00") || strings.ContainsAny(targetExt, `/\`) || strings.Contains(targetExt, "..") {
		return "", fmt.Errorf("invalid target extension")
	}
	for _, r := range targetExt {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("invalid target extension")
		}
	}

	ext := shared.NormaliseExt(normaliseTargetExt(targetExt))
	if _, ok := canonicalVideo[ext]; ok {
		return ext, nil
	}
	return "", fmt.Errorf("unsupported target extension: %s", targetExt)
}

// IsValidTargetExt reports whether ext is a recognised image or video output extension.
func IsValidTargetExt(ext string) bool {
	_, imgErr := validateImageTargetExt(ext)
	if imgErr == nil {
		return true
	}
	_, vidErr := validateVideoTargetExt(ext)
	return vidErr == nil
}

// ConvertImage converts an image file using the imaging library.
func ConvertImage(source, targetExt, outputDir string) (string, error) {
	if !filepath.IsAbs(source) || strings.Contains(source, "\x00") {
		return "", fmt.Errorf("invalid source path")
	}
	ext, err := validateImageTargetExt(targetExt)
	if err != nil {
		return "", err
	}

	stem := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	var dest string
	if outputDir != "" {
		os.MkdirAll(outputDir, 0755)
		dest = filepath.Join(outputDir, stem+ext)
	} else {
		dest = filepath.Join(filepath.Dir(source), stem+ext)
	}

	// Avoid overwriting
	if _, err := os.Stat(dest); err == nil {
		dest = filepath.Join(filepath.Dir(dest),
			strings.TrimSuffix(filepath.Base(dest), ext)+"_converted"+ext)
	}

	sourceExt := shared.NormaliseExt(strings.ToLower(filepath.Ext(source)))
	if sourceExt == ".avif" || ext == ".avif" {
		return convertImageByFFmpeg(source, dest, ext)
	}

	img, err := imaging.Open(source)
	if err != nil {
		// Relax: fallback to ffmpeg conversion for special unsupported formats
		return convertImageByFFmpeg(source, dest, ext)
	}

	opts := []imaging.EncodeOption{}
	extLower := strings.ToLower(ext)
	if extLower == ".jpg" || extLower == ".jpeg" {
		opts = append(opts, imaging.JPEGQuality(95))
	}

	if err := imaging.Save(img, dest, opts...); err != nil {
		// Fallback to ffmpeg for formats imaging can't encode
		return convertImageByFFmpeg(source, dest, ext)
	}

	return dest, nil
}

func convertImageByFFmpeg(source, dest, ext string) (string, error) {
	candidates := ffmpegCandidates()
	if len(candidates) == 0 {
		return "", fmt.Errorf("ffmpeg is not installed or not on PATH")
	}

	extLower := strings.ToLower(ext)

	var lastErr error
	for _, bin := range candidates {
		src := pathForBin(bin, source)
		dst := pathForBin(bin, dest)
		args := []string{"-y", "-i", src}

		if extLower == ".avif" {
			// AV1 (YUV 4:2:0) requires even dimensions and no alpha channel.
			// crop: trim 1px from odd dimensions. format=yuv420p: strip alpha (rgba → yuv420p).
			args = append(args, "-vf", "crop=trunc(iw/2)*2:trunc(ih/2)*2,format=yuv420p")
			if ffmpegHasEncoder(bin, "libsvtav1") {
				args = append(args, "-c:v", "libsvtav1", "-crf", "28", "-preset", "8")
			} else if ffmpegHasEncoder(bin, "libaom-av1") {
				args = append(args, "-c:v", "libaom-av1", "-crf", "28", "-cpu-used", "4")
			} else {
				args = append(args, "-c:v", "libx264")
			}
		} else if extLower == ".webp" {
			args = append(args, "-c:v", "libwebp")
		} else if extLower == ".png" || extLower == ".jpg" || extLower == ".jpeg" || extLower == ".bmp" || extLower == ".gif" {
			// no explicit codec required
		} else {
			// generic fallback for unknown image extensions
		}

		args = append(args, dst)

		out, err := exec.Command(bin, args...).CombinedOutput()
		if err == nil {
			return dest, nil
		}
		lastErr = fmt.Errorf("ffmpeg image conversion failed: %s", strings.TrimSpace(string(out)))
	}
	return "", lastErr
}

// ConvertVideo converts a video file using ffmpeg.
// codec is one of: h264, h265, av1, vp8, vp9. Defaults to h264 when empty.
func ConvertVideo(source, targetExt, codec, outputDir string, av1CRF int) (string, error) {
	if !filepath.IsAbs(source) || strings.Contains(source, "\x00") {
		return "", fmt.Errorf("invalid source path")
	}
	candidates := ffmpegCandidates()
	if len(candidates) == 0 {
		return "", fmt.Errorf("ffmpeg is not installed or not on PATH")
	}
	bin := candidates[0]

	ext, err := validateVideoTargetExt(targetExt)
	if err != nil {
		return "", err
	}

	stem := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	var dest string
	if outputDir != "" {
		os.MkdirAll(outputDir, 0755)
		dest = filepath.Join(outputDir, stem+ext)
	} else {
		dest = filepath.Join(filepath.Dir(source), stem+ext)
	}

	// Avoid overwriting
	if _, err := os.Stat(dest); err == nil {
		dest = filepath.Join(filepath.Dir(dest),
			strings.TrimSuffix(filepath.Base(dest), ext)+"_converted"+ext)
	}

	if codec == "" {
		codec = "h264"
	}

	encoder, err := getVideoEncoder(codec)
	if err != nil {
		return "", err
	}

	cmd := []string{bin, "-y", "-i", pathForBin(bin, source), "-c:v", encoder, "-c:a", "aac"}

	isAV1 := encoder == "libsvtav1" || encoder == "libaom-av1"
	if isAV1 {
		// AV1 requires even dimensions for YUV 4:2:0
		cmd = append(cmd, "-vf", "crop=trunc(iw/2)*2:trunc(ih/2)*2")
	}

	switch encoder {
	case "libsvtav1":
		crf := 35
		if av1CRF >= 10 && av1CRF <= 63 {
			crf = av1CRF
		}
		cmd = append(cmd, "-preset", "8", "-crf", fmt.Sprintf("%d", crf))
	case "libaom-av1":
		crf := 35
		if av1CRF >= 10 && av1CRF <= 63 {
			crf = av1CRF
		}
		cmd = append(cmd, "-cpu-used", "4", "-crf", fmt.Sprintf("%d", crf))
	case "libvpx", "libvpx-vp9":
		crf := 35
		if av1CRF >= 10 && av1CRF <= 63 {
			crf = av1CRF
		}
		cmd = append(cmd, "-crf", fmt.Sprintf("%d", crf), "-b:v", "0")
	case "libx265":
		cmd = append(cmd, "-preset", "fast", "-crf", "28")
	default: // libx264
		cmd = append(cmd, "-preset", "fast", "-crf", "23")
	}

	// For AV1, cap output bitrate at 65% of source to guarantee a size reduction.
	if isAV1 {
		if br := probeVideoBitrate(source, bin); br > 0 {
			maxrate := br * 65 / 100
			cmd = append(cmd, "-maxrate", fmt.Sprintf("%d", maxrate),
				"-bufsize", fmt.Sprintf("%d", br*2))
		}
	}

	cmd = append(cmd, pathForBin(bin, dest))

	out, err2 := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
	if err2 != nil {
		return "", fmt.Errorf("ffmpeg error: %s", strings.TrimSpace(string(out)))
	}

	return dest, nil
}

// ConvertFile is the high-level converter — routes to image or video handler.
// codec is used only for video conversion (h264, h265, av1, vp8, vp9).
func ConvertFile(source, targetExt, codec, outputDir string, av1CRF int) (string, error) {
	if shared.IsImage(source) {
		return ConvertImage(source, targetExt, outputDir)
	}
	if shared.IsVideo(source) {
		return ConvertVideo(source, targetExt, codec, outputDir, av1CRF)
	}
	return "", fmt.Errorf("unsupported file type: %s", source)
}

func normaliseTargetExt(ext string) string {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return shared.NormaliseExt(ext)
}
