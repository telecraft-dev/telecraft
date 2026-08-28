package consoleassets

import (
	"io/fs"
	"testing"
)

// The package answers the same way whether or not a console was staged
// before the build, because both are ordinary states: a release binary
// carries one, and a checkout where npm has never run does not.
func TestTheAnswerMatchesWhatWasStaged(t *testing.T) {
	dist, err := fs.Sub(staged, "dist")
	if err != nil {
		t.Fatalf("the embedded directory is missing, so this package would not build: %v", err)
	}
	_, statErr := fs.Stat(dist, indexFile)

	bundle, ok := FS()
	if ok != (statErr == nil) {
		t.Fatalf("FS() said %v about a bundle whose index.html gave %v", ok, statErr)
	}
	if !ok {
		if bundle != nil {
			t.Error("FS() answered no bundle and returned one anyway")
		}
		return
	}
	body, err := fs.ReadFile(bundle, indexFile)
	if err != nil {
		t.Fatalf("the staged bundle's document does not read: %v", err)
	}
	if len(body) == 0 {
		t.Error("the staged bundle's document is empty")
	}
}
