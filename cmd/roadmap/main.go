package main

import (
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

type ThemeColors struct {
	Background string `json:"background"`
	Surface    string `json:"surface"`
	Text       string `json:"text"`
	Muted      string `json:"muted"`
	Grid       string `json:"grid"`
	Feature    string `json:"feature"`
	Milestone  string `json:"milestone"`
	Closed     string `json:"closed"`
	Future     string `json:"future"`
}

type Theme struct {
	Default string      `json:"default"`
	Light   ThemeColors `json:"light"`
	Dark    ThemeColors `json:"dark"`
}

type Config struct {
	Provider     string `json:"provider"`
	BaseURL      string `json:"base_url"`
	Owner        string `json:"owner"`
	Repository   string `json:"repository"`
	FeatureLabel string `json:"feature_label"`
	Theme        Theme  `json:"theme"`
}

type app struct {
	mu         sync.RWMutex
	config     Config
	configPath string
	tokenPath  string
}

func loadConfig(path string) (Config, error) {
	var cfg Config
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	applyDefaults(&cfg)
	return cfg, validateConfig(cfg)
}

func applyDefaults(cfg *Config) {
	cfg.Provider = strings.ToLower(strings.TrimSpace(cfg.Provider))
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	cfg.Owner = strings.TrimSpace(cfg.Owner)
	cfg.Repository = strings.TrimSpace(cfg.Repository)
	cfg.FeatureLabel = strings.TrimSpace(cfg.FeatureLabel)
	if cfg.FeatureLabel == "" {
		cfg.FeatureLabel = "type::feature"
	}
	if cfg.Theme.Default == "" {
		cfg.Theme.Default = "auto"
	}
	light := ThemeColors{"#ffffff", "#f7f8fa", "#182230", "#667085", "#d0d5dd", "#1e88e5", "#7e57c2", "#12b76a", "#98a2b3"}
	dark := ThemeColors{"#0d1117", "#161b22", "#e6edf3", "#8b949e", "#30363d", "#58a6ff", "#bc8cff", "#32d583", "#667085"}
	fillColors(&cfg.Theme.Light, light)
	fillColors(&cfg.Theme.Dark, dark)
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
	if colors.Closed == "" {
		colors.Closed = defaults.Closed
	}
	if colors.Future == "" {
		colors.Future = defaults.Future
	}
}

func validateConfig(cfg Config) error {
	switch strings.ToLower(cfg.Provider) {
	case "github", "gitlab", "gitea":
	default:
		return fmt.Errorf("unsupported provider %q", cfg.Provider)
	}
	if strings.TrimSpace(cfg.Owner) == "" || strings.TrimSpace(cfg.Repository) == "" {
		return errors.New("owner and repository are required")
	}
	if cfg.Provider == "gitea" && cfg.BaseURL == "" {
		return errors.New("base_url is required for Gitea")
	}
	if strings.TrimSpace(cfg.FeatureLabel) == "" {
		return errors.New("feature_label is required")
	}
	if cfg.Theme.Default != "auto" && cfg.Theme.Default != "light" && cfg.Theme.Default != "dark" {
		return errors.New("theme.default must be auto, light, or dark")
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

func (a *app) snapshot() Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.config
}

func (a *app) saveConfig(cfg Config) error {
	applyDefaults(&cfg)
	if err := validateConfig(cfg); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := writeFileAtomic(a.configPath, b, 0o644); err != nil {
		return err
	}
	a.mu.Lock()
	a.config = cfg
	a.mu.Unlock()
	return nil
}

func (a *app) token() (string, error) {
	b, err := os.ReadFile(a.tokenPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	token := strings.TrimSpace(string(b))
	if strings.IndexFunc(token, unicode.IsSpace) >= 0 {
		return "", errors.New("saved token contains whitespace; reconnect it in Settings")
	}
	return token, err
}

func makeProvider(cfg Config, token string) provider.Provider {
	switch strings.ToLower(cfg.Provider) {
	case "gitea":
		return provider.NewGitea(cfg.BaseURL, token, cfg.Owner, cfg.Repository)
	case "gitlab":
		return provider.NewGitLab(cfg.BaseURL, token, cfg.Owner, cfg.Repository)
	default:
		return provider.NewGitHub(cfg.BaseURL, token, cfg.Owner, cfg.Repository)
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
		token, err := a.token()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"config": a.snapshot(), "connected": token != ""})
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
	w.Header().Set("Content-Disposition", `attachment; filename="roadmap-config.json"`)
	writeJSON(w, http.StatusOK, a.snapshot())
}

func (a *app) tokenHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var body struct {
			Token string `json:"token"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		body.Token = strings.TrimSpace(body.Token)
		if body.Token == "" {
			http.Error(w, "token is required", http.StatusBadRequest)
			return
		}
		if strings.IndexFunc(body.Token, unicode.IsSpace) >= 0 {
			http.Error(w, "token must not contain whitespace", http.StatusBadRequest)
			return
		}
		if err := writeFileAtomic(a.tokenPath, []byte(body.Token+"\n"), 0o600); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"connected": true})
	case http.MethodDelete:
		if err := os.Remove(a.tokenPath); err != nil && !errors.Is(err, os.ErrNotExist) {
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
	token, err := a.token()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cfg := a.snapshot()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	data, err := service.New(makeProvider(cfg, token), cfg.FeatureLabel).Load(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, data)
}

func main() {
	cfgPath := flag.String("config", "config.json", "config file")
	tokenPath := flag.String("token-file", "token", "token file")
	addr := flag.String("listen", "127.0.0.1:8080", "listen address")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	a := &app{config: cfg, configPath: *cfgPath, tokenPath: *tokenPath}
	if err := os.Chmod(*tokenPath, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatal(err)
	}

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
