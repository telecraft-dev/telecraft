package catalogue

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustImport(t *testing.T, src Source) *Catalogue {
	t.Helper()
	cat, _, err := Import(snapshotDir, src)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

// Two imports of the same tree at the same tag must encode to identical
// bytes — determinism is what makes idempotency testable at all.
func TestImportIsByteDeterministic(t *testing.T) {
	a, err := mustImport(t, snapshotSource).Encode()
	if err != nil {
		t.Fatal(err)
	}
	b, err := mustImport(t, snapshotSource).Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("two imports of the same tree encoded differently")
	}
}

// Acceptance criterion 3, first half: re-importing the same tag writes
// nothing — the artefact on disk already IS this import, byte for byte.
func TestReimportingTheSameTagIsIdempotent(t *testing.T) {
	dir := t.TempDir()

	path1, changed, err := mustImport(t, snapshotSource).Write(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("first write reported nothing changed")
	}
	first, err := os.ReadFile(path1)
	if err != nil {
		t.Fatal(err)
	}

	path2, changed, err := mustImport(t, snapshotSource).Write(dir)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("re-importing the same tag rewrote the artefact")
	}
	if path2 != path1 {
		t.Errorf("re-import wrote to %s, want %s", path2, path1)
	}
	second, err := os.ReadFile(path1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("artefact bytes changed across an idempotent re-import")
	}
}

// Acceptance criterion 3, second half: a new tag is a new atomic Catalogue
// version written beside the old one. Installed catalogues are retained,
// never replaced (ADR-0020 §9).
func TestANewTagIsANewVersionBesideTheOld(t *testing.T) {
	dir := t.TempDir()

	if _, _, err := mustImport(t, snapshotSource).Write(dir); err != nil {
		t.Fatal(err)
	}
	next := snapshotSource
	next.Ref, next.Commit = "v0.159.0", "0000000000000000000000000000000000000001"
	if _, _, err := mustImport(t, next).Write(dir); err != nil {
		t.Fatal(err)
	}

	old, err := Load(filepath.Join(dir, ArtefactName("v0.158.0")))
	if err != nil {
		t.Fatalf("the old version is gone or unreadable: %v", err)
	}
	fresh, err := Load(filepath.Join(dir, ArtefactName("v0.159.0")))
	if err != nil {
		t.Fatalf("the new version did not land: %v", err)
	}
	if old.Version() != "v0.158.0" || fresh.Version() != "v0.159.0" {
		t.Errorf("versions = %q and %q — each artefact must carry its own tag", old.Version(), fresh.Version())
	}
}

// A release tag that would escape the artefact directory cannot name a file.
func TestRefWithAPathSeparatorCannotNameAnArtefact(t *testing.T) {
	cat := mustImport(t, snapshotSource)
	cat.Source.Ref = "../escape"
	if _, _, err := cat.Write(t.TempDir()); err == nil {
		t.Fatal("expected an error for a ref holding a path separator")
	}
}

// What Write stores, Load returns — the artefact is the transport (bundled,
// downloaded, or carried across an air gap; ADR-0020 §5), so the round trip
// must be lossless.
func TestWriteThenLoadRoundTrips(t *testing.T) {
	dir := t.TempDir()
	cat := mustImport(t, snapshotSource)
	path, _, err := cat.Write(dir)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := cat.Encode()
	got, err := loaded.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Fatal("the loaded catalogue does not re-encode to the stored bytes")
	}
	if _, ok := loaded.Lookup(Connector, "spanmetrics"); !ok {
		t.Error("alias lookups do not survive the round trip")
	}
}

// mutate writes a hand-altered copy of a valid artefact and returns its path.
func mutate(t *testing.T, alter func(string) string) string {
	t.Helper()
	dir := t.TempDir()
	path, _, err := mustImport(t, snapshotSource).Write(dir)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "altered.json")
	if err := os.WriteFile(out, []byte(alter(string(raw))), 0o644); err != nil {
		t.Fatal(err)
	}
	return out
}

// Loading fails closed on anything the artefact contract does not allow: a
// travelling artefact that was tampered with, truncated, or written by a
// future format must be refused, never partially accepted. Table-driven over
// hand-altered copies of a valid artefact.
func TestLoadFailsClosed(t *testing.T) {
	cases := map[string]struct {
		alter func(string) string
		want  string
	}{
		"unknown field": {
			func(s string) string { return strings.Replace(s, `"format_version"`, `"invented_field": 1, "format_version"`, 1) },
			"invented_field",
		},
		"future format version": {
			func(s string) string { return strings.Replace(s, `"format_version": 1`, `"format_version": 2`, 1) },
			"format_version",
		},
		"missing release tag": {
			func(s string) string { return strings.Replace(s, `"ref": "v0.158.0"`, `"ref": ""`, 1) },
			"source.ref",
		},
		"truncated file": {
			func(s string) string { return s[:len(s)/2] },
			"",
		},
		"duplicated component": {
			func(s string) string {
				// Duplicate the whole components array into itself, so every
				// (class, type) key appears twice.
				return strings.Replace(s, `"components": [`, `"components": [`+"\n"+componentEntries(t, s)+",", 1)
			},
			"primary key",
		},
		"trailing data": {
			func(s string) string { return s + "{}\n" },
			"trailing",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := mutate(t, tc.alter)
			cat, err := Load(path)
			if err == nil {
				t.Fatal("expected the load to fail closed")
			}
			if cat != nil {
				t.Fatal("Load failed but returned a catalogue")
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error does not mention %q: %v", tc.want, err)
			}
		})
	}
}

// componentEntries extracts the body of the components array from an encoded
// artefact, for splicing into tampering tests.
func componentEntries(t *testing.T, artefact string) string {
	t.Helper()
	start := strings.Index(artefact, `"components": [`)
	end := strings.LastIndex(artefact, "]")
	if start < 0 || end < 0 {
		t.Fatal("could not locate the components array")
	}
	return artefact[start+len(`"components": [`) : end]
}

func TestLoadOfAMissingFileIsAnError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("expected an error for a missing artefact")
	}
}
