package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// The planted binaries are written at run time rather than kept under
// testdata/, which is the one fixture this check cannot have: a committed
// executable is exactly what it exists to reject, so the tool would fail on
// its own test data. A header is all the detection reads, so a header is
// all a fixture needs to be.

// machO64 is the opening of a 64-bit Mach-O executable, the format of the
// binary this check was written for (issue #122).
func machO64() []byte {
	h := make([]byte, 32)
	binary.BigEndian.PutUint32(h, 0xfeedfacf)
	return h
}

// machOFat is a two-architecture Mach-O binary: the magic, the count, and
// an architecture table whose first entry is ARM64.
func machOFat() []byte {
	h := make([]byte, 48)
	binary.BigEndian.PutUint32(h[0:], 0xcafebabe)
	binary.BigEndian.PutUint32(h[4:], 2)
	binary.BigEndian.PutUint32(h[8:], 0x0100000c)
	return h
}

// elf64 is the opening of an ELF executable: the magic, then 64-bit,
// little-endian, version 1.
func elf64() []byte {
	h := make([]byte, 64)
	copy(h, []byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0})
	return h
}

// pe is a Windows executable: the DOS stub's "MZ", the offset it carries at
// 0x3c, and the signature that offset points at.
func pe() []byte {
	const at = 0x80
	h := make([]byte, at+4)
	copy(h, []byte{'M', 'Z'})
	binary.LittleEndian.PutUint32(h[0x3c:], at)
	copy(h[at:], []byte{'P', 'E', 0, 0})
	return h
}

// TestPlantedExecutablesAreFound is the acceptance case: a compiled
// executable in the tree fails the check, and the finding names the file so
// the reader knows what to remove.
func TestPlantedExecutablesAreFound(t *testing.T) {
	cases := []struct {
		path   string
		body   []byte
		format string
	}{
		{"vendorlint", machO64(), "Mach-O"},
		{"tools/universal", machOFat(), "Mach-O"},
		{"cmd/telecraft", elf64(), "ELF"},
		{"telecraft.exe", pe(), "PE"},
	}

	root := t.TempDir()
	paths := make([]string, 0, len(cases))
	for _, c := range cases {
		plant(t, root, c.path, c.body)
		paths = append(paths, c.path)
	}

	result, err := scan(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != len(cases) {
		t.Errorf("scanned %d files, want %d", result.Scanned, len(cases))
	}
	if len(result.Findings) != len(cases) {
		t.Fatalf("got %d findings, want %d: %v", len(result.Findings), len(cases), result.Findings)
	}
	for i, c := range cases {
		got := result.Findings[i]
		if got.Path != c.path || got.Format != c.format {
			t.Errorf("finding %d: got %s as %s, want %s as %s", i, got.Path, got.Format, c.path, c.format)
		}
	}
}

// TestOrdinaryFilesPass covers the other half of the bargain. A check that
// flags a fixture, an image or a font is a check somebody switches off, so
// the two ambiguous magics are pinned here against the formats that share
// their opening bytes.
func TestOrdinaryFilesPass(t *testing.T) {
	javaClass := make([]byte, 32)
	binary.BigEndian.PutUint32(javaClass[0:], 0xcafebabe)
	binary.BigEndian.PutUint32(javaClass[4:], 52) // minor 0, major 52

	dosStubOnly := make([]byte, 128)
	copy(dosStubOnly, []byte{'M', 'Z'})
	binary.LittleEndian.PutUint32(dosStubOnly[0x3c:], 0x80) // pointing at nothing

	cases := []struct {
		path string
		body []byte
	}{
		{"lint.go", []byte("package main\n\nfunc main() {}\n")},
		{"README.md", []byte("# Telecraft\n\nA compiler for observability.\n")},
		{"logo.png", append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, make([]byte, 64)...)},
		{"shot.jpg", append([]byte{0xff, 0xd8, 0xff, 0xe0}, make([]byte, 64)...)},
		{"mark.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)},
		{"body.woff2", append([]byte{'w', 'O', 'F', '2'}, make([]byte, 64)...)},
		{"Estate.class", javaClass},
		{"notes.txt", []byte("MZ is a pair of letters, and this file is prose.\n")},
		{"stub.bin", dosStubOnly},
		{"empty", nil},
		{"run.sh", []byte("#!/bin/sh\nexec go run ./tools/binlint\n")},
	}

	root := t.TempDir()
	paths := make([]string, 0, len(cases))
	for _, c := range cases {
		plant(t, root, c.path, c.body)
		paths = append(paths, c.path)
	}

	result, err := scan(root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Errorf("ordinary files flagged: %v", result.Findings)
	}
	if result.Scanned != len(cases) {
		t.Errorf("scanned %d files, want %d", result.Scanned, len(cases))
	}
}

// TestCleanTree runs the check the way CI runs it, over this repository's
// own tracked files. It is the regression test for issue #122: it fails
// again the moment another build artefact is committed.
func TestCleanTree(t *testing.T) {
	result, err := Run("../..")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Errorf("compiled executables are tracked: %v", result.Findings)
	}
	if result.Scanned == 0 {
		t.Fatal("scanned no files: the tracked-file listing is not reaching the repository")
	}
}

func plant(t *testing.T, root, path string, body []byte) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, body, 0o755); err != nil {
		t.Fatal(err)
	}
}
