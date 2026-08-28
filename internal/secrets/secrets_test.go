package secrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A name cannot describe a path, which is what makes resolving one against
// a directory safe however it arrived.
func TestASecretNameCannotDescribeAPath(t *testing.T) {
	for name, ok := range map[string]bool{
		"staff-oidc":        true,
		"forge":             true,
		"key42":             true,
		"":                  false,
		"-leading-hyphen":   false,
		"trailing-hyphen-":  false,
		"double--hyphen":    false,
		"Upper":             false,
		"with.dot":          false,
		"with/slash":        false,
		"../../etc/passwd":  false,
		"with space":        false,
		"with_underscore":   false,
		`with\backslash`:    false,
		"with\nnewline":     false,
		"trailing/":         false,
		"nested/path/again": false,
	} {
		t.Run(name, func(t *testing.T) {
			err := CheckName(name)
			if ok && err != nil {
				t.Errorf("%q was refused: %v", name, err)
			}
			if !ok && err == nil {
				t.Errorf("%q was accepted", name)
			}
		})
	}
}

// The file's contents are the value, with the one trailing newline every
// tool that writes a file adds.
func TestTheFileContentsAreTheValue(t *testing.T) {
	dir := t.TempDir()
	place(t, dir, "staff-oidc", "s3cret\n")
	place(t, dir, "no-newline", "s3cret")
	place(t, dir, "two-newlines", "s3cret\n\n")

	for name, want := range map[string]string{
		"staff-oidc":   "s3cret",
		"no-newline":   "s3cret",
		"two-newlines": "s3cret\n",
	} {
		got, err := Dir(dir).Value(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

// A name nothing answers says what was searched for and where, because an
// operator reading it has to know what to place.
func TestAnUnplacedSecretNamesWhatWasSearched(t *testing.T) {
	dir := t.TempDir()

	_, err := Dir(dir).Value("staff-oidc")
	if err == nil {
		t.Fatal("a secret nobody placed resolved")
	}
	if !strings.Contains(err.Error(), "staff-oidc") || !strings.Contains(err.Error(), dir) {
		t.Errorf("the error names neither the secret nor the directory: %v", err)
	}

	if _, err := Dir("").Value("staff-oidc"); err == nil {
		t.Error("a secret resolved against no directory at all")
	}
}

// Where a process's own secret would sit, and nowhere at all when no
// directory is configured.
func TestThePathDefaultsUnderTheDirectory(t *testing.T) {
	if got := Dir("/run/secrets").Path("session-key"); got != filepath.Join("/run/secrets", "session-key") {
		t.Errorf("Path = %q", got)
	}
	if got := Dir("").Path("session-key"); got != "" {
		t.Errorf("Path = %q, want nothing: an unset directory has no default to give", got)
	}
}

func place(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
