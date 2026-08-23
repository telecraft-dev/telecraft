package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The operator's file beside a git-delivered collector's artefact. There is
// no composition here and that is the assertion: the collector merges the
// two files itself, so anything this tool merged in first would be a
// difference the platform invented rather than one an operator made.

func TestTheLocalFileIsCopiedRatherThanComposed(t *testing.T) {
	authored := filepath.Join(t.TempDir(), "appliance-1.yaml")
	body := "service:\n  telemetry:\n    resource:\n      service.instance.id: appliance-1\n"
	if err := os.WriteFile(authored, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()

	path, err := writeLocalFile(authored, filepath.Join(out, "appliance-1"))
	if err != nil {
		t.Fatalf("writeLocalFile: %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(written), body) {
		t.Errorf("the authored file did not survive verbatim:\n%s", written)
	}
	if !strings.Contains(string(written), "Generated: edit that file, not this one") {
		t.Error("the copy does not say it is generated, so somebody will edit it and lose the edit on the next prepare")
	}
}

func TestTheLocalFileNamesWhatTheComposeFileMounts(t *testing.T) {
	authored := filepath.Join(t.TempDir(), "appliance-1.yaml")
	if err := os.WriteFile(authored, []byte("service: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "appliance-1")

	path, err := writeLocalFile(authored, dir)
	if err != nil {
		t.Fatalf("writeLocalFile: %v", err)
	}

	// compose.yaml mounts this path by name. A rename here without one
	// there leaves the collector refusing to start on a missing --config,
	// which reads as a broken image rather than as the rename it is.
	if filepath.Base(path) != "local.yaml" {
		t.Errorf("wrote %s, which is not the file devenv/compose.yaml mounts", filepath.Base(path))
	}
}

// Both halves of the copy fail closed. A collector given no local file, or
// given half of one, starts against a configuration nobody authored.
func TestTheLocalFileFailsClosed(t *testing.T) {
	authored := filepath.Join(t.TempDir(), "appliance-1.yaml")
	if err := os.WriteFile(authored, []byte("service: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("in the way\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct{ local, dir string }{
		"an authored file that is not there": {
			local: filepath.Join(t.TempDir(), "nowhere.yaml"),
			dir:   t.TempDir(),
		},
		"a run directory it cannot create": {
			local: authored,
			dir:   filepath.Join(blocked, "appliance-1"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			if path, err := writeLocalFile(tc.local, tc.dir); err == nil {
				t.Fatalf("wrote %s where nothing should have been written", path)
			}
		})
	}
}
