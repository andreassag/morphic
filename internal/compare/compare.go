package compare

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/exterex/morphic/internal/shared"
)

// MediaItemMeta holds metadata for one side of a comparison.
type MediaItemMeta struct {
	Path        string    `json:"path"`
	Filename    string    `json:"filename"`
	Size        int64     `json:"size"`
	SizeFmt     string    `json:"size_fmt"`
	Width       int       `json:"width"`
	Height      int       `json:"height"`
	Resolution  string    `json:"resolution"`
	Format      string    `json:"format"`
	Duration    float64   `json:"duration,omitempty"`
	DurationFmt string    `json:"duration_fmt,omitempty"`
	FPS         float64   `json:"fps,omitempty"`
	ModTime     time.Time `json:"mod_time"`
	Type        string    `json:"type"` // "image" | "video"
}

// ComparisonResult encapsulates metadata comparison between two files.
type ComparisonResult struct {
	Left        MediaItemMeta `json:"left"`
	Right       MediaItemMeta `json:"right"`
	SizeDiff    int64         `json:"size_diff"`
	SizeDiffFmt string        `json:"size_diff_fmt"`
	IsSameDim   bool          `json:"is_same_dimensions"`
}

// CompareMetadata reads and compares metadata between two media files.
func CompareMetadata(ctx context.Context, leftPath, rightPath string) (*ComparisonResult, error) {
	leftMeta, err := extractMeta(ctx, leftPath)
	if err != nil {
		return nil, fmt.Errorf("reading left file %s: %w", leftPath, err)
	}

	rightMeta, err := extractMeta(ctx, rightPath)
	if err != nil {
		return nil, fmt.Errorf("reading right file %s: %w", rightPath, err)
	}

	diff := leftMeta.Size - rightMeta.Size
	diffFmt := shared.FormatFileSize(int64(math.Abs(float64(diff))))
	if diff > 0 {
		diffFmt = "+" + diffFmt
	} else if diff < 0 {
		diffFmt = "-" + diffFmt
	}

	return &ComparisonResult{
		Left:        *leftMeta,
		Right:       *rightMeta,
		SizeDiff:    diff,
		SizeDiffFmt: diffFmt,
		IsSameDim:   leftMeta.Width == rightMeta.Width && leftMeta.Height == rightMeta.Height && leftMeta.Width > 0,
	}, nil
}

func extractMeta(ctx context.Context, path string) (*MediaItemMeta, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	ext := shared.NormaliseExt(filepath.Ext(path))
	mediaType := "image"
	if shared.IsVideo(path) {
		mediaType = "video"
	}

	meta := &MediaItemMeta{
		Path:     path,
		Filename: filepath.Base(path),
		Size:     info.Size(),
		SizeFmt:  shared.FormatFileSize(info.Size()),
		Format:   ext,
		ModTime:  info.ModTime(),
		Type:     mediaType,
	}

	if mediaType == "video" {
		probeVideoMeta(ctx, path, meta)
	} else {
		img, err := shared.OpenImageFile(ctx, path)
		if err == nil && img != nil {
			bounds := img.Bounds()
			meta.Width = bounds.Dx()
			meta.Height = bounds.Dy()
			meta.Resolution = fmt.Sprintf("%dx%d", meta.Width, meta.Height)
		}
	}

	return meta, nil
}

func probeVideoMeta(ctx context.Context, path string, meta *MediaItemMeta) {
	candidates := shared.FFmpegCandidates()
	if len(candidates) == 0 {
		return
	}
	ffmpegBin := candidates[0]
	probeBin := strings.Replace(ffmpegBin, "ffmpeg", "ffprobe", 1)

	probeOut, err := exec.CommandContext(ctx, probeBin,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,duration,r_frame_rate,codec_name",
		"-of", "csv=p=0",
		shared.PathForBin(probeBin, path),
	).Output()
	if err != nil {
		return
	}

	parts := strings.Split(strings.TrimSpace(string(probeOut)), ",")
	if len(parts) >= 1 {
		meta.Width, _ = strconv.Atoi(parts[0])
	}
	if len(parts) >= 2 {
		meta.Height, _ = strconv.Atoi(parts[1])
	}
	if meta.Width > 0 && meta.Height > 0 {
		meta.Resolution = fmt.Sprintf("%dx%d", meta.Width, meta.Height)
	}
	if len(parts) >= 3 && parts[2] != "" && parts[2] != "N/A" {
		meta.Duration, _ = strconv.ParseFloat(parts[2], 64)
	}
	if len(parts) >= 4 {
		meta.FPS = parseFPS(parts[3])
	}
	if len(parts) >= 5 && parts[4] != "" {
		meta.Format = fmt.Sprintf("%s (%s)", meta.Format, parts[4])
	}
	if meta.Duration > 0 {
		meta.DurationFmt = formatDuration(meta.Duration)
	}
}

