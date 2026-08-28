// Package secrets resolves the material an estate names against the
// directory the deployment filled (ADR-0071).
//
// One mechanism, in every deployment shape: an estate file names a Secret,
// the deployment places a file of that name in the Secret directory, and
// the process reads it. It resolves nothing else and reaches no network to
// resolve anything, which is what keeps the air-gapped shape the same shape
// as the largest one (REQ-006).
//
// Reading happens at the point of use, so rotation is writing the file:
// nothing here holds a value, leases one, or watches for a change.
package secrets

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// name is what an estate file may call a secret: lower-case letters,
// digits and hyphens, and nothing else. A name cannot describe a path, so
// a name can never escape the directory it is resolved against.
var name = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// CheckName reports whether a name is one an estate may write, naming what
// is wrong with it when it is not.
func CheckName(secret string) error {
	if secret == "" {
		return fmt.Errorf("a secret name is missing")
	}
	if !name.MatchString(secret) {
		return fmt.Errorf("%q is not a secret name. A name is lower-case letters, digits and hyphens, so that it can never describe a path", secret)
	}
	return nil
}

// Dir is the Secret directory: whatever the deployment filled, whether
// that is a service user's directory, a compose secrets block, or a
// projected volume.
type Dir string

// Value reads the secret of that name. The file's contents are the value,
// with one trailing newline stripped, because every tool that writes one
// adds it.
//
// A name nothing answers is an error naming the name and the directory
// searched, so an operator reads what to place and where.
func (d Dir) Value(secret string) (string, error) {
	if err := CheckName(secret); err != nil {
		return "", err
	}
	if d == "" {
		return "", fmt.Errorf("the secret %q is named, and this process has no secret directory to read it from", secret)
	}
	body, err := os.ReadFile(filepath.Join(string(d), secret))
	if err != nil {
		return "", fmt.Errorf("the secret %q is named, and there is no file of that name in %s", secret, string(d))
	}
	return strings.TrimSuffix(string(body), "\n"), nil
}

// Path is where a secret of this name would be read from, for the process's
// own secrets, which the estate does not name and which take a file path
// with a default under this directory. An unset directory has no default to
// give.
func (d Dir) Path(secret string) string {
	if d == "" {
		return ""
	}
	return filepath.Join(string(d), secret)
}
