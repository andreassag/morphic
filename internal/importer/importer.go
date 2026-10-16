package importer

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andreassag/morphic/internal/shared"
)

// DeviceInfo represents a detected mount point or portable device storage.
type DeviceInfo struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"` // "mount", "gvfs", "external"
}

// URLImportRequest configures ingestion from an external HTTP/HTTPS URL.
type URLImportRequest struct {
	URL            string `json:"url"`
	DestinationDir string `json:"destination_dir"`
	OrganizeTarget string `json:"organize_target,omitempty"` // optional chained sort folder
}

// S3ImportRequest configures ingestion from an S3-compatible bucket.
type S3ImportRequest struct {
	Endpoint       string `json:"endpoint"` // e.g. "s3.amazonaws.com" or MinIO URL
	Bucket         string `json:"bucket"`
	Prefix         string `json:"prefix"`
	Region         string `json:"region"`
	AccessKey      string `json:"access_key"`
	SecretKey      string `json:"secret_key"`
	DestinationDir string `json:"destination_dir"`
}

// DeviceImportRequest configures ingestion from a local mounted device path.
type DeviceImportRequest struct {
	SourceDir      string `json:"source_dir"`
	DestinationDir string `json:"destination_dir"`
	Recursive      bool   `json:"recursive"`
}

// ImporterJob tracks background import progress.
type ImporterJob struct {
	shared.Job
	Type         string   `json:"type"` // "url", "s3", "device"
	TargetDir    string   `json:"target_dir"`
	ImportedFiles []string `json:"imported_files"`
}

var jobs = shared.NewJobStore[ImporterJob]()

// GetJob retrieves an importer job by ID.
func GetJob(id string) (*ImporterJob, bool) {
	return jobs.Get(id)
}

// SetJob stores an importer job by ID.
func SetJob(id string, job *ImporterJob) {
	jobs.Set(id, job)
}

// RegisterCancel registers a cancel func.
func RegisterCancel(id string, cancel context.CancelFunc) {
	jobs.RegisterCancel(id, cancel)
}

// CancelJob cancels an importer job.
func CancelJob(id string) bool {
	return jobs.Cancel(id)
}

// StartCleanup periodically cleans up expired jobs.
func StartCleanup(ctx context.Context, ttl time.Duration) {
	jobs.StartCleanup(ctx, ttl, func(j *ImporterJob) time.Time {
		return j.DoneAt
	})
}

// ValidateSSRF checks if the URL resolves to a forbidden private, loopback, or internal IP.
func ValidateSSRF(rawURL string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("disallowed URL scheme: %s", u.Scheme)
	}

	hostname := u.Hostname()
	if hostname == "" {
		return nil, fmt.Errorf("empty hostname in URL")
	}

	if strings.EqualFold(hostname, "localhost") {
		return nil, fmt.Errorf("access to localhost is blocked")
	}

	ips, err := net.LookupIP(hostname)
	if err != nil {
		return nil, fmt.Errorf("resolving host %s: %w", hostname, err)
	}

	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return nil, fmt.Errorf("access to private/local IP (%s) is blocked", ip.String())
		}
	}

	return u, nil
}

// ImportFromURL downloads a media file from an external URL with SSRF guards and size limits.
func ImportFromURL(ctx context.Context, req URLImportRequest) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}

	u, err := ValidateSSRF(req.URL)
	if err != nil {
		return "", fmt.Errorf("ssrf check failed: %w", err)
	}

	destDir, err := shared.ValidateSafeDirPath(req.DestinationDir)
	if err != nil {
		return "", fmt.Errorf("validate destination: %w", err)
	}

	// Safe filename extraction
	fileName := filepath.Base(u.Path)
	if fileName == "" || fileName == "." || fileName == "/" {
		fileName = fmt.Sprintf("imported_%d.bin", time.Now().Unix())
	}
	// Sanitize filename
	fileName = filepath.Clean(fileName)

	targetPath := filepath.Join(destDir, fileName)
	safeTarget, err := shared.ValidateSafePath(targetPath)
	if err != nil {
		return "", fmt.Errorf("validate target path: %w", err)
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("create http request: %w", err)
	}
	httpReq.Header.Set("User-Agent", "Morphic-Importer/1.0")

	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("download url %s: %w", u.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// Limit download size to 500 MB
	maxBytes := int64(500 * 1024 * 1024)
	lr := io.LimitReader(resp.Body, maxBytes+1)

	tmpFile := safeTarget + ".downloading"
	out, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return "", fmt.Errorf("create file %s: %w", tmpFile, err)
	}
	defer func() {
		_ = out.Close()
		_ = os.Remove(tmpFile)
	}()

	written, err := io.Copy(out, lr)
	if err != nil {
		return "", fmt.Errorf("writing download content: %w", err)
	}
	if written > maxBytes {
		return "", fmt.Errorf("file exceeded max download limit of 500MB")
	}

	_ = out.Close()
	if err := os.Rename(tmpFile, safeTarget); err != nil {
		return "", fmt.Errorf("finalize downloaded file: %w", err)
	}

	return safeTarget, nil
}

