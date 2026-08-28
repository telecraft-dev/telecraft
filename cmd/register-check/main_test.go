package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func registerDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const acme = `name: acme
display_name: Acme Logistics
state: active
address: https://acme.telecraft.example
estate:
  kind: hosted
`

// A register that loads reports what it holds: every Organisation, where
// each is reached, and where its estate comes from.
func TestALoadedRegisterReportsWhatItHolds(t *testing.T) {
	dir := registerDir(t, map[string]string{
		"acme.yaml":   acme,
		"corvid.yaml": "name: corvid\nstate: retired\n",
	})

	var stdout, stderr bytes.Buffer
	if code := run([]string{dir}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, want 0: %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"loaded 2 Organisations, 1 active",
		"acme", "active", "https://acme.telecraft.example", "hosted estate",
		"corvid", "retired",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the report never says %q:\n%s", want, out)
		}
	}
}

// A register that does not load exits 1 and says everything that is wrong
// with it, because it is read as one reviewed change.
func TestARegisterThatDoesNotLoadExitsAndSaysWhy(t *testing.T) {
	dir := registerDir(t, map[string]string{
		"acme.yaml":   "name: acme\nstate: active\n",
		"beacon.yaml": "name: beacon\nstate: nowhere\n",
	})

	var stdout, stderr bytes.Buffer
	if code := run([]string{dir}, &stdout, &stderr); code != 1 {
		t.Fatalf("exit %d, want 1", code)
	}
	for _, want := range []string{"acme.yaml", "beacon.yaml", "address", "nowhere"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("the refusal never mentions %q:\n%s", want, stderr.String())
		}
	}
	if stdout.Len() != 0 {
		t.Errorf("a register that did not load reported anyway:\n%s", stdout.String())
	}
}

// A deployment serving nobody yet has an empty register, and reading one
// is not a failure.
func TestAnEmptyRegisterReportsNothing(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{t.TempDir()}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, want 0: %s", code, stderr.String())
	}
	if got := strings.TrimSpace(stdout.String()); got != "loaded 0 Organisations, 0 active" {
		t.Errorf("the report reads %q", got)
	}
}
