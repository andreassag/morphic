package trimmer

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/andreassag/morphic/internal/shared"
	"github.com/disintegration/imaging"
)

// ImageTransformOpts configures crop, rotate, flip, and resize operations on an image.
type ImageTransformOpts struct {
	SourcePath string `json:"source_path"`
	CropX      int    `json:"crop_x"`
	CropY      int    `json:"crop_y"`
	CropWidth  int    `json:"crop_width"`
	CropHeight int    `json:"crop_height"`
	Rotate     int    `json:"rotate"` // 0, 90, 180, 270
	FlipH      bool   `json:"flip_h"`
	FlipV      bool   `json:"flip_v"`
	ResizeW    int    `json:"resize_w"`
	ResizeH    int    `json:"resize_h"`
}

// VideoTrimOpts configures video clipping via ffmpeg.
type VideoTrimOpts struct {
	SourcePath string  `json:"source_path"`
	StartTime  float64 `json:"start_time"`
	EndTime    float64 `json:"end_time"`
	StreamCopy bool    `json:"stream_copy"`
}

// ExportOpts configures export for image or video trimming.
type ExportOpts struct {
	Type       string             `json:"type"` // "image" or "video"
	SourcePath string             `json:"source_path"`
	OutputPath string             `json:"output_path"`
	ImageOpts  ImageTransformOpts `json:"image_opts"`
	VideoOpts  VideoTrimOpts      `json:"video_opts"`
}

// ApplyImageTransform loads the source image, applies transformations, and returns the modified image.
func ApplyImageTransform(ctx context.Context, opts ImageTransformOpts) (image.Image, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	safeSrc, err := shared.ValidateMediaFilePath(opts.SourcePath)
	if err != nil {
		return nil, fmt.Errorf("validate source path: %w", err)
	}

	img, err := imaging.Open(safeSrc, imaging.AutoOrientation(true))
	if err != nil {
		return nil, fmt.Errorf("open image %s: %w", safeSrc, err)
	}

	// 1. Crop
	bounds := img.Bounds()
	if opts.CropWidth > 0 && opts.CropHeight > 0 {
		x0 := opts.CropX
		y0 := opts.CropY
		if x0 < bounds.Min.X {
			x0 = bounds.Min.X
		}
		if y0 < bounds.Min.Y {
			y0 = bounds.Min.Y
		}
		x1 := x0 + opts.CropWidth
		y1 := y0 + opts.CropHeight
		if x1 > bounds.Max.X {
			x1 = bounds.Max.X
		}
		if y1 > bounds.Max.Y {
			y1 = bounds.Max.Y
		}
		if x1 > x0 && y1 > y0 {
			img = imaging.Crop(img, image.Rect(x0, y0, x1, y1))
		}
	}

	// 2. Rotate
	switch opts.Rotate {
	case 90:
		img = imaging.Rotate270(img) // imaging rotate angle is counter-clockwise, so 90 deg cw = Rotate270
	case 180:
		img = imaging.Rotate180(img)
	case 270:
		img = imaging.Rotate90(img)
	}

	// 3. Flip
	if opts.FlipH {
		img = imaging.FlipH(img)
	}
	if opts.FlipV {
		img = imaging.FlipV(img)
	}

	// 4. Resize
	if opts.ResizeW > 0 || opts.ResizeH > 0 {
		img = imaging.Resize(img, opts.ResizeW, opts.ResizeH, imaging.Lanczos)
	}

	return img, nil
}

// PreviewImageTransform generates a JPEG preview byte buffer of the transformed image.
func PreviewImageTransform(ctx context.Context, opts ImageTransformOpts) ([]byte, error) {
	img, err := ApplyImageTransform(ctx, opts)
	if err != nil {
		return nil, err
	}

	// Downscale preview if larger than 1280px for fast response
	bounds := img.Bounds()
	if bounds.Dx() > 1280 || bounds.Dy() > 1280 {
		img = imaging.Fit(img, 1280, 1280, imaging.Linear)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("encode preview jpeg: %w", err)
	}
	return buf.Bytes(), nil
}

// ExportMedia saves transformed image or trimmed video to a target file.
func ExportMedia(ctx context.Context, opts ExportOpts) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	safeSrc, err := shared.ValidateMediaFilePath(opts.SourcePath)
	if err != nil {
		return "", fmt.Errorf("validate source path: %w", err)
	}

	outputPath := opts.OutputPath
	if outputPath == "" {
		ext := filepath.Ext(safeSrc)
		base := strings.TrimSuffix(filepath.Base(safeSrc), ext)
		outputPath = filepath.Join(filepath.Dir(safeSrc), base+"_trimmed"+ext)
	}

	safeOut, err := shared.ValidateSafePath(outputPath)
	if err != nil {
		return "", fmt.Errorf("validate output path: %w", err)
	}

	if safeOut == safeSrc {
		return "", fmt.Errorf("output path cannot overwrite original source")
	}

	if err := os.MkdirAll(filepath.Dir(safeOut), 0o755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	if opts.Type == "video" || shared.IsVideo(safeSrc) {
		candidates := shared.FFmpegCandidates()
		if len(candidates) == 0 {
			return "", fmt.Errorf("ffmpeg not found for video trimming")
		}
		bin := candidates[0]

		args := []string{"-y"}
		if opts.VideoOpts.StartTime > 0 {
			args = append(args, "-ss", fmt.Sprintf("%.3f", opts.VideoOpts.StartTime))
		}
		if opts.VideoOpts.EndTime > opts.VideoOpts.StartTime {
			args = append(args, "-to", fmt.Sprintf("%.3f", opts.VideoOpts.EndTime))
		}
		args = append(args, "-i", shared.PathForBin(bin, safeSrc))

		if opts.VideoOpts.StreamCopy {
			args = append(args, "-c", "copy")
		} else {
			args = append(args, "-c:v", "libx264", "-c:a", "aac")
		}
		args = append(args, shared.PathForBin(bin, safeOut))

		cmd := exec.CommandContext(ctx, bin, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("ffmpeg video trim failed: %w (output: %s)", err, string(out))
		}
		return safeOut, nil
	}

	// Image export
	opts.ImageOpts.SourcePath = safeSrc
	img, err := ApplyImageTransform(ctx, opts.ImageOpts)
	if err != nil {
		return "", err
	}

	if err := imaging.Save(img, safeOut); err != nil {
		return "", fmt.Errorf("save image to %s: %w", safeOut, err)
	}
	return safeOut, nil
}