func parseFPS(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "0/0" {
		return 0
	}
	parts := strings.Split(s, "/")
	if len(parts) == 2 {
		num, err1 := strconv.ParseFloat(parts[0], 64)
		den, err2 := strconv.ParseFloat(parts[1], 64)
		if err1 == nil && err2 == nil && den > 0 {
			return math.Round((num/den)*100) / 100
		}
	}
	f, _ := strconv.ParseFloat(s, 64)
	return math.Round(f*100) / 100
}

func formatDuration(seconds float64) string {
	s := int(math.Round(seconds))
	if s < 60 {
		return fmt.Sprintf("%ds", s)
	}
	m := s / 60
	s = s % 60
	if m < 60 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := m / 60
	m = m % 60
	return fmt.Sprintf("%dh %dm %ds", h, m, s)
}

// GenerateDiffImage generates a visual difference image overlay between two images.
func GenerateDiffImage(ctx context.Context, leftPath, rightPath string) ([]byte, error) {
	if shared.IsVideo(leftPath) || shared.IsVideo(rightPath) {
		return nil, fmt.Errorf("difference heatmap is only supported for static images")
	}

	imgA, err := shared.OpenImageFile(ctx, leftPath)
	if err != nil {
		return nil, fmt.Errorf("opening left image: %w", err)
	}

	imgB, err := shared.OpenImageFile(ctx, rightPath)
	if err != nil {
		return nil, fmt.Errorf("opening right image: %w", err)
	}

	boundsA := imgA.Bounds()
	boundsB := imgB.Bounds()

	targetW := boundsA.Dx()
	targetH := boundsA.Dy()

	// Cap diff image dimensions to 1280px for fast preview generation
	const maxDiffDim = 1280
	if targetW > maxDiffDim || targetH > maxDiffDim {
		imgA = imaging.Fit(imgA, maxDiffDim, maxDiffDim, imaging.Lanczos)
		boundsA = imgA.Bounds()
		targetW = boundsA.Dx()
		targetH = boundsA.Dy()
	}

	// Resize imgB to match imgA bounds if dimensions differ
	if boundsB.Dx() != targetW || boundsB.Dy() != targetH {
		imgB = imaging.Resize(imgB, targetW, targetH, imaging.Lanczos)
	}

	diffImg := image.NewRGBA(image.Rect(0, 0, targetW, targetH))

	for y := 0; y < targetH; y++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		for x := 0; x < targetW; x++ {
			r1, g1, b1, _ := imgA.At(x, y).RGBA()
			r2, g2, b2, _ := imgB.At(x, y).RGBA()

			dr := uint8(math.Abs(float64(r1>>8) - float64(r2>>8)))
			dg := uint8(math.Abs(float64(g1>>8) - float64(g2>>8)))
			db := uint8(math.Abs(float64(b1>>8) - float64(b2>>8)))

			diffScore := int(dr) + int(dg) + int(db)
			if diffScore > 15 {
				// Highlight differences in high-contrast cyan/red heatmap
				intensity := uint8(math.Min(255, float64(diffScore)*2))
				diffImg.Set(x, y, color.RGBA{R: intensity, G: 50, B: 255 - intensity, A: 255})
			} else {
				// Dim identical background
				lum := uint8(float64(r1>>8)*0.299 + float64(g1>>8)*0.587 + float64(b1>>8)*0.114)
				diffImg.Set(x, y, color.RGBA{R: lum / 4, G: lum / 4, B: lum / 4, A: 255})
			}
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, diffImg, &jpeg.Options{Quality: 85}); err != nil {
		return nil, fmt.Errorf("encoding diff image: %w", err)
	}

	return buf.Bytes(), nil
}
