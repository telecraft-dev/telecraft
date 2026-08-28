package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/auth"
	"github.com/telecraft-dev/telecraft/internal/ownership"
)

// An estate is created for somebody: the team tree names them as an Owner,
// and the users file lets them sign in.
func TestInitCreatesAnEstateForItsFirstAdministrator(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "estate")
	var stdout, stderr bytes.Buffer
	code := runInit([]string{
		"-estate", dir,
		"-email", "robin@acme.example",
		"-name", "Robin Vale",
		"-team", "engineering",
		"-team-name", "Engineering",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init = %d: %s", code, stderr.String())
	}

	tree, err := ownership.LoadTeams(filepath.Join(dir, ownership.TeamsFile))
	if err != nil {
		t.Fatalf("the created team tree does not load: %v", err)
	}
	owner, known := tree.Owners["robin"]
	if !known {
		t.Fatalf("the tree holds %v, want the first administrator as an Owner", tree.Owners)
	}
	if owner.Team != "engineering" {
		t.Errorf("the Owner belongs to %q, want the team the estate was created with", owner.Team)
	}
	users, err := auth.LoadUsers(filepath.Join(dir, auth.UsersFile), tree)
	if err != nil {
		t.Fatalf("the created users file does not load: %v", err)
	}
	if user, ok := users.ByEmail("robin@acme.example"); !ok || user.Owner != "robin" {
		t.Errorf("the user is %+v %v, want the administrator acting as their Owner", user, ok)
	}
	// Nobody can sign in yet, and somebody is about to try.
	if !strings.Contains(stdout.String(), "Nobody can sign in") {
		t.Errorf("init does not say that the estate has no way of signing in yet:\n%s", stdout.String())
	}
}

// The created repository is a bare one holding the estate as a commit: an
// ordinary remote, which the server serves like any other.
func TestInitCreatesABareRepository(t *testing.T) {
	path := filepath.Join(t.TempDir(), "estate.git")
	var stdout, stderr bytes.Buffer
	code := runInit([]string{
		"-bare", path,
		"-email", "robin@acme.example",
		"-name", "Robin Vale",
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("init = %d: %s", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(path, "HEAD")); err != nil {
		t.Errorf("what was created is not a bare repository: %v", err)
	}
}

// The usage errors say what is missing rather than creating half an
// estate.
func TestInitRefusesWhatItCannotCreate(t *testing.T) {
	occupied := t.TempDir()
	if err := os.WriteFile(filepath.Join(occupied, "teams.yaml"), []byte("teams: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		args []string
		code int
	}{
		"neither a directory nor a repository": {
			args: []string{"-email", "robin@acme.example", "-name", "Robin Vale"},
			code: 2,
		},
		"both": {
			args: []string{"-estate", t.TempDir(), "-bare", t.TempDir(), "-email", "robin@acme.example", "-name", "Robin Vale"},
			code: 2,
		},
		"nobody to create it for": {
			args: []string{"-estate", filepath.Join(t.TempDir(), "estate")},
			code: 2,
		},
		"an address that is not one": {
			args: []string{"-estate", filepath.Join(t.TempDir(), "estate"), "-email", "robin", "-name", "Robin Vale"},
			code: 2,
		},
		"a place that already holds something": {
			args: []string{"-bare", occupied, "-email", "robin@acme.example", "-name", "Robin Vale"},
			code: 1,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := runInit(tc.args, &stdout, &stderr); code != tc.code {
				t.Errorf("init = %d, want %d: %s%s", code, tc.code, stdout.String(), stderr.String())
			}
			if stderr.Len() == 0 {
				t.Error("nothing was said about what is wrong")
			}
		})
	}
}
