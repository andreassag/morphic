package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/exterex/morphic/web"
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

