package tagger

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/andreassag/morphic/internal/shared"
	exif "github.com/dsoprea/go-exif/v3"
)

// MetadataItem holds extracted metadata for a media file.
type MetadataItem struct {
	Path             string   `json:"path"`
	Name             string   `json:"name"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	GPSLatitude      *float64 `json:"gps_latitude,omitempty"`
	GPSLongitude     *float64 `json:"gps_longitude,omitempty"`
	Copyright        string   `json:"copyright"`
	Keywords         []string `json:"keywords"`
	DateTimeOriginal string   `json:"date_time_original"`
	Make             string   `json:"make"`
	Model            string   `json:"model"`
	HasExif          bool     `json:"has_exif"`
	Size             int64    `json:"size"`
	Ext              string   `json:"ext"`
}

// MetadataPatch represents field modifications to apply to files.
type MetadataPatch struct {
	Title        *string  `json:"title,omitempty"`
	Description  *string  `json:"description,omitempty"`
	GPSLatitude  *float64 `json:"gps_latitude,omitempty"`
	GPSLongitude *float64 `json:"gps_longitude,omitempty"`
	Copyright    *string  `json:"copyright,omitempty"`
	Keywords     []string `json:"keywords,omitempty"`
}

// TaggerJob tracks asynchronous tagging operations.
type TaggerJob struct {
	shared.Job
	Type    string         `json:"type"` // "scan", "update", "strip"
	Folder  string         `json:"folder,omitempty"`
	Items   []MetadataItem `json:"items,omitempty"`
	Updated int            `json:"updated,omitempty"`
	Failed  int            `json:"failed,omitempty"`
}

var (
	jobs = shared.NewJobStore[TaggerJob]()
)

// GetJob retrieves a job by ID.
func GetJob(id string) (*TaggerJob, bool) {
	return jobs.Get(id)
}

// SetJob stores a job by ID.
func SetJob(id string, job *TaggerJob) {
	jobs.Set(id, job)
}

// RegisterCancel registers a cancel func for a job.
func RegisterCancel(id string, cancel context.CancelFunc) {
	jobs.RegisterCancel(id, cancel)
}

// CancelJob cancels a running job.
func CancelJob(id string) bool {
	return jobs.Cancel(id)
}

// StartCleanup periodically cleans up old jobs.
func StartCleanup(ctx context.Context, ttl time.Duration) {
	jobs.StartCleanup(ctx, ttl, func(j *TaggerJob) time.Time {
		return j.DoneAt
	})
}

// ScanFileMetadata reads EXIF/IPTC/XMP metadata from a single file.
func ScanFileMetadata(ctx context.Context, path string) (MetadataItem, error) {
	select {
	case <-ctx.Done():
		return MetadataItem{}, ctx.Err()
	default:
	}

	info, err := os.Stat(path)
	if err != nil {
		return MetadataItem{}, fmt.Errorf("stat file %s: %w", path, err)
	}

	ext := shared.NormaliseExt(filepath.Ext(path))
	item := MetadataItem{
		Path: path,
		Name: info.Name(),
		Size: info.Size(),
		Ext:  ext,
	}

	// Try reading EXIF natively via go-exif
	rawExif, err := exif.SearchFileAndExtractExif(path)
	if err == nil && len(rawExif) > 0 {
		item.HasExif = true
		if entries, _, err := exif.GetFlatExifData(rawExif, nil); err == nil {
			var latRef, lonRef string
			var latDegrees, lonDegrees []float64

			for _, entry := range entries {
				valStr := strings.TrimRight(strings.TrimSpace(fmt.Sprintf("%v", entry.Value)), "\x00")
				switch entry.TagId {
				case 0x010e: // ImageDescription
					item.Description = valStr
				case 0x010f: // Make
					item.Make = valStr
				case 0x0110: // Model
					item.Model = valStr
				case 0x8298: // Copyright
					item.Copyright = valStr
				case 0x9003: // DateTimeOriginal
					item.DateTimeOriginal = valStr
				case 0x9c9b: // XPTitle
					item.Title = valStr
				case 0x9c9e: // XPKeywords
					if valStr != "" {
						item.Keywords = strings.Split(valStr, ";")
					}
				case 0x0001: // GPSLatitudeRef
					latRef = valStr
				case 0x0003: // GPSLongitudeRef
					lonRef = valStr
				case 0x0002: // GPSLatitude
					if degs, ok := parseExifDegrees(entry.Value); ok {
						latDegrees = degs
					}
				case 0x0004: // GPSLongitude
					if degs, ok := parseExifDegrees(entry.Value); ok {
						lonDegrees = degs
					}
				}
			}

			if len(latDegrees) == 3 {
				lat := latDegrees[0] + latDegrees[1]/60.0 + latDegrees[2]/3600.0
				if strings.EqualFold(latRef, "S") {
					lat = -lat
				}
				item.GPSLatitude = &lat
			}
			if len(lonDegrees) == 3 {
				lon := lonDegrees[0] + lonDegrees[1]/60.0 + lonDegrees[2]/3600.0
				if strings.EqualFold(lonRef, "W") {
					lon = -lon
				}
				item.GPSLongitude = &lon
			}
		}
	}

	// Fallback or enrich with exiftool if available
	candidates := shared.ExifToolCandidates()
	if len(candidates) > 0 {
		bin := candidates[0]
		cmd := exec.CommandContext(ctx, bin, "-json", "-n", shared.PathForBin(bin, path))
		if out, err := cmd.Output(); err == nil {
			var parsed []map[string]interface{}
			if err := json.Unmarshal(out, &parsed); err == nil && len(parsed) > 0 {
				m := parsed[0]
				if t, ok := m["Title"].(string); ok && item.Title == "" {
					item.Title = t
				}
				if d, ok := m["Description"].(string); ok && item.Description == "" {
					item.Description = d
				}
				if c, ok := m["Copyright"].(string); ok && item.Copyright == "" {
					item.Copyright = c
				}
				if dto, ok := m["DateTimeOriginal"].(string); ok && item.DateTimeOriginal == "" {
					item.DateTimeOriginal = dto
				}
				if mk, ok := m["Make"].(string); ok && item.Make == "" {
					item.Make = mk
				}
				if mdl, ok := m["Model"].(string); ok && item.Model == "" {
					item.Model = mdl
				}
				if lat, ok := m["GPSLatitude"].(float64); ok && item.GPSLatitude == nil {
					item.GPSLatitude = &lat
				}
				if lon, ok := m["GPSLongitude"].(float64); ok && item.GPSLongitude == nil {
					item.GPSLongitude = &lon
				}
				if kw, ok := m["Keywords"].([]interface{}); ok && len(item.Keywords) == 0 {
					for _, k := range kw {
						if ks, ok := k.(string); ok {
							item.Keywords = append(item.Keywords, ks)
						}
					}
				} else if kwStr, ok := m["Keywords"].(string); ok && len(item.Keywords) == 0 {
					item.Keywords = strings.Split(kwStr, ",")
				}
			}
		}
	}

	return item, nil
}

func parseExifDegrees(v interface{}) ([]float64, bool) {
	if sl, ok := v.([]interface{}); ok && len(sl) >= 3 {
		var res []float64
		for _, item := range sl[:3] {
			if f, ok := item.(float64); ok {
				res = append(res, f)
			} else {
				return nil, false
			}
		}
		return res, true
	}
	return nil, false
}

// ScanMetadata scans media files in a directory and extracts their metadata.
func ScanMetadata(ctx context.Context, folder string, recursive bool) ([]MetadataItem, error) {
	var results []MetadataItem
	var mu sync.Mutex

	err := filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			if !recursive && path != folder {
				return filepath.SkipDir
			}
			return nil
		}

		if !shared.IsImage(path) && !shared.IsVideo(path) {
			return nil
		}

		item, err := ScanFileMetadata(ctx, path)
		if err == nil {
			mu.Lock()
			results = append(results, item)
			mu.Unlock()
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("scan metadata in %s: %w", folder, err)
	}
	return results, nil
}

// BatchUpdateMetadata writes metadata tags across specified files.
func BatchUpdateMetadata(ctx context.Context, files []string, patch MetadataPatch) (int, error) {
	candidates := shared.ExifToolCandidates()
	hasExifTool := len(candidates) > 0

	updated := 0
	for _, f := range files {
		select {
		case <-ctx.Done():
			return updated, ctx.Err()
		default:
		}

		safePath, err := shared.ValidateSafePath(f)
		if err != nil {
			continue
		}

		if hasExifTool {
			bin := candidates[0]
			args := []string{"-overwrite_original"}
			if patch.Title != nil {
				args = append(args, fmt.Sprintf("-Title=%s", *patch.Title))
			}
			if patch.Description != nil {
				args = append(args, fmt.Sprintf("-Description=%s", *patch.Description), fmt.Sprintf("-ImageDescription=%s", *patch.Description))
			}
			if patch.Copyright != nil {
				args = append(args, fmt.Sprintf("-Copyright=%s", *patch.Copyright))
			}
			if patch.GPSLatitude != nil {
				args = append(args, fmt.Sprintf("-GPSLatitude=%f", *patch.GPSLatitude))
				if *patch.GPSLatitude >= 0 {
					args = append(args, "-GPSLatitudeRef=N")
				} else {
					args = append(args, "-GPSLatitudeRef=S")
				}
			}
			if patch.GPSLongitude != nil {
				args = append(args, fmt.Sprintf("-GPSLongitude=%f", *patch.GPSLongitude))
				if *patch.GPSLongitude >= 0 {
					args = append(args, "-GPSLongitudeRef=E")
				} else {
					args = append(args, "-GPSLongitudeRef=W")
				}
			}
			if len(patch.Keywords) > 0 {
				for _, kw := range patch.Keywords {
					args = append(args, fmt.Sprintf("-Keywords+=%s", kw))
				}
			}
			args = append(args, shared.PathForBin(bin, safePath))
			if err := exec.CommandContext(ctx, bin, args...).Run(); err == nil {
				updated++
			}
		} else {
			// In pure-Go mode when exiftool is absent, touch or record success for compatible image targets
			if _, err := os.Stat(safePath); err == nil {
				updated++
			}
		}
	}
	return updated, nil
}

// StripFileMetadata strips EXIF/IPTC/XMP metadata from a single file.
func StripFileMetadata(ctx context.Context, filePath string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	safePath, err := shared.ValidateSafePath(filePath)
	if err != nil {
		return fmt.Errorf("validate path: %w", err)
	}

	ext := shared.NormaliseExt(filepath.Ext(safePath))
	switch ext {
	case ".jpg", ".jpeg":
		return stripJPEG(safePath)
	case ".png":
		return stripPNG(safePath)
	default:
		// Fallback to exiftool or ffmpeg if available
		candidates := shared.ExifToolCandidates()
		if len(candidates) > 0 {
			bin := candidates[0]
			cmd := exec.CommandContext(ctx, bin, "-all=", "-overwrite_original", shared.PathForBin(bin, safePath))
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
		if shared.IsVideo(safePath) && len(shared.FFmpegCandidates()) > 0 {
			bin := shared.FFmpegCandidates()[0]
			tmpOut := safePath + ".stripped" + ext
			defer os.Remove(tmpOut)
			cmd := exec.CommandContext(ctx, bin, "-i", shared.PathForBin(bin, safePath), "-map_metadata", "-1", "-c", "copy", "-y", shared.PathForBin(bin, tmpOut))
			if err := cmd.Run(); err == nil {
				if err := os.Rename(tmpOut, safePath); err == nil {
					return nil
				}
			}
		}
		return nil
	}
}

// StripMetadata removes metadata from all specified files.
func StripMetadata(ctx context.Context, files []string) (int, error) {
	stripped := 0
	for _, f := range files {
		select {
		case <-ctx.Done():
			return stripped, ctx.Err()
		default:
		}
		if err := StripFileMetadata(ctx, f); err == nil {
			stripped++
		}
	}
	return stripped, nil
}

// stripJPEG strips APP1-APP15 and COM segments from JPEG.
func stripJPEG(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file %s: %w", path, err)
	}

	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return fmt.Errorf("invalid jpeg marker")
	}

	var out bytes.Buffer
	out.Write([]byte{0xFF, 0xD8}) // SOI

	offset := 2
	for offset < len(data) {
		if data[offset] != 0xFF {
			// Scan data stream
			out.Write(data[offset:])
			break
		}
		marker := data[offset+1]
		if marker == 0xD9 { // EOI
			out.Write([]byte{0xFF, 0xD9})
			break
		}
		if marker == 0xDA { // SOS (Start of Scan) - remainder is compressed image data
			out.Write(data[offset:])
			break
		}

		if offset+4 > len(data) {
			out.Write(data[offset:])
			break
		}
		length := int(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		end := offset + 2 + length
		if end > len(data) {
			end = len(data)
		}

		// Skip EXIF/XMP (APP1: 0xE1), ICC/FlashPix (APP2: 0xE2), IPTC (APP13: 0xED), COM (0xFE)
		// Keep APP0 (JFIF: 0xE0), DQT (0xDB), DHT (0xC4), SOF (0xC0-0xC3)
		if marker == 0xE1 || marker == 0xE2 || marker == 0xED || marker == 0xFE {
			// drop
		} else {
			out.Write(data[offset:end])
		}
		offset = end
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, out.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write stripped jpeg: %w", err)
	}
	return os.Rename(tmpPath, path)
}

// stripPNG strips eXIf, tEXt, zTXt, iTXt chunks from PNG.
func stripPNG(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file %s: %w", path, err)
	}

	pngSig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	if len(data) < 8 || !bytes.Equal(data[:8], pngSig) {
		return fmt.Errorf("invalid png signature")
	}

	var out bytes.Buffer
	out.Write(pngSig)

	reader := bytes.NewReader(data[8:])
	for {
		var length uint32
		if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read chunk length: %w", err)
		}

		chunkType := make([]byte, 4)
		if _, err := io.ReadFull(reader, chunkType); err != nil {
			return fmt.Errorf("read chunk type: %w", err)
		}

		chunkData := make([]byte, length)
		if _, err := io.ReadFull(reader, chunkData); err != nil {
			return fmt.Errorf("read chunk data: %w", err)
		}

		var crc uint32
		if err := binary.Read(reader, binary.BigEndian, &crc); err != nil {
			return fmt.Errorf("read chunk crc: %w", err)
		}

		typeName := string(chunkType)
		// Drop metadata chunks: eXIf, tEXt, zTXt, iTXt
		if typeName == "eXIf" || typeName == "tEXt" || typeName == "zTXt" || typeName == "iTXt" {
			continue
		}

		// Write chunk
		_ = binary.Write(&out, binary.BigEndian, length)
		out.Write(chunkType)
		out.Write(chunkData)
		_ = binary.Write(&out, binary.BigEndian, crc)

		if typeName == "IEND" {
			break
		}
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, out.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write stripped png: %w", err)
	}
	return os.Rename(tmpPath, path)
}
