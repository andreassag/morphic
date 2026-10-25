package hasher

import (
	"bufio"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andreassag/morphic/internal/shared"
	"github.com/cespare/xxhash/v2"
)

// FileHashItem holds the computed hashes for a single file.
type FileHashItem struct {
	Path     string    `json:"path"`
	RelPath  string    `json:"rel_path"`
	Size     int64     `json:"size"`
	ModTime  time.Time `json:"mod_time"`
	MD5      string    `json:"md5,omitempty"`
	SHA256   string    `json:"sha256,omitempty"`
	XXHash64 string    `json:"xxhash64,omitempty"`
	Error    string    `json:"error,omitempty"`
}

// VerificationResult holds the status of verifying files against a manifest.
type VerificationResult struct {
	TotalChecked int            `json:"total_checked"`
	OKCount      int            `json:"ok_count"`
	CorruptCount int            `json:"corrupt_count"`
	MissingCount int            `json:"missing_count"`
	NewCount     int            `json:"new_count"`
	OKFiles      []string       `json:"ok_files"`
	Corrupted    []FileHashItem `json:"corrupted"`
	Missing      []string       `json:"missing"`
	NewFiles     []string       `json:"new_files"`
}

// HasherJob represents a background hashing task.
type HasherJob struct {
	mu        sync.RWMutex
	shared.Job
	TargetDir string         `json:"target_dir"`
	Items     []FileHashItem `json:"items"`
}

// SetProgress updates the job's progress in a thread-safe manner.
func (j *HasherJob) SetProgress(done, total int) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if total > 0 {
		j.Progress = float64(done) / float64(total)
	}
}

// SetDone marks the job as done and stores items in a thread-safe manner.
func (j *HasherJob) SetDone(items []FileHashItem) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Status = shared.JobStatusDone
	j.Progress = 1.0
	j.Items = items
	j.DoneAt = time.Now()
}

// SetFailed marks the job as failed with an error message in a thread-safe manner.
func (j *HasherJob) SetFailed(errMsg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Status = shared.JobStatusFailed
	j.Error = errMsg
	j.DoneAt = time.Now()
}

// Snapshot returns a thread-safe copy of the job's current state.
func (j *HasherJob) Snapshot() HasherJob {
	j.mu.RLock()
	defer j.mu.RUnlock()
	var itemsCopy []FileHashItem
	if j.Items != nil {
		itemsCopy = make([]FileHashItem, len(j.Items))
		copy(itemsCopy, j.Items)
	}
	return HasherJob{
		Job:       j.Job,
		TargetDir: j.TargetDir,
		Items:     itemsCopy,
	}
}

var jobs = shared.NewJobStore[HasherJob]()

// GetJob retrieves a hasher job by ID.
func GetJob(id string) (*HasherJob, bool) {
	return jobs.Get(id)
}

// SetJob stores a hasher job by ID.
func SetJob(id string, job *HasherJob) {
	jobs.Set(id, job)
}

// RegisterCancel registers a cancel func.
func RegisterCancel(id string, cancel context.CancelFunc) {
	jobs.RegisterCancel(id, cancel)
}

// CancelJob cancels a hasher job.
func CancelJob(id string) bool {
	return jobs.Cancel(id)
}

// StartCleanup periodically cleans up expired jobs.
func StartCleanup(ctx context.Context, ttl time.Duration) {
	jobs.StartCleanup(ctx, ttl, func(j *HasherJob) time.Time {
		return j.DoneAt
	})
}

