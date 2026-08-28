package seed

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/pkg/auth"
	"github.com/telecraft-dev/telecraft/pkg/ownership"
)

// anyValue answers every secret name, which is what a deployment that
// placed the files the estate names looks like from here.
type anyValue struct{}

func (anyValue) Value(string) (string, error) { return "the value", nil }

func newEstate() Estate {
	return Estate{
		Team:     "engineering",
		TeamName: "Engineering",
		Administrator: Administrator{
			Email: "Robin@acme.example",
			Name:  "Robin Vale",
		},
	}
}

// The whole of what a seed is: one team, one person, and how they sign in.
// The Owner they act as follows from their address where nobody named one.
func TestASeedIsOneTeamOnePersonAndHowTheySignIn(t *testing.T) {
	estate := newEstate()
	estate.SignIn = []Provider{{Preset: "google", ClientID: "telecraft", Secret: "google-client-secret"}}

	files, err := estate.Files()
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	for _, name := range []string{teamsFile, usersFile, providersFile} {
		if _, ok := files[name]; !ok {
			t.Errorf("the seed writes no %s", name)
		}
	}
	if len(files) != 3 {
		t.Errorf("the seed writes %d files, want the three an estate is created with", len(files))
	}
	if got := string(files[usersFile]); !strings.Contains(got, "owner: robin") {
		t.Errorf("the user acts as no Owner taken from their address:\n%s", got)
	}
	if got := string(files[usersFile]); !strings.Contains(got, "email: robin@acme.example") {
		t.Errorf("the address is not the one a provider asserts:\n%s", got)
	}
	if got := string(files[providersFile]); strings.Contains(got, "the value") {
		t.Errorf("the seed wrote a secret value into the estate:\n%s", got)
	}
}

// An estate that declares no provider is the bootstrap shape: no auth.yaml
// at all, which offers basic auth against the hashes users.yaml carries.
func TestASeedWithNoProviderWritesNoProvidersFile(t *testing.T) {
	files, err := newEstate().Files()
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	if _, ok := files[providersFile]; ok {
		t.Error("an estate that declares no provider was given one")
	}
}

// What the seed writes is what the product reads. The check is the
// product's own loaders over the files the seed produced, so a change to
// either that parts them fails here rather than at somebody's first sign
// in.
func TestASeededEstateIsOneTheProductLoads(t *testing.T) {
	estate := newEstate()
	estate.SignIn = []Provider{
		{Preset: "google", ClientID: "telecraft", Secret: "google-client-secret"},
		{Preset: "entra", Directory: "acme.example", ClientID: "telecraft", Secret: "entra-client-secret"},
		{Issuer: "https://issuer.example", Name: "Staff", ClientID: "telecraft", Secret: "staff-oidc"},
	}
	files, err := estate.Files()
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}

	dir := t.TempDir()
	if err := Write(dir, files); err != nil {
		t.Fatalf("writing: %v", err)
	}
	tree, err := ownership.LoadTeams(filepath.Join(dir, teamsFile))
	if err != nil {
		t.Fatalf("the seeded team tree does not load: %v", err)
	}
	users, err := auth.LoadUsers(filepath.Join(dir, usersFile), tree)
	if err != nil {
		t.Fatalf("the seeded users do not load: %v", err)
	}
	if _, ok := users.ByEmail("robin@acme.example"); !ok {
		t.Error("the person the estate was created for cannot be found by the address their provider asserts")
	}
	signIn, err := auth.LoadSignIn(filepath.Join(dir, providersFile), tree, users, anyValue{}, auth.WithoutBasicAuth())
	if err != nil {
		t.Fatalf("the seeded providers do not load: %v", err)
	}
	if len(signIn.Providers) != 3 {
		t.Fatalf("the estate offers %d ways of signing in, want the three it was created with", len(signIn.Providers))
	}
	if got := signIn.Providers[0].Name(); got != "Google" {
		t.Errorf("the first provider is shown as %q, want the name the preset carries", got)
	}
}

// A seed the product would refuse is refused here, before a repository is
// created around it, and every problem is named at once.
func TestASeedTheProductWouldRefuseIsRefused(t *testing.T) {
	for name, tc := range map[string]struct {
		estate Estate
		want   string
	}{
		"no team": {
			estate: Estate{Administrator: Administrator{Email: "robin@acme.example", Name: "Robin Vale"}},
			want:   "names no team",
		},
		"no address": {
			estate: Estate{Team: "engineering", Administrator: Administrator{Name: "Robin Vale"}},
			want:   "not an email address",
		},
		"no name": {
			estate: Estate{Team: "engineering", Administrator: Administrator{Email: "robin@acme.example"}},
			want:   "has no name",
		},
		"a provider with no secret": {
			estate: func() Estate {
				e := newEstate()
				e.SignIn = []Provider{{Preset: "google", ClientID: "telecraft"}}
				return e
			}(),
			want: "names no secret",
		},
		"a provider that is neither": {
			estate: func() Estate {
				e := newEstate()
				e.SignIn = []Provider{{ClientID: "telecraft", Secret: "staff-oidc"}}
				return e
			}(),
			want: "neither a preset nor an issuer",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := tc.estate.Files(); err == nil {
				t.Fatal("the estate was rendered")
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("the refusal does not say %q: %v", tc.want, err)
			}
		})
	}
}

// A created estate is a bare repository holding one commit: an ordinary
// remote that clones, which is the whole of what a Hosted repository is.
func TestARepositoryIsCreatedWithOneCommitAndClones(t *testing.T) {
	ctx := context.Background()
	files, err := newEstate().Files()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "estate.git")
	author := Author{Name: "Telecraft", Email: "telecraft@acme.example"}
	if err := Repository(ctx, path, files, author, "Create the estate"); err != nil {
		t.Fatalf("creating: %v", err)
	}

	clone := filepath.Join(t.TempDir(), "clone")
	if _, err := git(ctx, "", "clone", "--quiet", path, clone); err != nil {
		t.Fatalf("cloning what was created: %v", err)
	}
	for name := range files {
		if _, err := os.Stat(filepath.Join(clone, name)); err != nil {
			t.Errorf("the clone holds no %s: %v", name, err)
		}
	}
	branch, err := git(ctx, clone, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if branch != DefaultBranch {
		t.Errorf("the clone is on %q, want %q", branch, DefaultBranch)
	}
	log, err := git(ctx, clone, "log", "--format=%an <%ae>%n%s")
	if err != nil {
		t.Fatal(err)
	}
	if want := "Telecraft <telecraft@acme.example>\nCreate the estate"; log != want {
		t.Errorf("the history is %q, want the one commit %q", log, want)
	}

	// Creating an estate happens once. A second run over the same path
	// would be either a no-op somebody misread or a repository somebody
	// lost.
	if err := Repository(ctx, path, files, author, "Create the estate"); err == nil {
		t.Error("a repository was created over one that was already there")
	}
}
