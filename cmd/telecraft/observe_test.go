package main

import (
	"bytes"
	"strings"
	"testing"
)

// observe is a printer, not a gate: a backend that answers nothing still
// exits 0, with the cause beside every reading (ADR-0008). What follows
// covers the paths a user reaches by mistyping the invocation, and asserts
// on the message each one prints rather than only on the exit code.

// unreachableBackend is a port nothing listens on, so every reading comes
// back degraded and the suite needs no telemetry backend.
const unreachableBackend = "http://127.0.0.1:1"

func TestObserveCommandUsageErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"no service": {
			args: []string{"observe"},
			want: "observe: -service is required",
		},
		"a flag that does not exist": {
			args: []string{"observe", "-services", "checkout"},
			want: "flag provided but not defined: -services",
		},
		"a window that is not a duration": {
			args: []string{"observe", "-service", "checkout", "-window", "fortnight"},
			want: "invalid value \"fortnight\" for flag -window",
		},
		// An empty endpoint reaches the provider constructor, which is the
		// last thing observe can refuse before it has read anything.
		"an empty endpoint": {
			args: []string{"observe", "-service", "checkout", "-endpoint", ""},
			want: "endpoint is required",
		},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(tc.args, &stdout, &stderr); code != 2 {
				t.Fatalf("exit %d, want 2", code)
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("stderr lacks %q:\n%s", tc.want, stderr.String())
			}
		})
	}
}

// A backend that cannot be reached leaves every signal unknown. The reading
// prints its cause and observe still exits 0: scripting against presence
// belongs to check, not to this printer.
func TestObserveCommandPrintsDegradedReadingsAndExitsZero(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"observe",
		"-service", "checkout",
		"-environment", "production",
		"-endpoint", unreachableBackend,
		"-window", "5m",
		// The empty entries are the ones a shell leaves behind; they are
		// trimmed rather than measured as an attribute named "".
		"-attributes", "service.version, deployment.environment.name, ,",
		"-timeout", "10s",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, want 0: not knowing is a normal state\nstderr:\n%s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"service   checkout",
		"env       production",
		"window    5m",
		"known=false",
		"attribute names: known=false",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

// The endpoint falls back to the environment, so a shell that exports it
// once does not repeat it on every invocation.
func TestObserveCommandTakesItsEndpointFromTheEnvironment(t *testing.T) {
	t.Setenv("TELECRAFT_TELEMETRY_ENDPOINT", unreachableBackend)
	t.Setenv("TELECRAFT_TELEMETRY_API_KEY", "not-a-real-key")
	var stdout, stderr bytes.Buffer

	code := run([]string{"observe", "-service", "checkout", "-timeout", "10s"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit %d, want 0\nstderr:\n%s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "env       ") {
		t.Errorf("an unnarrowed reading printed an Environment line:\n%s", stdout.String())
	}
}

// Deliberately uncovered: the branches that print a known reading
// (present, volume, per-attribute coverage, and the sampled attribute-name
// list. Reaching them means a telemetry backend answering real queries,
// which the provider's own live suite covers against a real one rather
// than against a double standing in for it here.
