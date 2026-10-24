// Command server is wake.calmapps.com (package site): the app's page,
// and the releases and Sparkle appcast.
//
//	go run ./cmd/server      # http://127.0.0.1:8011
//
// | Variable     |                                                       |
// | PORT         | 8011 unless set                                       |
// | BASE_URL     | the site's address                                    |
// | RELEASES_DIR | the folder release.sh writes, with the installers,    |
// |              | appcast.xml and latest.json                           |
//
// They come from the process environment, falling back to a .env file
// beside the binary.
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imclaren/wake/site"
)

func main() {
	loadDotEnv()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8011"
	}
	// Bound to localhost: nginx is the way in.
	srv := &http.Server{Addr: "127.0.0.1:" + port, Handler: site.New(site.Env()), ReadHeaderTimeout: 10 * time.Second}
	log.Printf("listening on http://%s", srv.Addr)
	log.Fatal(srv.ListenAndServe())
}

// loadDotEnv reads KEY=VALUE lines from a .env file beside the executable,
// the layout the deploy scripts produce. Values already in the environment
// win, so that systemd or a shell can always override the file.
func loadDotEnv() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(exe), ".env"))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, strings.Trim(strings.TrimSpace(value), `"'`)); err != nil {
			log.Printf("could not set %s from .env: %v", key, err)
		}
	}
}