// ComputeFileHashes calculates MD5, SHA256, and xxHash64 concurrently in a single read pass.
func ComputeFileHashes(ctx context.Context, path, baseDir string, computeMD5, computeSHA256, computeXXHash bool) (FileHashItem, error) {
	select {
	case <-ctx.Done():
		return FileHashItem{}, ctx.Err()
	default:
	}

	info, err := os.Stat(path)
	if err != nil {
		return FileHashItem{}, fmt.Errorf("stat %s: %w", path, err)
	}

	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		rel = filepath.Base(path)
	}

	item := FileHashItem{
		Path:    path,
		RelPath: filepath.ToSlash(rel),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}

	file, err := os.Open(path)
	if err != nil {
		return item, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	var writers []io.Writer
	hMD5 := md5.New()
	hSHA256 := sha256.New()
	hXXHash := xxhash.New()

	if computeMD5 {
		writers = append(writers, hMD5)
	}
	if computeSHA256 {
		writers = append(writers, hSHA256)
	}
	if computeXXHash {
		writers = append(writers, hXXHash)
	}

	if len(writers) == 0 {
		return item, nil
	}

	multi := io.MultiWriter(writers...)
	buf := make([]byte, 64*1024)
	for {
		select {
		case <-ctx.Done():
			return item, ctx.Err()
		default:
		}
		n, rErr := file.Read(buf)
		if n > 0 {
			if _, wErr := multi.Write(buf[:n]); wErr != nil {
				return item, fmt.Errorf("write hash buffer: %w", wErr)
			}
		}
		if rErr == io.EOF {
			break
		}
		if rErr != nil {
			return item, fmt.Errorf("read %s: %w", path, rErr)
		}
	}

	if computeMD5 {
		item.MD5 = hex.EncodeToString(hMD5.Sum(nil))
	}
	if computeSHA256 {
		item.SHA256 = hex.EncodeToString(hSHA256.Sum(nil))
	}
	if computeXXHash {
		item.XXHash64 = fmt.Sprintf("%016x", hXXHash.Sum64())
	}

	return item, nil
}

// HashDirectory scans and computes hashes for all files in a directory using worker pool.
func HashDirectory(ctx context.Context, folder string, computeMD5, computeSHA256, computeXXHash bool, onProgress func(done, total int)) ([]FileHashItem, error) {
	safeDir, err := shared.ValidateSafeDirPath(folder)
	if err != nil {
		return nil, fmt.Errorf("validate directory: %w", err)
	}

	var fileList []string
	err = filepath.WalkDir(safeDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if !d.IsDir() {
			fileList = append(fileList, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk directory %s: %w", safeDir, err)
	}

	total := len(fileList)
	if total == 0 {
		return nil, nil
	}

	numWorkers := runtime.NumCPU()
	if numWorkers > 8 {
		numWorkers = 8
	}
	if numWorkers > total {
		numWorkers = total
	}

	jobsCh := make(chan string, total)
	resultsCh := make(chan FileHashItem, total)

	for _, f := range fileList {
		jobsCh <- f
	}
	close(jobsCh)

	var wg sync.WaitGroup
	var completed int64
	var progressMu sync.Mutex

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobsCh {
				item, err := ComputeFileHashes(ctx, path, safeDir, computeMD5, computeSHA256, computeXXHash)
				if err != nil {
					item.Error = err.Error()
				}
				resultsCh <- item
				done := atomic.AddInt64(&completed, 1)
				if onProgress != nil {
					progressMu.Lock()
					onProgress(int(done), total)
					progressMu.Unlock()
				}
			}
		}()
	}

	wg.Wait()
	close(resultsCh)

	var items []FileHashItem
	for item := range resultsCh {
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].RelPath < items[j].RelPath
	})

	return items, nil
}

// ExportManifest formats items into .sha256, .md5, .xxh64, or .json.
func ExportManifest(items []FileHashItem, format string) ([]byte, error) {
	var sb strings.Builder
	switch strings.ToLower(format) {
	case "sha256":
		for _, it := range items {
			if it.SHA256 != "" {
				sb.WriteString(fmt.Sprintf("%s  %s\n", it.SHA256, it.RelPath))
			}
		}
		return []byte(sb.String()), nil
	case "md5":
		for _, it := range items {
			if it.MD5 != "" {
				sb.WriteString(fmt.Sprintf("%s  %s\n", it.MD5, it.RelPath))
			}
		}
		return []byte(sb.String()), nil
	case "xxh64", "xxhash":
		for _, it := range items {
			if it.XXHash64 != "" {
				sb.WriteString(fmt.Sprintf("%s  %s\n", it.XXHash64, it.RelPath))
			}
		}
		return []byte(sb.String()), nil
	case "json":
		fallthrough
	default:
		return json.MarshalIndent(items, "", "  ")
	}
}

