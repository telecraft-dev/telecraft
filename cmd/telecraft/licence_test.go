package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// run is one invocation of the licence printer, with its output.
func runLicencePrinter(t *testing.T, args ...string) (string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runLicence(args, &stdout, &stderr)
	if stderr.Len() > 0 {
		t.Logf("stderr: %s", stderr.String())
	}
	return stdout.String(), code
}

// With no file it prints the Standard line and stops. That is the ordinary
// case: the whole free product, and nothing wrong with it.
func TestLicencePrintsTheStandardLineWithNoFile(t *testing.T) {
	out, code := runLicencePrinter(t)
	if code != 0 {
		t.Errorf("exit = %d, want 0: it reports rather than judges", code)
	}
	if strings.TrimSpace(out) != "Standard Edition" {
		t.Errorf("it printed %q, want the Standard line alone", out)
	}
}

// A file that was not accepted is reported, named, and still not a
// failure of the command.
func TestLicenceReportsAFileThatWasNotAccepted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acme.licence")
	if err := os.WriteFile(path, []byte("nothing signed anything here\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, code := runLicencePrinter(t, "-licence-file", path)
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	for _, want := range []string{"Standard Edition, the licence file was not accepted", "problem", path} {
		if !strings.Contains(out, want) {
			t.Errorf("the output does not carry %q:\n%s", want, out)
		}
	}
}

// The environment configures it like every other flag, so a container
// names the file without an entrypoint that rewrites arguments.
func TestLicenceReadsItsFileFromTheEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acme.licence")
	t.Setenv("TELECRAFT_LICENCE_FILE", path)

	out, _ := runLicencePrinter(t)
	if !strings.Contains(out, path) {
		t.Errorf("the environment named %s and the output does not:\n%s", path, out)
	}
}

// It never sells. The surfaces report, and a console that starts selling
// stops being an instrument.
func TestLicenceReportsAndNeverSells(t *testing.T) {
	out, _ := runLicencePrinter(t)
	for _, forbidden := range []string{"trial", "upgrade", "http", "contact", "sales", "pricing"} {
		if strings.Contains(strings.ToLower(out), forbidden) {
			t.Errorf("the output carries %q:\n%s", forbidden, out)
		}
	}
}

// The command is reachable by name, and an unknown one still is not.
func TestLicenceIsACommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"licence"}, &stdout, &stderr); code != 0 {
		t.Errorf("telecraft licence = %d, want 0: %s", code, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "Standard Edition" {
		t.Errorf("telecraft licence printed %q", stdout.String())
	}
}
