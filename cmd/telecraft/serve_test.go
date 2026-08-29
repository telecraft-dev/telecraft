package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/secrets"
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

// The HTTP endpoint carries the console, the API and both probes, so a
// server with nowhere to put it is refused before anything is served. The
// refusal is exit 2 because the invocation is what is wrong.
func TestServeCommandRefusesAnEmptyHTTPAddress(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"serve", "-estate", t.TempDir(), "-http", ""}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("exit %d, want 2\nstderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "serve: no HTTP endpoint") {
		t.Errorf("stderr does not say what the server refused:\n%s", stderr.String())
	}
}

// An empty OpAMP address is not a mistake: it closes that endpoint, which
// is the shape of an instance whose collectors are all read through a
// vendor provider. So the command gets past its own checks and fails on
// the estate it was pointed at, which is the next thing wrong here.
//
// What it fails on moved. An empty directory used to be refused for having
// no Tiers; a new estate is now served rather than refused (issue #243), so
// what is left wrong with an empty directory is the team tree, without
// which no finding can reach an owner. The point of the test is unchanged:
// the flags were accepted and the estate was not.
func TestServeCommandTakesAnEmptyOpAMPAddressAsAClosedEndpoint(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"serve", "-estate", t.TempDir(), "-http", "127.0.0.1:0", "-listen", ""}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("exit %d, want 1\nstderr:\n%s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "no HTTP endpoint") {
		t.Errorf("the flags were refused, so the estate was never reached:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "teams.yaml") {
		t.Errorf("stderr does not name the estate it could not read:\n%s", stderr.String())
	}
}

// A password crossing a network in clear text is not a default: an
// external URL naming a non-loopback host over plain HTTP is refused
// unless the operator says they mean it.
func TestServeCommandRefusesPlainHTTPAcrossANetwork(t *testing.T) {
	estate := t.TempDir()
	var stdout, stderr bytes.Buffer

	code := run([]string{"serve", "-estate", estate, "-external-url", "http://telecraft.example"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("exit %d, want 2\nstderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "clear text") {
		t.Errorf("stderr does not say what is wrong with the URL:\n%s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = run([]string{"serve", "-estate", estate, "-external-url", "http://telecraft.example", "-insecure-http", "-http", "127.0.0.1:0"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("saying it is meant did not get past the check: exit %d, want 1\nstderr:\n%s", code, stderr.String())
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

// Every flag has an environment variable, so a container configures the
// process without an entrypoint that rewrites arguments. A flag beats one.
func TestServeCommandReadsFlagsFromTheEnvironment(t *testing.T) {
	estate := t.TempDir()
	t.Setenv("TELECRAFT_HTTP", "")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"serve", "-estate", estate}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d, want 2: the environment named no HTTP endpoint\nstderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no HTTP endpoint") {
		t.Errorf("stderr does not say what the environment configured:\n%s", stderr.String())
	}

	// The flag beats the environment variable, so the same run gets past
	// the check and fails on the estate instead.
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"serve", "-estate", estate, "-http", "127.0.0.1:0"}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit %d, want 1: the flag did not beat the environment variable\nstderr:\n%s", code, stderr.String())
	}
}

// No flag carries secret material. The process's own secrets take a file
// path, defaulting to a documented name under the secret directory, and the
// two absences are told apart: a path somebody named and this process
// cannot read is a mistake, and a defaulted path with nothing at it is an
// absence.
func TestTheProcessReadsItsOwnSecretsFromFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, sessionKeyName), []byte("a signing key of at least 32 bytes\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	path, named := secretFile("", false, secrets.Dir(dir), sessionKeyName)
	if named {
		t.Error("a defaulted path reads as one somebody named")
	}
	key, err := readSecretFile(path, named)
	if err != nil {
		t.Fatal(err)
	}
	if string(key) != "a signing key of at least 32 bytes" {
		t.Errorf("key = %q, want the file's contents with the trailing newline stripped", key)
	}

	// Nothing placed, and nothing named: the capability is unavailable,
	// which for the session key means one is drawn at start.
	path, named = secretFile("", false, secrets.Dir(t.TempDir()), sessionKeyName)
	if key, err := readSecretFile(path, named); err != nil || key != nil {
		t.Errorf("an absent default gave %q, %v, want no key and no error", key, err)
	}

	// A path somebody named and this process cannot read is a mistake.
	path, named = secretFile(filepath.Join(dir, "nothing-here"), true, secrets.Dir(dir), sessionKeyName)
	if _, err := readSecretFile(path, named); err == nil {
		t.Error("a named path that does not exist was taken as an absence")
	}
}

// A secret directory nobody filled refuses to start an Instance whose
// estate names a secret, rather than withdrawing sign-in quietly.
func TestServeCommandRefusesASecretNobodyPlaced(t *testing.T) {
	estate := estateWithProviders(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"serve", "-estate", estate, "-http", "127.0.0.1:0", "-listen", "", "-secrets-dir", t.TempDir()}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("exit %d, want 1\nstderr:\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "staff-oidc") {
		t.Errorf("stderr does not name the secret nobody placed:\n%s", stderr.String())
	}
}

// estateWithProviders is the smallest estate that names a secret: enough
// files for the server to reach the auth wiring and refuse there.
func estateWithProviders(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join("..", "..", "internal", "console", "testdata", "estate"))); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("users.yaml", "users:\n  - email: jo@example.com\n    name: Jo Author\n    owner: gateway-owners\n")
	write("auth.yaml", "providers:\n  - kind: oidc\n    name: staff\n    issuer: https://issuer.example\n    client_id: telecraft\n    secret: staff-oidc\n")
	return root
}
