package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/andreassag/morphic/web"
	"github.com/gin-gonic/gin"
)

func setupTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r, err := web.SetupRouter(context.Background(), nil)
	if err != nil {
		t.Fatalf("failed to setup router: %v", err)
	}
	return r
}

func TestHealthAndReadyEndpoints(t *testing.T) {
	r := setupTestRouter(t)

	// Healthz
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/healthz", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("healthz returned %d, want %d", w.Code, http.StatusOK)
	}

	// Ready
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/ready", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("ready returned %d, want %d", w.Code, http.StatusOK)
	}
}

func TestSystemInfoEndpoint(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/system_info", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("system_info returned %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if resp["version"] == nil || resp["platform"] == nil {
		t.Error("expected version and platform in system_info response")
	}
	if resp["exiftool"] == nil || resp["sevenzip"] == nil {
		t.Error("expected exiftool and sevenzip in system_info response")
	}
}

func TestConverterFormatsEndpoint(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/converter/formats", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("formats returned %d, want %d", w.Code, http.StatusOK)
	}
}

func TestConverterDeleteEndpoint(t *testing.T) {
	r := setupTestRouter(t)

	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "delete_me.txt")
	os.WriteFile(f1, []byte("hello"), 0o644)

	body, _ := json.Marshal(map[string]interface{}{
		"files": []string{f1},
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/converter/delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("delete returned %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHistoryEndpoint(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/history", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("history returned %d, want %d", w.Code, http.StatusOK)
	}
}

func TestWatchEndpoint(t *testing.T) {
	r := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/watch", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("watch returned %d, want %d", w.Code, http.StatusOK)
	}
}

func TestBrowseEndpoint_TildeAndAbs(t *testing.T) {
	r := setupTestRouter(t)

	// Test GET /api/browse with empty path (defaults to home)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/browse", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("browse with empty path returned %d, want %d", w.Code, http.StatusOK)
	}

	// Test GET /api/browse?path=~ (expands to home)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/browse?path=~", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("browse with tilde path returned %d, want %d", w.Code, http.StatusOK)
	}

	// Test GET /api/browse?path=/ (root directory should be browsable)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/browse?path=/", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("browse with root path '/' returned %d, want %d", w.Code, http.StatusOK)
	}
}

