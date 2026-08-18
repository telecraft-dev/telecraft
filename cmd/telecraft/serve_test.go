package main

import (
	"bytes"
	"testing"
)

// serve reads exactly one source: a local checkout or a fetched repo —
// neither, or both, is a usage error, never a guess.
func TestServeCommandRequiresExactlyOneSource(t *testing.T) {
	for name, args := range map[string][]string{
		"no source":    {"serve"},
		"both sources": {"serve", "-estate", "/tmp/a", "-repo", "file:///tmp/b"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Errorf("%s: exit %d, want 2\n%s", name, code, stderr.String())
		}
	}
}