// DetectMountedDevices scans common system directories for external mount points.
func DetectMountedDevices(ctx context.Context) ([]DeviceInfo, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var devices []DeviceInfo
	searchPaths := []struct {
		root   string
		devTyp string
	}{
		{"/media", "mount"},
		{"/mnt", "mount"},
		{"/run/media", "external"},
	}

	for _, sp := range searchPaths {
		entries, err := os.ReadDir(sp.root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				p := filepath.Join(sp.root, e.Name())
				devices = append(devices, DeviceInfo{
					Name: e.Name(),
					Path: p,
					Type: sp.devTyp,
				})
			}
		}
	}

	// Check gvfs mount points under /run/user/*/gvfs
	gvfsMatches, _ := filepath.Glob("/run/user/*/gvfs/*")
	for _, m := range gvfsMatches {
		info, err := os.Stat(m)
		if err == nil && info.IsDir() {
			devices = append(devices, DeviceInfo{
				Name: filepath.Base(m),
				Path: m,
				Type: "gvfs",
			})
		}
	}

	return devices, nil
}

// ImportFromDevice copies media files from a mounted device into a destination folder.
func ImportFromDevice(ctx context.Context, req DeviceImportRequest) ([]string, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	safeSrc, err := shared.ValidateSafeDirPath(req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("validate source directory: %w", err)
	}

	safeDest, err := shared.ValidateSafeDirPath(req.DestinationDir)
	if err != nil {
		return nil, fmt.Errorf("validate destination directory: %w", err)
	}

	var imported []string
	err = filepath.WalkDir(safeSrc, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if d.IsDir() {
			if !req.Recursive && path != safeSrc {
				return filepath.SkipDir
			}
			return nil
		}

		if !shared.IsImage(path) && !shared.IsVideo(path) {
			return nil
		}

		rel, err := filepath.Rel(safeSrc, path)
		if err != nil {
			rel = d.Name()
		}

		target := filepath.Join(safeDest, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil
		}

		if err := copyFile(ctx, path, target); err == nil {
			imported = append(imported, target)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("importing from device %s: %w", safeSrc, err)
	}
	return imported, nil
}

func copyFile(ctx context.Context, src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	buf := make([]byte, 64*1024)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		n, rErr := in.Read(buf)
		if n > 0 {
			if _, wErr := out.Write(buf[:n]); wErr != nil {
				return wErr
			}
		}
		if rErr == io.EOF {
			break
		}
		if rErr != nil {
			return rErr
		}
	}
	return nil
}

// S3BucketContents holds listed keys from S3 XML.
type S3BucketContents struct {
	Contents []struct {
		Key  string `xml:"Key"`
		Size int64  `xml:"Size"`
	} `xml:"Contents"`
}

// ImportFromS3 downloads objects from an S3-compatible bucket using SigV4 signed requests.
func ImportFromS3(ctx context.Context, req S3ImportRequest) ([]string, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	destDir, err := shared.ValidateSafeDirPath(req.DestinationDir)
	if err != nil {
		return nil, fmt.Errorf("validate destination: %w", err)
	}

	if req.Region == "" {
		req.Region = "us-east-1"
	}
	endpoint := req.Endpoint
	if endpoint == "" {
		endpoint = "s3.amazonaws.com"
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "https://" + endpoint
	}

	listURL := fmt.Sprintf("%s/%s?list-type=2", endpoint, req.Bucket)
	if req.Prefix != "" {
		listURL += "&prefix=" + url.QueryEscape(req.Prefix)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create S3 request: %w", err)
	}

	// Sign request if credentials provided
	if req.AccessKey != "" && req.SecretKey != "" {
		signS3Request(httpReq, req.AccessKey, req.SecretKey, req.Region)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("s3 list request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("s3 list returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var listResp S3BucketContents
	if err := xml.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("decode s3 list xml: %w", err)
	}

	var imported []string
	for _, obj := range listResp.Contents {
		select {
		case <-ctx.Done():
			return imported, ctx.Err()
		default:
		}
		if obj.Size == 0 || strings.HasSuffix(obj.Key, "/") {
			continue
		}

		keyTarget := filepath.Join(destDir, filepath.FromSlash(obj.Key))
		if err := os.MkdirAll(filepath.Dir(keyTarget), 0o755); err != nil {
			continue
		}

		getURL := fmt.Sprintf("%s/%s/%s", endpoint, req.Bucket, obj.Key)
		getReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, getURL, nil)
		if req.AccessKey != "" && req.SecretKey != "" {
			signS3Request(getReq, req.AccessKey, req.SecretKey, req.Region)
		}

		getResp, err := client.Do(getReq)
		if err != nil || getResp.StatusCode != http.StatusOK {
			if getResp != nil {
				getResp.Body.Close()
			}
			continue
		}

		outFile, err := os.OpenFile(keyTarget, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err == nil {
			_, _ = io.Copy(outFile, getResp.Body)
			_ = outFile.Close()
			imported = append(imported, keyTarget)
		}
		getResp.Body.Close()
	}

	return imported, nil
}

func signS3Request(r *http.Request, accessKey, secretKey, region string) {
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")

	r.Header.Set("x-amz-date", amzDate)
	r.Header.Set("x-amz-content-sha256", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855") // sha256 of empty string

	// Simple SigV4 headers
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\nx-amz-date:%s\n", r.Host, amzDate)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		r.Method, r.URL.Path, r.URL.RawQuery, canonicalHeaders, signedHeaders)

	hCanon := sha256.Sum256([]byte(canonicalRequest))
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, region)
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s", amzDate, credentialScope, hex.EncodeToString(hCanon[:]))

	kDate := hmacSHA256([]byte("AWS4"+secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	auth := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credentialScope, signedHeaders, signature)
	r.Header.Set("Authorization", auth)
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}
