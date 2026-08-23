package main

// The observation body — querying the telemetry backend and printing each
// signal's reading — requires a live telemetry backend and is not tested
// here. What is covered: the three error paths a user hits before any backend
// call (missing -service, unknown flag, explicitly empty -endpoint).

import (
	"bytes"
	"strings"
	"testing"
)

// observe exits 2 on any usage error before it ever tries the backend, so
// these paths are exercised by running the command directly without a real
// telemetry server.

func TestObserveCommandRequiresService(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"observe"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "-service is required") {
		t.Errorf("stderr does not name the missing flag:\n%s", stderr.String())
	}
}

func TestObserveCommandRejectsBadFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"observe", "-bogus-flag"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

func TestObserveCommandFailsWithEmptyEndpoint(t *testing.T) {
	// An explicitly empty -endpoint cannot be satisfied by the fallback, so
	// the provider constructor rejects it — the message must reach the user.
	var stdout, stderr bytes.Buffer
	code := run([]string{"observe", "-endpoint", "", "-service", "svc"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "observe:") {
		t.Errorf("stderr does not carry the observe prefix:\n%s", stderr.String())
	}
}
