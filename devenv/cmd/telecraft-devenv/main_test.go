package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevenvRunWithNoArgsPrintsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("stderr does not contain usage line:\n%s", stderr.String())
	}
}

func TestDevenvRunRefusesUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"nosuchcmd"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("stderr does not contain usage line:\n%s", stderr.String())
	}
}

func TestPrepareCommandRejectsBadFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"prepare", "-bogus-flag"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

func TestPrepareCommandFailsWhenIdentityDirectoryIsAbsent(t *testing.T) {
	// A nonexistent identity directory means no collectors to compose,
	// which is a real error rather than a usage error.
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"prepare",
		"-identity", "/nonexistent/identity",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit %d, want 1\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "prepare:") {
		t.Errorf("stderr does not carry the prepare prefix:\n%s", stderr.String())
	}
}

func TestPrepareCommandFailsWhenNoIdentityFilesFound(t *testing.T) {
	// An empty identity directory means there is nothing to prepare, which
	// is caught early so the operator does not get a silent no-op.
	identity := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"prepare",
		"-identity", identity,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit %d, want 1\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "no identity files") {
		t.Errorf("stderr does not name the problem:\n%s", stderr.String())
	}
}

func TestPrepareCommandFailsWhenForeignDirectoryIsAbsent(t *testing.T) {
	identity := t.TempDir()
	if err := os.WriteFile(filepath.Join(identity, "coll-a.yaml"), []byte("base_tier: platform/gateway\noverlay: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"prepare",
		"-identity", identity,
		"-foreign", "/nonexistent/foreign",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit %d, want 1\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "prepare:") {
		t.Errorf("stderr does not carry the prepare prefix:\n%s", stderr.String())
	}
}

func TestPrepareCommandFailsWhenDriftOverlayIsAbsent(t *testing.T) {
	// When -drift names a collector, -drift-overlay must point at a file
	// that exists. A missing overlay means the drift scenario cannot be
	// composed, and composing with silent defaults would produce the wrong
	// output rather than a useful error.
	identity := t.TempDir()
	if err := os.WriteFile(filepath.Join(identity, "drifter.yaml"), []byte("base_tier: platform/gateway\noverlay: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	foreign := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"prepare",
		"-identity", identity,
		"-foreign", foreign,
		"-drift", "drifter",
		"-drift-overlay", "/nonexistent/overlay.yaml",
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("exit %d, want 1\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "prepare:") {
		t.Errorf("stderr does not carry the prepare prefix:\n%s", stderr.String())
	}
}

func TestRunLoopCommandRejectsBadFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"run", "-bogus-flag"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

func TestRunLoopCommandFailsWhenEstateIsAbsent(t *testing.T) {
	// The estate cannot be loaded from a directory that does not exist.
	// loadInputs returns a descriptive error, which the command surfaces
	// before it ever touches a telemetry backend.
	var stdout, stderr bytes.Buffer
	code := run([]string{
		"run",
		"-estate", "/nonexistent/estate",
	}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit %d, want 2\n%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "run:") {
		t.Errorf("stderr does not carry the run prefix:\n%s", stderr.String())
	}
}