func TestBrowseResolve_FindsPickedFolderByNameAndChildren(t *testing.T) {
	r := setupTestRouter(t)

	tmp := t.TempDir()
	// Two folders with the same name; only one contains the picked children.
	target := filepath.Join(tmp, "a", "morphic_pick_xyz")
	decoy := filepath.Join(tmp, "b", "morphic_pick_xyz")
	for _, d := range []string{target, decoy} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(target, "one.jpg"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(target, "two.jpg"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(decoy, "other.txt"), []byte("x"), 0o644)

	body, _ := json.Marshal(map[string]interface{}{
		"name":     "morphic_pick_xyz",
		"children": []string{"One.JPG", "two.jpg"},
		"hint":     tmp,
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/browse/resolve", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("resolve returned %d, want 200 (body: %s)", w.Code, w.Body.String())
	}

	var resp struct {
		Resolved bool   `json:"resolved"`
		Path     string `json:"path"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid resolve response: %v", err)
	}
	if !resp.Resolved || resp.Path != target {
		t.Errorf("expected resolved path %q, got resolved=%v path=%q", target, resp.Resolved, resp.Path)
	}

	// Unknown folder: not resolved, no candidates, still 200
	body, _ = json.Marshal(map[string]interface{}{"name": "definitely_not_a_real_folder_123", "hint": tmp})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/browse/resolve", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("resolve (unknown) returned %d, want 200", w.Code)
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Resolved {
		t.Errorf("expected unresolved result for unknown folder, got path=%q", resp.Path)
	}
}

func TestBrowseEndpoint_TranslatesWindowsPaths(t *testing.T) {
	r := setupTestRouter(t)

	// "C:\" style paths are translated to /mnt/c on non-Windows runners; the request must not 500.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", `/api/browse?path=D%3A%5CPhotos`, nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Errorf("windows-style path returned %d, want 200 or 400", w.Code)
	}
}

func TestConverterScanEndpoint(t *testing.T) {
	r := setupTestRouter(t)

	// 1. Bare non-absolute path should return 400 INVALID_FOLDER
	body, _ := json.Marshal(map[string]interface{}{
		"folder": "relative_folder",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/converter/scan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("relative folder returned %d, want %d", w.Code, http.StatusBadRequest)
	}

	// 2. Valid absolute directory should return 200
	tmp := t.TempDir()
	body, _ = json.Marshal(map[string]interface{}{
		"folder": tmp,
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/converter/scan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("valid folder scan returned %d, want %d", w.Code, http.StatusOK)
	}
}

func TestDupfinderScanEndpoint(t *testing.T) {
	r := setupTestRouter(t)

	// 1. Bare non-absolute path should return 400 INVALID_FOLDER
	body, _ := json.Marshal(map[string]interface{}{
		"folder": "my_photos",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/dupfinder/scan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("relative folder returned %d, want %d", w.Code, http.StatusBadRequest)
	}

	// 2. Valid absolute folder should start scan and return 202
	tmp := t.TempDir()
	body, _ = json.Marshal(map[string]interface{}{
		"folder": tmp,
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/dupfinder/scan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("valid dupfinder scan returned %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestDupfinderDeleteAndTrashHistory(t *testing.T) {
	trashDir := t.TempDir()
	t.Setenv("MORPHIC_TRASH_DIR", trashDir)

	r := setupTestRouter(t)

	tmp := t.TempDir()
	dupFile := filepath.Join(tmp, "duplicate_photo.jpg")
	content := []byte("image content here")
	if err := os.WriteFile(dupFile, content, 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// 1. Delete the file via /api/dupfinder/delete
	body, _ := json.Marshal(map[string]interface{}{
		"files": []string{dupFile},
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/dupfinder/delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("dupfinder delete returned %d, want %d", w.Code, http.StatusOK)
	}

	var delResp web.DeleteFilesResponse
	if err := json.Unmarshal(w.Body.Bytes(), &delResp); err != nil {
		t.Fatalf("failed to decode delete response: %v", err)
	}
	if len(delResp.Results) != 1 || delResp.Results[0].Status != "deleted" {
		t.Fatalf("expected 1 deleted result, got %+v", delResp.Results)
	}
	auditID := delResp.Results[0].AuditID

	// 2. Query /api/history to verify it appears in Safe Trash
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/history?operation=delete", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("history returned %d, want %d", w.Code, http.StatusOK)
	}

	var histResp struct {
		Entries []map[string]interface{} `json:"entries"`
		Total   int64                    `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &histResp); err != nil {
		t.Fatalf("failed to decode history response: %v", err)
	}
	if histResp.Total != 1 || len(histResp.Entries) != 1 {
		t.Fatalf("expected 1 history entry, got total=%d, entries=%d", histResp.Total, len(histResp.Entries))
	}

	// 3. Restore the file via /api/history/:id/undo
	undoURL := fmt.Sprintf("/api/history/%d/undo", auditID)
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", undoURL, nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("undo returned %d, want %d (body: %s)", w.Code, http.StatusOK, w.Body.String())
	}

	// 4. Verify file restored to disk
	restored, err := os.ReadFile(dupFile)
	if err != nil {
		t.Fatalf("restored file missing: %v", err)
	}
	if string(restored) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", restored, content)
	}
}

