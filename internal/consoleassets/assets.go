// Package consoleassets carries the built console inside the binary.
//
// A single binary plus a directory is a complete instance (ADR-0032 §3),
// and an instance whose UI is a separate tree somebody has to place beside
// it does not keep that promise. One artefact also means one version, which
// is what the console assumes when it names the release it came from
// (ADR-0065), and the zero-CDN rule then holds at run time by construction
// rather than only over the build (ADR-0045 §5).
//
// The bundle is staged into dist/ by `npm run bundle` in console/, because
// an embed pattern cannot reach outside its own package. dist/ holds one
// tracked placeholder and nothing else, so `go build ./...`, `go vet ./...`
// and `go test ./...` all pass on a checkout where npm has never run: the
// binary is built without a console and says so on the console route
// (ADR-0067 §3).
package consoleassets

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var staged embed.FS

// indexFile is the document a single-page bundle is served from, and the
// one file whose presence answers whether there is a bundle at all.
const indexFile = "index.html"

// FS is the embedded bundle, rooted where index.html sits. The second
// return is false when no console was built into this binary.
func FS() (fs.FS, bool) {
	dist, err := fs.Sub(staged, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(dist, indexFile); err != nil {
		return nil, false
	}
	return dist, true
}