// VerifyManifest checks current disk files against expected hashes from manifest.
func VerifyManifest(ctx context.Context, baseDir string, manifestContent []byte, format string) (*VerificationResult, error) {
	safeDir, err := shared.ValidateSafeDirPath(baseDir)
	if err != nil {
		return nil, fmt.Errorf("validate directory: %w", err)
	}

	expectedMap := make(map[string]FileHashItem)

	if format == "json" || (len(manifestContent) > 0 && manifestContent[0] == '[') {
		var items []FileHashItem
		if err := json.Unmarshal(manifestContent, &items); err != nil {
			return nil, fmt.Errorf("unmarshal json manifest: %w", err)
		}
		for _, it := range items {
			expectedMap[filepath.ToSlash(it.RelPath)] = it
		}
	} else {
		// Line-based parser: <hash> <relpath> or <hash>  <relpath>
		scanner := bufio.NewScanner(strings.NewReader(string(manifestContent)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				hashVal := parts[0]
				rel := strings.Join(parts[1:], " ")
				rel = strings.TrimPrefix(rel, "*") // handle binary mode prefix in md5sum/sha256sum
				it := FileHashItem{RelPath: filepath.ToSlash(rel)}
				switch strings.ToLower(format) {
				case "md5":
					it.MD5 = hashVal
				case "xxh64", "xxhash":
					it.XXHash64 = hashVal
				case "sha256":
					fallthrough
				default:
					it.SHA256 = hashVal
				}
				expectedMap[filepath.ToSlash(rel)] = it
			}
		}
	}

	result := &VerificationResult{}
	visited := make(map[string]bool)

	for rel, expected := range expectedMap {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		fullPath := filepath.Join(safeDir, filepath.FromSlash(rel))
		visited[fullPath] = true
		result.TotalChecked++

		info, err := os.Stat(fullPath)
		if os.IsNotExist(err) {
			result.MissingCount++
			result.Missing = append(result.Missing, rel)
			continue
		}

		// Recompute hash
		actual, err := ComputeFileHashes(ctx, fullPath, safeDir, expected.MD5 != "", expected.SHA256 != "", expected.XXHash64 != "")
		if err != nil {
			result.CorruptCount++
			actual.Error = err.Error()
			result.Corrupted = append(result.Corrupted, actual)
			continue
		}

		isCorrupt := false
		if expected.SHA256 != "" && !strings.EqualFold(actual.SHA256, expected.SHA256) {
			isCorrupt = true
		}
		if expected.MD5 != "" && !strings.EqualFold(actual.MD5, expected.MD5) {
			isCorrupt = true
		}
		if expected.XXHash64 != "" && !strings.EqualFold(actual.XXHash64, expected.XXHash64) {
			isCorrupt = true
		}
		if expected.Size > 0 && info.Size() != expected.Size {
			isCorrupt = true
		}

		if isCorrupt {
			result.CorruptCount++
			result.Corrupted = append(result.Corrupted, actual)
		} else {
			result.OKCount++
			result.OKFiles = append(result.OKFiles, rel)
		}
	}

	// Check for new files not in manifest
	_ = filepath.WalkDir(safeDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return nil
		}
		if !visited[path] {
			rel, _ := filepath.Rel(safeDir, path)
			result.NewCount++
			result.NewFiles = append(result.NewFiles, filepath.ToSlash(rel))
		}
		return nil
	})

	return result, nil
}
