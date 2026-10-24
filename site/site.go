// Package site is wake.calmapps.com: the app's page, and the releases
// that vero builds, the installers and the Sparkle appcast, served from the
// folder the release script writes through vero's kit/site.
//
// It knows nobody. There is no sign-in and nothing is stored. The app
// talks to nothing here but the appcast and the installers.
package site

import (
	"embed"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	kitsite "github.com/imclaren/vero/kit/site"
)

// Config is how the site runs.
type Config struct {
	// BaseURL is the site's own address, https://wake.calmapps.com.
	BaseURL string
	// ReleasesDir is the folder vero-repo build wrote: the installers,
	// appcast.xml and latest.json.
	ReleasesDir string
	// Log is told what goes wrong.
	Log func(format string, args ...any)
}

// Env is the configuration from the environment: BASE_URL and
// RELEASES_DIR, with what a local run wants when they are not set.
func Env() Config {
	cfg := Config{BaseURL: os.Getenv("BASE_URL"), ReleasesDir: os.Getenv("RELEASES_DIR")}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://127.0.0.1:8011"
	}
	if cfg.ReleasesDir == "" {
		cfg.ReleasesDir = "releases"
	}
	return cfg
}

// Site is the site.
type Site struct {
	cfg Config
	mux *http.ServeMux
}

// New makes the site.
func New(cfg Config) *Site {
	if cfg.Log == nil {
		cfg.Log = log.Printf
	}
	s := &Site{cfg: cfg, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /{$}", s.home)
	s.mux.HandleFunc("GET /icon.png", func(w http.ResponseWriter, r *http.Request) {
		data, _ := pages.ReadFile("pages/icon.png")
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Write(data)
	})
	// The installer, for the page's button: kit/site sends the browser to
	// the newest one named in latest.json.
	s.mux.HandleFunc("GET /download", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/download?for=pkg", http.StatusFound)
	})
	// The release folder as it is: its own install page, the appcast where
	// every installed copy looks, latest.json and the installers.
	s.mux.Handle("/releases/", kitsite.Handler(kitsite.Options{Dir: cfg.ReleasesDir, Prefix: "/releases"}))
	s.mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("User-agent: *\nDisallow: /releases/\n"))
	})
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok\n"))
	})
	return s
}

func (s *Site) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

//go:embed pages
var pages embed.FS

var templates = template.Must(template.ParseFS(pages, "pages/*.html"))

// home is the page: what the app does, and the download.
func (s *Site) home(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := map[string]any{
		"BaseURL":  strings.TrimSuffix(s.cfg.BaseURL, "/"),
		"Version":  s.version(),
		"Released": fileExists(filepath.Join(s.cfg.ReleasesDir, "latest.json")),
		"Year":     time.Now().Year(),
	}
	if err := templates.ExecuteTemplate(w, "home.html", data); err != nil {
		s.cfg.Log("home.html: %v", err)
	}
}

// version is the newest release's, from latest.json, or "" before the
// first release.
func (s *Site) version() string {
	data, err := os.ReadFile(filepath.Join(s.cfg.ReleasesDir, "latest.json"))
	if err != nil {
		return ""
	}
	var latest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &latest); err != nil {
		s.cfg.Log("latest.json: %v", err)
		return ""
	}
	return latest.Version
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