func TestTrashAndHistorySeparation(t *testing.T) {
	trashDir := t.TempDir()
	t.Setenv("MORPHIC_TRASH_DIR", trashDir)

	r := setupTestRouter(t)

	tmp := t.TempDir()
	dupFile := filepath.Join(tmp, "trash_test_file.png")
	if err := os.WriteFile(dupFile, []byte("fake png content"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// 1. Delete file
	delBody, _ := json.Marshal(map[string]interface{}{
		"files": []string{dupFile},
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/dupfinder/delete", bytes.NewReader(delBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete failed: %d", w.Code)
	}

	// 2. Safe Trash endpoint GET /api/trash should return individual deleted file
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/trash", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("trash returned %d, want %d", w.Code, http.StatusOK)
	}
	var trashResp struct {
		Entries []map[string]interface{} `json:"entries"`
		Total   int64                    `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &trashResp); err != nil {
		t.Fatalf("failed to decode /api/trash response: %v", err)
	}
	if trashResp.Total < 1 || len(trashResp.Entries) < 1 {
		t.Fatalf("expected at least 1 entry in /api/trash, got %d", trashResp.Total)
	}
	entry := trashResp.Entries[0]
	if entry["original_path"] != dupFile {
		t.Errorf("expected original_path=%q, got %v", dupFile, entry["original_path"])
	}

	// 3. Audit History endpoint GET /api/history should return bulk operation
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/history", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("history returned %d, want %d", w.Code, http.StatusOK)
	}
	var histResp struct {
		Entries []map[string]interface{} `json:"entries"`
		Total   int64                    `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &histResp); err != nil {
		t.Fatalf("failed to decode /api/history response: %v", err)
	}
	if histResp.Total < 1 || len(histResp.Entries) < 1 {
		t.Fatalf("expected at least 1 entry in /api/history, got %d", histResp.Total)
	}
	histOp := histResp.Entries[0]
	if histOp["operation"] != "delete" {
		t.Errorf("expected bulk delete operation, got %v", histOp["operation"])
	}
	if histOp["summary"] == nil || histOp["summary"] == "" {
		t.Errorf("expected summary in bulk audit operation, got %v", histOp["summary"])
	}
}

func TestMediaCompareAndDiff(t *testing.T) {
	r := setupTestRouter(t)

	tmp := t.TempDir()
	v1 := filepath.Join(tmp, "video1.mp4")
	v2 := filepath.Join(tmp, "video2.mp4")
	_ = os.WriteFile(v1, []byte("fake mp4 data 1"), 0o644)
	_ = os.WriteFile(v2, []byte("fake mp4 data 2"), 0o644)

	// 1. Missing paths should fail
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/media/compare?left=", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing paths, got %d", w.Code)
	}

	// 2. Diff on video files should reject with error
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/media/diff?left=%s&right=%s", v1, v2), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 DIFF_FAILED for video diff, got %d", w.Code)
	}
}

func TestTrashDeleteAndEmptyEndpoints(t *testing.T) {
	trashDir := t.TempDir()
	t.Setenv("MORPHIC_TRASH_DIR", trashDir)

	r := setupTestRouter(t)

	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "trash1.txt")
	f2 := filepath.Join(tmp, "trash2.txt")
	_ = os.WriteFile(f1, []byte("data 1"), 0o644)
	_ = os.WriteFile(f2, []byte("data 2"), 0o644)

	// Move both to trash via delete endpoint
	body, _ := json.Marshal(map[string]interface{}{"files": []string{f1, f2}})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/converter/delete", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("delete failed: %d", w.Code)
	}

	// List trash
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/trash", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("list trash failed: %d", w.Code)
	}
	var listResp struct {
		Entries []map[string]interface{} `json:"entries"`
		Total   int64                    `json:"total"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &listResp)
	if listResp.Total != 2 {
		t.Fatalf("expected 2 trash items, got %d", listResp.Total)
	}

	// Delete single item via DELETE /api/trash/:id
	firstID := int64(listResp.Entries[0]["id"].(float64))
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", fmt.Sprintf("/api/trash/%d", firstID), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("DELETE /api/trash/:id returned %d, want 200", w.Code)
	}

	// Verify count is now 1
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/trash", nil)
	r.ServeHTTP(w, req)
	_ = json.Unmarshal(w.Body.Bytes(), &listResp)
	if listResp.Total != 1 {
		t.Errorf("expected 1 remaining trash item, got %d", listResp.Total)
	}

	// Empty remaining items via DELETE /api/trash
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("DELETE", "/api/trash", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("DELETE /api/trash returned %d, want 200", w.Code)
	}

	// Verify trash is empty
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/trash", nil)
	r.ServeHTTP(w, req)
	_ = json.Unmarshal(w.Body.Bytes(), &listResp)
	if listResp.Total != 0 {
		t.Errorf("expected 0 trash items after empty, got %d", listResp.Total)
	}
}

func TestOrganizerPlanAndExecuteFlow(t *testing.T) {
	r := setupTestRouter(t)

	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "photo1.jpg")
	_ = os.WriteFile(f1, []byte("fake jpeg content"), 0o644)

	// POST /api/organizer/plan
	planBody, _ := json.Marshal(map[string]interface{}{
		"folder":    tmp,
		"mode":      "sort",
		"template":  "{year}/{month}",
		"operation": "copy",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/organizer/plan", bytes.NewReader(planBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("POST /api/organizer/plan returned %d, want 202", w.Code)
	}

	var planResp struct {
		JobID string `json:"job_id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &planResp)
	if planResp.JobID == "" {
		t.Fatalf("expected job_id from plan start")
	}

	// GET /api/organizer/status/:id
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", fmt.Sprintf("/api/organizer/status/%s", planResp.JobID), nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET status returned %d, want 200", w.Code)
	}
}

