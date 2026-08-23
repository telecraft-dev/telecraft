package console

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// locate finds the authored lines that carry one key in one estate file,
// so a "why?" popover shows the config that implied a derived value rather
// than a restatement of the value (ADR-0041 §3). The lines are read from
// the checkout the snapshot is taken from, so file, number and text are the
// repository's own, never reconstructed.
//
// A file or key that is not there yields nothing: a derivation with no
// visible cause says so by showing no lines, which is honest, where an
// invented line would not be.
func (b *builder) locate(rel, key string) []ProvenanceLine {
	f, err := os.Open(filepath.Join(b.in.Root, filepath.FromSlash(rel)))
	if err != nil {
		return nil
	}
	defer f.Close()

	var out []ProvenanceLine
	scanner := bufio.NewScanner(f)
	for n := 1; scanner.Scan(); n++ {
		text := scanner.Text()
		trimmed := strings.TrimSpace(text)
		if !strings.HasPrefix(trimmed, key+":") {
			continue
		}
		out = append(out, ProvenanceLine{File: rel, Line: n, Text: trimmed})
		// One line per key: the first occurrence is the authored one, and
		// a popover listing every indented repetition would bury it.
		break
	}
	return out
}
