package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/auth"
	"github.com/telecraft-dev/telecraft/internal/ownership"
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
}
