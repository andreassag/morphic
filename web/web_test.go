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