func TestNewModulesEndpoints(t *testing.T) {
	r := setupTestRouter(t)
	assets, _ := filepath.Abs("../assets/test")

	// 1. Tagger Scan
	t.Run("TaggerScan", func(t *testing.T) {
		body := fmt.Sprintf(`{"folder": "%s", "recursive": false}`, assets)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/tagger/scan", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusAccepted {
			t.Errorf("expected 202 Accepted, got %d", w.Code)
		}
	})

	// 2. Inspector Analyze
	t.Run("InspectorAnalyze", func(t *testing.T) {
		body := fmt.Sprintf(`{"folder": "%s", "max_top_n": 5}`, assets)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/inspector/analyze", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", w.Code)
		}
	})

	// 3. Gallery Items
	t.Run("GalleryItems", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", fmt.Sprintf("/api/gallery/items?folder=%s&limit=5", assets), nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", w.Code)
		}
	})

	// 4. Trimmer Preview
	t.Run("TrimmerPreview", func(t *testing.T) {
		sampleJpg := filepath.Join(assets, "sample1.jpg")
		body := fmt.Sprintf(`{"source_path": "%s", "resize_w": 50, "resize_h": 50}`, sampleJpg)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/trimmer/preview", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", w.Code)
		}
	})

	// 5. Hasher Generate
	t.Run("HasherGenerate", func(t *testing.T) {
		body := fmt.Sprintf(`{"folder": "%s", "algorithms": ["sha256", "md5"]}`, assets)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/hasher/generate", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusAccepted {
			t.Errorf("expected 202 Accepted, got %d", w.Code)
		}
	})

	// 6. Archiver Create
	t.Run("ArchiverCreate", func(t *testing.T) {
		tmpDest := t.TempDir()
		body := fmt.Sprintf(`{"source_paths": ["%s"], "destination_dir": "%s", "format": "zip", "level": "fast"}`, assets, tmpDest)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/archiver/create", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusAccepted {
			t.Errorf("expected 202 Accepted, got %d", w.Code)
		}
	})

	// 7. Importer Devices
	t.Run("ImporterDevices", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/api/importer/devices", nil)
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", w.Code)
		}
	})

	// 8. Resizer Batch
	t.Run("ResizerBatch", func(t *testing.T) {
		tmpDest := t.TempDir()
		body := fmt.Sprintf(`{"source_dir": "%s", "destination_dir": "%s", "presets": ["thumbnails"]}`, assets, tmpDest)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/resizer/batch", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusAccepted {
			t.Errorf("expected 202 Accepted, got %d", w.Code)
		}
	})

	// 9. Reporter Generate
	t.Run("ReporterGenerate", func(t *testing.T) {
		body := fmt.Sprintf(`{"folder": "%s", "include_storage": true}`, assets)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/reporter/generate", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", w.Code)
		}
	})

	// 10. Syncer Plan
	t.Run("SyncerPlan", func(t *testing.T) {
		targetDir := t.TempDir()
		body := fmt.Sprintf(`{"source_dir": "%s", "target_dir": "%s", "mode": "one-way"}`, assets, targetDir)
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("POST", "/api/syncer/plan", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("expected 200 OK, got %d", w.Code)
		}
	})
}
