// Package spa serves the built single-page apps with history fallback.
package spa

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"shippin.cloud/console/internal/auth"
)

// Handler serves dir with index.html as the fallback page.
func Handler(dir string) http.Handler { return HandlerFor(dir, "index.html") }

// HandlerFor serves dir with index (e.g. ops.html, the operator app's entry) as the fallback page.
func HandlerFor(dir, index string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAPI(r.URL.Path) {
			auth.Problem(w, http.StatusNotFound, "unknown API path")
			return
		}
		p := filepath.Join(dir, filepath.Clean("/"+r.URL.Path))
		if st, err := os.Stat(p); err != nil || st.IsDir() || isEntry(r.URL.Path) {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFile(w, r, filepath.Join(dir, index))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fs.ServeHTTP(w, r)
	})
}

// isEntry: the HTML entry files are only ever served as the fallback of their own origin.
func isEntry(path string) bool { return path == "/index.html" || path == "/ops.html" }

// isAPI: /v1 and /v1/* are API space; anything reaching the SPA there is an unknown endpoint, never index.html.
func isAPI(path string) bool { return path == "/v1" || strings.HasPrefix(path, "/v1/") }
