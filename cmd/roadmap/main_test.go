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
	cfg := Config{
		Version:         configVersion,
		ActiveProjectID: "project-1",
		Connections:     []Connection{{ID: "github-main", Name: "GitHub", Provider: "github"}},
		Projects:        []Project{{ID: "project-1", Name: "Product", ConnectionID: "github-main", Owner: "acme", Repository: "product", FeatureLabel: "feature"}},
	}
	applyDefaults(&cfg)
	return &app{
		config:      cfg,
		secrets:     Secrets{Tokens: map[string]string{}},
		configPath:  filepath.Join(dir, "config.json"),
		secretsPath: filepath.Join(dir, "secrets.json"),
	}
}

func TestTokenIsStoredPrivatelyAndNeverExported(t *testing.T) {
	a := testApp(t)
	req := httptest.NewRequest(http.MethodPost, "/api/token", strings.NewReader(`{"connection_id":"github-main","token":"top-secret"}`))
	rec := httptest.NewRecorder()
	a.tokenHandler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save token: %d %s", rec.Code, rec.Body.String())
	}
	info, err := os.Stat(a.secretsPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("secrets mode = %o, want 600", info.Mode().Perm())
	}

	rec = httptest.NewRecorder()
	a.exportHandler(rec, httptest.NewRequest(http.MethodGet, "/api/config/export", nil))
	if strings.Contains(rec.Body.String(), "top-secret") || strings.Contains(strings.ToLower(rec.Body.String()), "token") {
		t.Fatalf("export leaked secret data: %s", rec.Body.String())
	}
}

func TestConfigHandlerReportsStatusPerConnectionWithoutSecrets(t *testing.T) {
	a := testApp(t)
	a.secrets.Tokens["github-main"] = "secret"
	rec := httptest.NewRecorder()
	a.configHandler(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	var body struct {
		ConnectionStatus map[string]bool `json:"connection_status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.ConnectionStatus["github-main"] || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}

func TestLegacyConfigIsRejected(t *testing.T) {
	cfg := Config{}
	if err := validateConfig(cfg); err == nil || !strings.Contains(err.Error(), "legacy") {
		t.Fatalf("validateConfig error = %v, want legacy error", err)
	}
}

func TestProjectsCanShareConnectionToken(t *testing.T) {
	a := testApp(t)
	a.config.Projects = append(a.config.Projects, Project{ID: "project-2", Name: "Other", ConnectionID: "github-main", Owner: "acme", Repository: "other", FeatureLabel: "feature"})
	a.secrets.Tokens["github-main"] = "shared"
	a.config.ActiveProjectID = "project-2"
	project, connection, token, err := a.active()
	if err != nil {
		t.Fatal(err)
	}
	if project.ID != "project-2" || connection.ID != "github-main" || token != "shared" {
		t.Fatalf("unexpected active selection: %#v %#v %q", project, connection, token)
	}
}

func TestAppearanceDefaultsAndLimits(t *testing.T) {
	a := testApp(t)
	appearance := a.config.Projects[0].Appearance
	if appearance.TimelineWidth != 1120 || appearance.MilestoneSize != 15 || appearance.LabelFontSize != 13 {
		t.Fatalf("unexpected appearance defaults: %#v", appearance)
	}
	a.config.Projects[0].Appearance.MilestoneSize = 31
	if err := validateConfig(a.config); err == nil || !strings.Contains(err.Error(), "milestone_size") {
		t.Fatalf("validateConfig error = %v, want milestone_size range error", err)
	}
}
