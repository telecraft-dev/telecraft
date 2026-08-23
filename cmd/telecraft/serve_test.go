package main

// The success path of runServe — printing the address, waiting for a signal,
// and stopping cleanly — requires a real estate with a valid rendered tree, a
// free port, and a process signal to trigger shutdown. That cannot be
// exercised without either a full estate fixture or a goroutine that sends
// itself a signal, neither of which belongs in a flag-and-error test file.

import (
	"bytes"
	"strings"
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

func TestServeCommandRejectsBadFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"serve", "-bogus-flag"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

func TestServeFailsWhenEstateCannotSnapshot(t *testing.T) {
	// An empty directory has no topology, so the initial snapshot fails.
	// The server returns exit 1, not 2, because the flags were valid —
	// the source was just unusable.
	var stdout, stderr bytes.Buffer
	code := run([]string{"serve", "-estate", t.TempDir()}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit %d, want 1 (serve start failed)\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "serve:") {
		t.Errorf("stderr does not carry the serve prefix:\n%s", stderr.String())
	}
}
