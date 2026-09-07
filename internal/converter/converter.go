package converter

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/exterex/morphic/internal/shared"
)

// HWAccelProfile defines a detected hardware acceleration encoder profile.
type HWAccelProfile struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"` // "nvenc", "qsv", "amf", "vaapi", "videotoolbox"
	Encoders []string `json:"encoders"`
}

// probeVideoBitrate returns the total bitrate (bits/s) of source, or 0 on failure.
func probeVideoBitrate(ctx context.Context, source, ffmpegBin string) int64 {
	probeBin := strings.Replace(ffmpegBin, "ffmpeg", "ffprobe", 1)
	if _, err := exec.LookPath(probeBin); err != nil {
		return 0
	}
	src := shared.PathForBin(probeBin, source)
	out, err := exec.CommandContext(ctx, probeBin,
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
func ffmpegHasEncoder(ctx context.Context, bin, encoder string) bool {
	out, err := exec.CommandContext(ctx, bin, "-hide_banner", "-encoders").Output()
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

// DetectAvailableHWAccels probes available GPU hardware encoders.
func DetectAvailableHWAccels(ctx context.Context) []HWAccelProfile {
	bin := "ffmpeg"
	if candidates := shared.FFmpegCandidates(); len(candidates) > 0 {
		bin = candidates[0]
	} else {
		return nil
	}

	var profiles []HWAccelProfile

	// NVENC (NVIDIA)
	var nvencEncoders []string
	for _, enc := range []string{"h264_nvenc", "hevc_nvenc", "av1_nvenc"} {
		if ffmpegHasEncoder(ctx, bin, enc) {
			nvencEncoders = append(nvencEncoders, enc)
		}
	}
	if len(nvencEncoders) > 0 {
		profiles = append(profiles, HWAccelProfile{
			Name:     "NVIDIA NVENC",
			Type:     "nvenc",
			Encoders: nvencEncoders,
		})
	}

	// QSV (Intel QuickSync)
	var qsvEncoders []string
	for _, enc := range []string{"h264_qsv", "hevc_qsv", "av1_qsv"} {
		if ffmpegHasEncoder(ctx, bin, enc) {
			qsvEncoders = append(qsvEncoders, enc)
		}
	}
	if len(qsvEncoders) > 0 {
		profiles = append(profiles, HWAccelProfile{
			Name:     "Intel QuickSync (QSV)",
			Type:     "qsv",
			Encoders: qsvEncoders,
		})
	}

	// AMF (AMD)
	var amfEncoders []string
	for _, enc := range []string{"h264_amf", "hevc_amf", "av1_amf"} {
		if ffmpegHasEncoder(ctx, bin, enc) {
			amfEncoders = append(amfEncoders, enc)
		}
	}
	if len(amfEncoders) > 0 {
		profiles = append(profiles, HWAccelProfile{
			Name:     "AMD AMF",
			Type:     "amf",
			Encoders: amfEncoders,
		})
	}

	// VAAPI (Linux)
	var vaapiEncoders []string
	for _, enc := range []string{"h264_vaapi", "hevc_vaapi", "av1_vaapi"} {
		if ffmpegHasEncoder(ctx, bin, enc) {
			vaapiEncoders = append(vaapiEncoders, enc)
		}
	}
	if len(vaapiEncoders) > 0 {
		profiles = append(profiles, HWAccelProfile{
			Name:     "VAAPI",
			Type:     "vaapi",
			Encoders: vaapiEncoders,
		})
	}

	// VideoToolbox (Apple)
	var vtEncoders []string
	for _, enc := range []string{"h264_videotoolbox", "hevc_videotoolbox"} {
		if ffmpegHasEncoder(ctx, bin, enc) {
			vtEncoders = append(vtEncoders, enc)
		}
	}
	if len(vtEncoders) > 0 {
		profiles = append(profiles, HWAccelProfile{
			Name:     "Apple VideoToolbox",
			Type:     "videotoolbox",
			Encoders: vtEncoders,
		})
	}

	return profiles
}

// getVideoEncoder returns the ffmpeg encoder name for the given codec ID and requested hardware accelerator.
func getVideoEncoder(ctx context.Context, codec, hwaccel string) (string, error) {
	bin := "ffmpeg"
	if candidates := shared.FFmpegCandidates(); len(candidates) > 0 {
		bin = candidates[0]
	}

	hwaccel = strings.ToLower(strings.TrimSpace(hwaccel))
	if hwaccel == "auto" {
		// Try best available hardware encoder
		for _, hw := range []string{"nvenc", "qsv", "amf", "vaapi", "videotoolbox"} {
			if enc, err := getHWEncoderName(ctx, bin, codec, hw); err == nil && enc != "" {
				return enc, nil
			}
		}
	} else if hwaccel != "" {
		if enc, err := getHWEncoderName(ctx, bin, codec, hwaccel); err == nil && enc != "" {
			return enc, nil
		}
	}

	// Fallback to software encoders
	switch codec {
	case "h264":
		return "libx264", nil
	case "h265":
		return "libx265", nil
	case "av1":
		for _, enc := range []string{"libsvtav1", "libaom-av1"} {
			if ffmpegHasEncoder(ctx, bin, enc) {
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

func getHWEncoderName(ctx context.Context, bin, codec, hw string) (string, error) {
	switch hw {
	case "nvenc":
		switch codec {
		case "h264":
			if ffmpegHasEncoder(ctx, bin, "h264_nvenc") {
				return "h264_nvenc", nil
			}
		case "h265":
			if ffmpegHasEncoder(ctx, bin, "hevc_nvenc") {
				return "hevc_nvenc", nil
			}
		case "av1":
			if ffmpegHasEncoder(ctx, bin, "av1_nvenc") {
				return "av1_nvenc", nil
			}
		}
	case "qsv":
		switch codec {
		case "h264":
			if ffmpegHasEncoder(ctx, bin, "h264_qsv") {
				return "h264_qsv", nil
			}
		case "h265":
			if ffmpegHasEncoder(ctx, bin, "hevc_qsv") {
				return "hevc_qsv", nil
			}
		case "av1":
			if ffmpegHasEncoder(ctx, bin, "av1_qsv") {
				return "av1_qsv", nil
			}
		}
	case "amf":
		switch codec {
		case "h264":
			if ffmpegHasEncoder(ctx, bin, "h264_amf") {
				return "h264_amf", nil
			}
		case "h265":
			if ffmpegHasEncoder(ctx, bin, "hevc_amf") {
				return "hevc_amf", nil
			}
		case "av1":
			if ffmpegHasEncoder(ctx, bin, "av1_amf") {
				return "av1_amf", nil
			}
		}
	case "vaapi":
		switch codec {
		case "h264":
			if ffmpegHasEncoder(ctx, bin, "h264_vaapi") {
				return "h264_vaapi", nil
			}
		case "h265":
			if ffmpegHasEncoder(ctx, bin, "hevc_vaapi") {
				return "hevc_vaapi", nil
			}
		case "av1":
			if ffmpegHasEncoder(ctx, bin, "av1_vaapi") {
				return "av1_vaapi", nil
			}
		}
	case "videotoolbox":
		switch codec {
		case "h264":
			if ffmpegHasEncoder(ctx, bin, "h264_videotoolbox") {
				return "h264_videotoolbox", nil
			}
		case "h265":
			if ffmpegHasEncoder(ctx, bin, "hevc_videotoolbox") {
				return "hevc_videotoolbox", nil
			}
		}
	}
	return "", fmt.Errorf("hwaccel %s not available for codec %s", hw, codec)
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

// ConvertImage converts an image file using the imaging library or ffmpeg.
func ConvertImage(ctx context.Context, source, targetExt, outputDir string) (string, error) {
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
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return "", fmt.Errorf("creating output directory: %w", err)
		}
		dest = filepath.Join(outputDir, stem+ext)
	} else {
		dest = filepath.Join(filepath.Dir(source), stem+ext)
	}

	// Avoid overwriting
	if _, err := os.Stat(dest); err == nil {
		dest = filepath.Join(filepath.Dir(dest),
			strings.TrimSuffix(filepath.Base(dest), ext)+"_converted"+ext)
	}

	sourceExt := shared.NormaliseExt(filepath.Ext(source))
	if sourceExt == ".avif" || ext == ".avif" {
		return convertImageByFFmpeg(ctx, source, dest, ext)
	}

	img, err := imaging.Open(source)
	if err != nil {
		// Fallback to ffmpeg conversion for formats imaging cannot decode
		return convertImageByFFmpeg(ctx, source, dest, ext)
	}

	opts := []imaging.EncodeOption{}
	extLower := strings.ToLower(ext)
	if extLower == ".jpg" || extLower == ".jpeg" {
		opts = append(opts, imaging.JPEGQuality(95))
	}

	if err := imaging.Save(img, dest, opts...); err != nil {
		// Fallback to ffmpeg for formats imaging can't encode
		return convertImageByFFmpeg(ctx, source, dest, ext)
	}

	return dest, nil
}

func convertImageByFFmpeg(ctx context.Context, source, dest, ext string) (string, error) {
	candidates := shared.FFmpegCandidates()
	if len(candidates) == 0 {
		return "", fmt.Errorf("ffmpeg is not installed or not on PATH")
	}

	extLower := strings.ToLower(ext)

	var lastErr error
	for _, bin := range candidates {
		src := shared.PathForBin(bin, source)
		dst := shared.PathForBin(bin, dest)
		args := []string{"-y", "-i", src}

		if extLower == ".avif" {
			args = append(args, "-vf", "crop=trunc(iw/2)*2:trunc(ih/2)*2,format=yuv420p")
			if ffmpegHasEncoder(ctx, bin, "libsvtav1") {
				args = append(args, "-c:v", "libsvtav1", "-crf", "28", "-preset", "8")
			} else if ffmpegHasEncoder(ctx, bin, "libaom-av1") {
				args = append(args, "-c:v", "libaom-av1", "-crf", "28", "-cpu-used", "4")
			} else {
				args = append(args, "-c:v", "libx264")
			}
		} else if extLower == ".webp" {
			args = append(args, "-c:v", "libwebp")
		}

		args = append(args, dst)

		out, err := exec.CommandContext(ctx, bin, args...).CombinedOutput()
		if err == nil {
			return dest, nil
		}
		lastErr = fmt.Errorf("ffmpeg image conversion failed: %s (err: %w)", strings.TrimSpace(string(out)), err)
	}
	return "", lastErr
}

// ConvertVideo converts a video file using ffmpeg with optional GPU hardware acceleration.
func ConvertVideo(ctx context.Context, source, targetExt, codec, hwaccel, outputDir string, av1CRF int) (string, error) {
	if !filepath.IsAbs(source) || strings.Contains(source, "\x00") {
		return "", fmt.Errorf("invalid source path")
	}
	candidates := shared.FFmpegCandidates()
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
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return "", fmt.Errorf("creating output directory: %w", err)
		}
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

	encoder, err := getVideoEncoder(ctx, codec, hwaccel)
	if err != nil {
		return "", err
	}

	cmd := []string{bin, "-y", "-i", shared.PathForBin(bin, source), "-c:v", encoder, "-c:a", "aac"}

	isAV1 := strings.Contains(encoder, "av1")
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
	case "av1_nvenc", "av1_qsv", "av1_amf", "av1_vaapi":
		cmd = append(cmd, "-preset", "fast")
	case "libvpx", "libvpx-vp9":
		crf := 35
		if av1CRF >= 10 && av1CRF <= 63 {
			crf = av1CRF
		}
		cmd = append(cmd, "-crf", fmt.Sprintf("%d", crf), "-b:v", "0")
	case "libx265", "hevc_nvenc", "hevc_qsv", "hevc_amf", "hevc_vaapi":
		cmd = append(cmd, "-preset", "fast")
	default:
		cmd = append(cmd, "-preset", "fast")
	}

	// For AV1, cap output bitrate at 65% of source to guarantee a size reduction.
	if isAV1 {
		if br := probeVideoBitrate(ctx, source, bin); br > 0 {
			maxrate := br * 65 / 100
			cmd = append(cmd, "-maxrate", fmt.Sprintf("%d", maxrate),
				"-bufsize", fmt.Sprintf("%d", br*2))
		}
	}

	cmd = append(cmd, shared.PathForBin(bin, dest))

	out, err2 := exec.CommandContext(ctx, cmd[0], cmd[1:]...).CombinedOutput()
	if err2 != nil {
		return "", fmt.Errorf("ffmpeg error: %s (err: %w)", strings.TrimSpace(string(out)), err2)
	}

	return dest, nil
}

// ConvertFile is the high-level converter — routes to image or video handler.
func ConvertFile(ctx context.Context, source, targetExt, codec, hwaccel, outputDir string, av1CRF int) (string, error) {
	if shared.IsImage(source) {
		return ConvertImage(ctx, source, targetExt, outputDir)
	}
	if shared.IsVideo(source) {
		return ConvertVideo(ctx, source, targetExt, codec, hwaccel, outputDir, av1CRF)
	}
	return "", fmt.Errorf("unsupported file type: %s", source)
}

func normaliseTargetExt(ext string) string {
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return shared.NormaliseExt(ext)
}
