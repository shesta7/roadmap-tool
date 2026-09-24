package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"roadmap-tool/internal/provider"
	"roadmap-tool/internal/service"
)

//go:embed web/templates/index.html web/static/*
var assets embed.FS

const configVersion = 2

type ThemeColors struct {
	Background string `json:"background"`
	Surface    string `json:"surface"`
	Text       string `json:"text"`
	Muted      string `json:"muted"`
	Grid       string `json:"grid"`
	Feature    string `json:"feature"`
	Milestone  string `json:"milestone"`
	Current    string `json:"current"`
	Closed     string `json:"closed"`
	Future     string `json:"future"`
}

type Theme struct {
	Default string      `json:"default"`
	Light   ThemeColors `json:"light"`
	Dark    ThemeColors `json:"dark"`
}

type RoadmapAppearance struct {
	TimelineWidth    int `json:"timeline_width"`
	TimelineHeight   int `json:"timeline_height"`
	MilestoneSize    int `json:"milestone_size"`
	SelectedRing     int `json:"selected_ring"`
	LabelWidth       int `json:"label_width"`
	LabelFontSize    int `json:"label_font_size"`
	FeatureRowHeight int `json:"feature_row_height"`
}

type Connection struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	BaseURL  string `json:"base_url"`
}

type Project struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	ConnectionID string            `json:"connection_id"`
	Owner        string            `json:"owner"`
	Repository   string            `json:"repository"`
	FeatureLabel string            `json:"feature_label"`
	Theme        Theme             `json:"theme"`
	Appearance   RoadmapAppearance `json:"appearance"`
}

type Config struct {
	Version         int          `json:"version"`
	ActiveProjectID string       `json:"active_project_id"`
	Connections     []Connection `json:"connections"`
	Projects        []Project    `json:"projects"`
}

type Secrets struct {
	Tokens map[string]string `json:"tokens"`
}

type app struct {
	mu          sync.RWMutex
	config      Config
	secrets     Secrets
	configPath  string
	secretsPath string
}

