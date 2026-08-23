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
	r, err := web.SetupRouter(context.Background())
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

	if _, err := os.Stat(f1); !os.IsNotExist(err) {
		t.Errorf("expected file %s to be deleted", f1)
	}
}
