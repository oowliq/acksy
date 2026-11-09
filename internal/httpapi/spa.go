package httpapi

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

const notBuiltPage = `<!doctype html><meta charset="utf-8"><title>Acksy</title>
<p>The web UI is not built. Run <code>make web</code> (or <code>pnpm -C web build</code>) and rebuild the server.</p>`

// spaHandler serves static files from fsys and falls back to index.html for
// client-side routes. Hashed assets are cached forever; index.html never.
func spaHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServerFS(fsys)
	hasIndex := fileExists(fsys, "index.html")
	csp := spaCSP(fsys)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", csp)
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}
		if !hasIndex {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(notBuiltPage))
			return
		}

		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" && name != "index.html" && fileExists(fsys, name) {
			switch {
			case strings.HasPrefix(name, "assets/"):
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			case name == "sw.js" || name == "manifest.webmanifest":
				// Unhashed: browsers must pick up new versions promptly.
				w.Header().Set("Cache-Control", "no-cache")
			}
			if name == "manifest.webmanifest" {
				w.Header().Set("Content-Type", "application/manifest+json")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// Unknown file with an extension (e.g. /missing.js) is a real 404,
		// everything else is a client-side route.
		if path.Ext(name) != "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, fsys, "index.html")
	})
}

func fileExists(fsys fs.FS, name string) bool {
	st, err := fs.Stat(fsys, name)
	return err == nil && !st.IsDir()
}