func decodeStrict(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func loadConfig(path string) (Config, error) {
	var cfg Config
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := decodeStrict(b, &cfg); err != nil {
		return cfg, fmt.Errorf("invalid config: %w", err)
	}
	applyDefaults(&cfg)
	return cfg, validateConfig(cfg)
}

func loadSecrets(path string) (Secrets, error) {
	secrets := Secrets{Tokens: map[string]string{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return secrets, nil
	}
	if err != nil {
		return secrets, err
	}
	if err := decodeStrict(b, &secrets); err != nil {
		return secrets, fmt.Errorf("invalid secrets file: %w", err)
	}
	if secrets.Tokens == nil {
		secrets.Tokens = map[string]string{}
	}
	for id, token := range secrets.Tokens {
		if strings.TrimSpace(id) == "" || invalidToken(token) {
			return Secrets{}, errors.New("secrets file contains an invalid connection ID or token")
		}
	}
	return secrets, nil
}

func applyDefaults(cfg *Config) {
	for i := range cfg.Connections {
		c := &cfg.Connections[i]
		c.ID = strings.TrimSpace(c.ID)
		c.Name = strings.TrimSpace(c.Name)
		c.Provider = strings.ToLower(strings.TrimSpace(c.Provider))
		c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	}
	for i := range cfg.Projects {
		p := &cfg.Projects[i]
		p.ID = strings.TrimSpace(p.ID)
		p.Name = strings.TrimSpace(p.Name)
		p.ConnectionID = strings.TrimSpace(p.ConnectionID)
		p.Owner = strings.TrimSpace(p.Owner)
		p.Repository = strings.TrimSpace(p.Repository)
		p.FeatureLabel = strings.TrimSpace(p.FeatureLabel)
		if p.Name == "" {
			p.Name = p.Repository
		}
		if p.FeatureLabel == "" {
			p.FeatureLabel = "type::feature"
		}
		applyThemeDefaults(&p.Theme)
		applyAppearanceDefaults(&p.Appearance)
	}
}

func applyThemeDefaults(theme *Theme) {
	if theme.Default == "" {
		theme.Default = "auto"
	}
	light := ThemeColors{Background: "#ffffff", Surface: "#f7f8fa", Text: "#182230", Muted: "#667085", Grid: "#d0d5dd", Feature: "#1e88e5", Milestone: "#7e57c2", Current: "#344054", Closed: "#12b76a", Future: "#98a2b3"}
	dark := ThemeColors{Background: "#0d1117", Surface: "#161b22", Text: "#e6edf3", Muted: "#8b949e", Grid: "#30363d", Feature: "#58a6ff", Milestone: "#bc8cff", Current: "#e6edf3", Closed: "#32d583", Future: "#667085"}
	fillColors(&theme.Light, light)
	fillColors(&theme.Dark, dark)
}

func applyAppearanceDefaults(appearance *RoadmapAppearance) {
	if appearance.TimelineWidth == 0 {
		appearance.TimelineWidth = 1120
	}
	if appearance.TimelineHeight == 0 {
		appearance.TimelineHeight = 178
	}
	if appearance.MilestoneSize == 0 {
		appearance.MilestoneSize = 15
	}
	if appearance.SelectedRing == 0 {
		appearance.SelectedRing = 2
	}
	if appearance.LabelWidth == 0 {
		appearance.LabelWidth = 210
	}
	if appearance.LabelFontSize == 0 {
		appearance.LabelFontSize = 13
	}
	if appearance.FeatureRowHeight == 0 {
		appearance.FeatureRowHeight = 49
	}
}

func fillColors(colors *ThemeColors, defaults ThemeColors) {
	if colors.Background == "" {
		colors.Background = defaults.Background
	}
	if colors.Surface == "" {
		colors.Surface = defaults.Surface
	}
	if colors.Text == "" {
		colors.Text = defaults.Text
	}
	if colors.Muted == "" {
		colors.Muted = defaults.Muted
	}
	if colors.Grid == "" {
		colors.Grid = defaults.Grid
	}
	if colors.Feature == "" {
		colors.Feature = defaults.Feature
	}
	if colors.Milestone == "" {
		colors.Milestone = defaults.Milestone
	}
	if colors.Current == "" {
		colors.Current = defaults.Current
	}
	if colors.Closed == "" {
		colors.Closed = defaults.Closed
	}
	if colors.Future == "" {
		colors.Future = defaults.Future
	}
}

func validateConfig(cfg Config) error {
	if cfg.Version != configVersion {
		return fmt.Errorf("config version must be %d (legacy configurations are not supported)", configVersion)
	}
	if len(cfg.Connections) == 0 || len(cfg.Projects) == 0 {
		return errors.New("at least one connection and one project are required")
	}
	connections := make(map[string]bool, len(cfg.Connections))
	for _, c := range cfg.Connections {
		if c.ID == "" || c.Name == "" {
			return errors.New("connection id and name are required")
		}
		if connections[c.ID] {
			return fmt.Errorf("duplicate connection id %q", c.ID)
		}
		connections[c.ID] = true
		switch c.Provider {
		case "github", "gitlab", "gitea":
		default:
			return fmt.Errorf("unsupported provider %q", c.Provider)
		}
		if c.Provider == "gitea" && c.BaseURL == "" {
			return fmt.Errorf("base_url is required for Gitea connection %q", c.Name)
		}
	}
	projects := make(map[string]bool, len(cfg.Projects))
	for _, p := range cfg.Projects {
		if p.ID == "" || p.Name == "" {
			return errors.New("project id and name are required")
		}
		if projects[p.ID] {
			return fmt.Errorf("duplicate project id %q", p.ID)
		}
		projects[p.ID] = true
		if !connections[p.ConnectionID] {
			return fmt.Errorf("project %q refers to an unknown connection", p.Name)
		}
		if p.Owner == "" || p.Repository == "" || p.FeatureLabel == "" {
			return fmt.Errorf("owner, repository, and feature_label are required for project %q", p.Name)
		}
		if p.Theme.Default != "auto" && p.Theme.Default != "light" && p.Theme.Default != "dark" {
			return fmt.Errorf("invalid default theme for project %q", p.Name)
		}
		if err := validateAppearance(p.Name, p.Appearance); err != nil {
			return err
		}
	}
	if !projects[cfg.ActiveProjectID] {
		return errors.New("active_project_id must refer to an existing project")
	}
	return nil
}

func validateAppearance(project string, appearance RoadmapAppearance) error {
	ranges := []struct {
		name     string
		value    int
		min, max int
	}{
		{"timeline_width", appearance.TimelineWidth, 720, 2400},
		{"timeline_height", appearance.TimelineHeight, 140, 280},
		{"milestone_size", appearance.MilestoneSize, 12, 30},
		{"selected_ring", appearance.SelectedRing, 1, 8},
		{"label_width", appearance.LabelWidth, 120, 360},
		{"label_font_size", appearance.LabelFontSize, 10, 20},
		{"feature_row_height", appearance.FeatureRowHeight, 40, 80},
	}
	for _, item := range ranges {
		if item.value < item.min || item.value > item.max {
			return fmt.Errorf("%s for project %q must be between %d and %d", item.name, project, item.min, item.max)
		}
	}
	return nil
}

func writeFileAtomic(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".roadmap-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func marshalFile(value any) ([]byte, error) {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func (a *app) snapshot() Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config
}

func (a *app) connectionStatus() map[string]bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	status := make(map[string]bool, len(a.config.Connections))
	for _, c := range a.config.Connections {
		status[c.ID] = a.secrets.Tokens[c.ID] != ""
	}
	return status
}

func (a *app) saveConfig(cfg Config) error {
	applyDefaults(&cfg)
	if err := validateConfig(cfg); err != nil {
		return err
	}
	b, err := marshalFile(cfg)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(a.configPath, b, 0o644); err != nil {
		return err
	}
	a.mu.Lock()
	a.config = cfg
	a.mu.Unlock()
	return nil
}

func invalidToken(token string) bool {
	return strings.TrimSpace(token) == "" || strings.IndexFunc(token, unicode.IsSpace) >= 0
}

func (a *app) saveToken(connectionID, token string) error {
	if invalidToken(token) {
		return errors.New("token must not be empty or contain whitespace")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	found := false
	for _, c := range a.config.Connections {
		if c.ID == connectionID {
			found = true
			break
		}
	}
	if !found {
		return errors.New("unknown connection")
	}
	next := Secrets{Tokens: make(map[string]string, len(a.secrets.Tokens)+1)}
	for id, value := range a.secrets.Tokens {
		next.Tokens[id] = value
	}
	next.Tokens[connectionID] = token
	b, err := marshalFile(next)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(a.secretsPath, b, 0o600); err != nil {
		return err
	}
	a.secrets = next
	return nil
}

func (a *app) deleteToken(connectionID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	next := Secrets{Tokens: make(map[string]string, len(a.secrets.Tokens))}
	for id, value := range a.secrets.Tokens {
		if id != connectionID {
			next.Tokens[id] = value
		}
	}
	b, err := marshalFile(next)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(a.secretsPath, b, 0o600); err != nil {
		return err
	}
	a.secrets = next
	return nil
}

func (a *app) active() (Project, Connection, string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var project Project
	for _, p := range a.config.Projects {
		if p.ID == a.config.ActiveProjectID {
			project = p
			break
		}
	}
	if project.ID == "" {
		return Project{}, Connection{}, "", errors.New("active project not found")
	}
	var connection Connection
	for _, c := range a.config.Connections {
		if c.ID == project.ConnectionID {
			connection = c
			break
		}
	}
	if connection.ID == "" {
		return Project{}, Connection{}, "", errors.New("project connection not found")
	}
	return project, connection, a.secrets.Tokens[connection.ID], nil
}

func makeProvider(connection Connection, project Project, token string) provider.Provider {
	switch connection.Provider {
	case "gitea":
		return provider.NewGitea(connection.BaseURL, token, project.Owner, project.Repository)
	case "gitlab":
		return provider.NewGitLab(connection.BaseURL, token, project.Owner, project.Repository)
	default:
		return provider.NewGitHub(connection.BaseURL, token, project.Owner, project.Repository)
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func onlyMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method != method {
		w.Header().Set("Allow", method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

func (a *app) configHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"config": a.snapshot(), "connection_status": a.connectionStatus()})
	case http.MethodPut:
		var cfg Config
		if !decodeJSON(w, r, &cfg) {
			return
		}
		if err := a.saveConfig(cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		w.Header().Set("Allow", "GET, PUT")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *app) exportHandler(w http.ResponseWriter, r *http.Request) {
	if !onlyMethod(w, r, http.MethodGet) {
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="roadmap-library.json"`)
	writeJSON(w, http.StatusOK, a.snapshot())
}

func (a *app) tokenHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var body struct {
			ConnectionID string `json:"connection_id"`
			Token        string `json:"token"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if err := a.saveToken(strings.TrimSpace(body.ConnectionID), strings.TrimSpace(body.Token)); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"connected": true})
	case http.MethodDelete:
		id := strings.TrimSpace(r.URL.Query().Get("connection_id"))
		if id == "" {
			http.Error(w, "connection_id is required", http.StatusBadRequest)
			return
		}
		if err := a.deleteToken(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"connected": false})
	default:
		w.Header().Set("Allow", "POST, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *app) roadmapHandler(w http.ResponseWriter, r *http.Request) {
	if !onlyMethod(w, r, http.MethodGet) {
		return
	}
	project, connection, token, err := a.active()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	data, err := service.New(makeProvider(connection, project, token), project.FeatureLabel).Load(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func main() {
	cfgPath := flag.String("config", "config.json", "configuration library file")
	secretsPath := flag.String("secrets", "secrets.json", "connection secrets file")
	addr := flag.String("listen", "127.0.0.1:8080", "listen address")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	secrets, err := loadSecrets(*secretsPath)
	if err != nil {
		log.Fatal(err)
	}
	if err := os.Chmod(*secretsPath, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatal(err)
	}
	a := &app{config: cfg, secrets: secrets, configPath: *cfgPath, secretsPath: *secretsPath}

	index, err := assets.ReadFile("web/templates/index.html")
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
	mux.HandleFunc("/api/config", a.configHandler)
	mux.HandleFunc("/api/config/export", a.exportHandler)
	mux.HandleFunc("/api/token", a.tokenHandler)
	mux.HandleFunc("/api/roadmap", a.roadmapHandler)
	staticSub, err := fs.Sub(assets, "web/static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	server := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	fmt.Printf("roadmap-tool listening on %s\n", *addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
