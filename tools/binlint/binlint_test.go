package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestPlantedBinary confirms that a Mach-O arm64 executable included in the
// tracked-file list is caught and named. The list is supplied directly so the
// test does not need a live git repository.
func TestPlantedBinary(t *testing.T) {
	dir := t.TempDir()

	// Mach-O little-endian 64-bit magic — the format of the binary that was
	// committed at the repository root and triggered issue #122.
	if err := os.WriteFile(filepath.Join(dir, "planted"), []byte{
		0xcf, 0xfa, 0xed, 0xfe, // magic
		0x0c, 0x00, 0x00, 0x01, // cpu type (arm64)
	}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := checkPaths(os.DirFS(dir), []string{"planted", "readme.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.executables) != 1 {
		t.Fatalf("want 1 finding, got %d: %v", len(r.executables), r.executables)
	}
	if r.executables[0] != "planted" {
		t.Errorf("want finding for %q, got %q", "planted", r.executables[0])
	}
	if r.checked != 2 {
		t.Errorf("want 2 files checked, got %d", r.checked)
	}
}

// TestCleanTree confirms that a list containing only text files produces no
// findings.
func TestCleanTree(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# clean\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := checkPaths(os.DirFS(dir), []string{"main.go", "README.md"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.executables) != 0 {
		t.Fatalf("want clean, got: %v", r.executables)
	}
}

// TestMatchesMagic pins the magic-byte detection for each supported format so
// that regressions in the byte constants are caught without a full-tree walk.
func TestMatchesMagic(t *testing.T) {
	cases := []struct {
		name  string
		bytes []byte
		want  bool
	}{
		{"ELF", []byte{0x7f, 'E', 'L', 'F'}, true},
		{"Mach-O LE32", []byte{0xce, 0xfa, 0xed, 0xfe}, true},
		{"Mach-O LE64", []byte{0xcf, 0xfa, 0xed, 0xfe}, true},
		{"Mach-O BE32", []byte{0xfe, 0xed, 0xfa, 0xce}, true},
		{"Mach-O BE64", []byte{0xfe, 0xed, 0xfa, 0xcf}, true},
		{"Mach-O fat / Java class", []byte{0xca, 0xfe, 0xba, 0xbe}, true},
		{"PE", []byte{'M', 'Z', 0x90, 0x00}, true},
		{"text file", []byte("pack"), false},
		{"empty", []byte{}, false},
		{"short", []byte{0x7f}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := matchesMagic(c.bytes); got != c.want {
				t.Errorf("matchesMagic(%#v) = %v, want %v", c.bytes, got, c.want)
			}
		})
	}
}
