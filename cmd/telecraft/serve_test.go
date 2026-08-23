package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// serve reads exactly one source: a local checkout or a fetched repo.
// Neither, or both, is a usage error, never a guess.
func TestServeCommandRequiresExactlyOneSource(t *testing.T) {
	for name, args := range map[string][]string{
		"no source":    {"serve"},
		"both sources": {"serve", "-estate", "/tmp/a", "-repo", "file:///tmp/b"},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != 2 {
			t.Errorf("%s: exit %d, want 2\n%s", name, code, stderr.String())
		} else if !strings.Contains(stderr.String(), "serve: exactly one of -estate or -repo names the source") {
			t.Errorf("%s: stderr does not say which flags name the source:\n%s", name, stderr.String())
		}
	}
}

func TestServeCommandRejectsAFlagThatDoesNotExist(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"serve", "-estates", "/tmp/a"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined: -estates") {
		t.Errorf("stderr does not name the flag that does not exist:\n%s", stderr.String())
	}
}

// A server with nowhere to listen is refused before anything is served,
// and the refusal is exit 2 because the invocation is what is wrong.
func TestServeCommandRefusesAnEmptyListenAddress(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"serve", "-estate", t.TempDir(), "-listen", ""}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("exit %d, want 2\nstderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "serve: no listen endpoint") {
		t.Errorf("stderr does not say what the server refused:\n%s", stderr.String())
	}
}

// A server that cannot take its first snapshot never starts, and a server
// that never starts is exit 1: the invocation was fine, the source was not.
func TestServeCommandFailsToStartOnASourceItCannotRead(t *testing.T) {
	for name, args := range map[string][]string{
		// A checkout that is not there.
		"a local estate that does not exist": {
			"serve", "-estate", filepath.Join(t.TempDir(), "nowhere"), "-listen", "127.0.0.1:0",
		},
		// A repo URL git cannot clone, into a cache directory the
		// invocation names.
		"a repo git cannot clone": {
			"serve", "-repo", "file:///nowhere/estate.git", "-cache", filepath.Join(t.TempDir(), "clone"), "-listen", "127.0.0.1:0",
		},
		// The same, with no -cache: the clone lands in a fresh temp dir
		// that the command removes on the way out.
		"a repo git cannot clone, with no cache directory": {
			"serve", "-repo", "file:///nowhere/estate.git", "-listen", "127.0.0.1:0",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != 1 {
				t.Fatalf("exit %d, want 1\nstderr:\n%s", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), "serve: ") {
				t.Errorf("stderr does not name the subcommand that failed:\n%s", stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("a server that never started announced an address:\n%s", stdout.String())
			}
		})
	}
}

// Deliberately uncovered: the clean shutdown path, a started server
// waiting on SIGINT or SIGTERM, then stopping. Reaching it from a test
// means signalling the test binary's own process, which every other test
// in the package would then be racing.
