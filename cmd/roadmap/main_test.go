package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testApp(t *testing.T) *app {
	t.Helper()
	dir := t.TempDir()
	cfg := Config{Provider: "github", Owner: "acme", Repository: "product", FeatureLabel: "feature"}
	applyDefaults(&cfg)
	return &app{config: cfg, configPath: filepath.Join(dir, "config.json"), tokenPath: filepath.Join(dir, "token")}
}

func TestTokenIsStoredPrivatelyAndNeverExported(t *testing.T) {
	a := testApp(t)
	req := httptest.NewRequest(http.MethodPost, "/api/token", strings.NewReader(`{"token":"top-secret"}`))
	rec := httptest.NewRecorder()
	a.tokenHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save token: %d %s", rec.Code, rec.Body.String())
	}
	info, err := os.Stat(a.tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("token mode = %o, want 600", info.Mode().Perm())
	}

	rec = httptest.NewRecorder()
	a.exportHandler(rec, httptest.NewRequest(http.MethodGet, "/api/config/export", nil))
	if strings.Contains(rec.Body.String(), "top-secret") || strings.Contains(strings.ToLower(rec.Body.String()), "token") {
		t.Fatalf("export leaked token data: %s", rec.Body.String())
	}
}

func TestConfigHandlerReportsConnectionWithoutReturningToken(t *testing.T) {
	a := testApp(t)
	if err := os.WriteFile(a.tokenPath, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	a.configHandler(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["connected"] != true || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}
