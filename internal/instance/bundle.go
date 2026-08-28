package instance

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/consoleassets"
)

// noConsole is what a binary built without a console answers on the console
// route. The API is served as usual, so a checkout where npm has never run
// still builds a working instance for everything but the browser.
const noConsole = "This binary was built without the console. The API on this address is unaffected.\n"

// console serves the embedded bundle, falling back to index.html so a deep
// link survives a reload: every console surface state is in the URL, and a
// path the bundle owns must load fresh (ADR-0042 §3.5).
//
// The bundle is served signed out, because a 401 from the API is what
// renders the sign-in surface.
func (s *Server) console() http.Handler {
	bundle, ok := consoleassets.FS()
	if !ok {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte(noConsole))
		})
	}
	files := http.FileServerFS(bundle)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if name := assetName(r.URL.Path); name != "" && isFile(bundle, name) {
			files.ServeHTTP(w, r)
			return
		}
		index, err := fs.ReadFile(bundle, "index.html")
		if err != nil {
			http.Error(w, "the console bundle in this binary has no index.html", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// The document names the chunks by hashed file name, so it is the
		// one file that must never be held: a reader on an old index would
		// ask for chunks a new build no longer carries.
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}

// assetName turns a request path into the name of a file in the bundle, or
// empty when it names no file inside it.
func assetName(p string) string {
	name := strings.TrimPrefix(path.Clean("/"+p), "/")
	if name == "" || name == "." || !fs.ValidPath(name) {
		return ""
	}
	return name
}

func isFile(bundle fs.FS, name string) bool {
	info, err := fs.Stat(bundle, name)
	return err == nil && !info.IsDir()
}
