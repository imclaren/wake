package site

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHome checks the page before the first release and after it, when the
// button shows the version from latest.json.
func TestHome(t *testing.T) {
	dir := t.TempDir()
	s := New(Config{BaseURL: "https://example.com", ReleasesDir: dir, Log: t.Logf})

	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "The first release is on its way.") {
		t.Fatalf("before a release: %d %q", rec.Code, rec.Body.String())
	}

	if err := os.WriteFile(filepath.Join(dir, "latest.json"), []byte(`{"name":"wake","version":"1.2.3","downloads":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(rec.Body.String(), "Version 1.2.3, as") {
		t.Fatalf("after a release: %q", rec.Body.String())
	}
}

// TestDownload checks that /download sends the browser to the installer
// through the release folder's own redirect.
func TestDownload(t *testing.T) {
	s := New(Config{BaseURL: "https://example.com", ReleasesDir: t.TempDir(), Log: t.Logf})
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "/download", nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/releases/download?for=pkg" {
		t.Fatalf("/download: %d to %q", rec.Code, rec.Header().Get("Location"))
	}
}

// TestSmallPages checks the privacy page, the icon, the panel's picture,
// robots.txt and the health check.
func TestSmallPages(t *testing.T) {
	s := New(Config{BaseURL: "https://example.com", ReleasesDir: t.TempDir(), Log: t.Logf})
	for path, want := range map[string]string{"/privacy": "text/html; charset=utf-8", "/icon.png": "image/png", "/favicon.ico": "image/png", "/demo/panel.png": "image/png", "/robots.txt": "text/plain; charset=utf-8", "/healthz": "text/plain; charset=utf-8"} {
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != want {
			t.Errorf("%s: %d %s, want 200 %s", path, rec.Code, rec.Header().Get("Content-Type"), want)
		}
	}
}
