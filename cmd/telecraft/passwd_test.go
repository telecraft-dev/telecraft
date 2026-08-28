package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/pkg/auth"
	"github.com/telecraft-dev/telecraft/pkg/ownership"
)

// The printed hash is a working users.yaml credential: write it into the
// file, load through the seam, sign in with basic auth.
func TestPasswdHashSignsIn(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runPasswd(nil, strings.NewReader("correct horse battery\n"), &stdout, &stderr); code != 0 {
		t.Fatalf("passwd = %d: %s", code, stderr.String())
	}
	hash := strings.TrimSpace(stdout.String())

	dir := t.TempDir()
	teams := "teams:\n  - id: data-flow\n    name: Data flow\n    owners: [gateway-owners]\n"
	users := fmt.Sprintf("users:\n  - email: jo@example.com\n    name: Jo Author\n    owner: gateway-owners\n    password: %q\n", hash)
	if err := os.WriteFile(filepath.Join(dir, "teams.yaml"), []byte(teams), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, auth.UsersFile), []byte(users), 0o600); err != nil {
		t.Fatal(err)
	}

	tree, err := ownership.LoadTeams(filepath.Join(dir, "teams.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := auth.LoadUsers(filepath.Join(dir, auth.UsersFile), tree)
	if err != nil {
		t.Fatal(err)
	}
	id, err := auth.Basic{Users: loaded}.Authenticate(context.Background(), "jo@example.com", "correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if id.Email != "jo@example.com" {
		t.Fatalf("Authenticate returned %+v", id)
	}
}

func TestPasswdRefusesASecretArgument(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runPasswd([]string{"hunter2"}, strings.NewReader(""), &stdout, &stderr); code != 2 {
		t.Fatalf("passwd with an argument = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "stdin") {
		t.Fatalf("stderr %q does not point at stdin", stderr.String())
	}
}

func TestPasswdRefusesAnEmptySecret(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runPasswd(nil, strings.NewReader("\n"), &stdout, &stderr); code != 2 {
		t.Fatalf("passwd with an empty secret = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "passwd: ") {
		t.Errorf("stderr does not name the subcommand that refused:\n%s", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("a refused secret still printed a hash:\n%s", stdout.String())
	}
}

func TestPasswdRejectsAFlagThatDoesNotExist(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := runPasswd([]string{"-cost", "12"}, strings.NewReader(""), &stdout, &stderr)

	if code != 2 {
		t.Fatalf("passwd with an unknown flag = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined: -cost") {
		t.Errorf("stderr does not name the flag that does not exist:\n%s", stderr.String())
	}
}

// failingReader is a stdin that breaks part-way, which is what a closed
// pipe looks like from here.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("the pipe closed") }

// A secret that could not be read is exit 1 with the cause: the command
// neither hashed nothing nor printed a hash of a truncated secret.
func TestPasswdReportsAStdinItCannotRead(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := runPasswd(nil, failingReader{}, &stdout, &stderr)

	if code != 1 {
		t.Fatalf("passwd over a broken stdin = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "passwd: the pipe closed") {
		t.Errorf("stderr does not carry the read error:\n%s", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("a secret that was never read still printed a hash:\n%s", stdout.String())
	}
}
