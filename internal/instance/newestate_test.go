package instance

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/serving"
	"github.com/telecraft-dev/telecraft/pkg/auth"
	"github.com/telecraft-dev/telecraft/pkg/ownership"
	"github.com/telecraft-dev/telecraft/pkg/seed"
)

// What a new estate is, and what the Instance does with one (ADR-0086).
//
// The estate under test is the one `telecraft init` writes, written by the
// same package the command writes it with, so a seed that grows a file
// changes this test rather than passing beside it.

// The whole of the first five minutes: create an estate, start over it,
// sign in, and read it. Every document answers, and answers empty.
func TestAnEstateWithNoTierIsServedRatherThanRefused(t *testing.T) {
	_, base := start(t, newEstate(t))
	client := signedInAs(t, base, seedEmail)

	// The three documents the console's shell is built from. Each is a
	// reading of an estate with one team and no objects, never a refusal.
	var estate struct {
		Environments []string `json:"environments"`
		Teams        struct {
			ID string `json:"id"`
		} `json:"teams"`
		Cards []any `json:"cards"`
	}
	decode(t, client, base+"/api/v1/estate", &estate)
	if estate.Teams.ID != seedTeam {
		t.Errorf("the estate document names team %q, want the one the estate was created with (%q)", estate.Teams.ID, seedTeam)
	}
	if len(estate.Cards) != 0 {
		t.Errorf("a new estate reports %d cards, want none", len(estate.Cards))
	}
	if len(estate.Environments) != 0 {
		t.Errorf("a new estate reports environments %v, want none", estate.Environments)
	}

	var objects []struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	}
	decode(t, client, base+"/api/v1/objects", &objects)
	if len(objects) != 1 || objects[0].Kind != "team" || objects[0].ID != seedTeam {
		t.Errorf("the object index holds %+v, want the one team the estate was created with", objects)
	}

	var topology struct {
		Tiers   []any `json:"tiers"`
		Sources []any `json:"sources"`
		Hops    []any `json:"hops"`
		Paths   []any `json:"paths"`
	}
	decode(t, client, base+"/api/v1/topology", &topology)
	if len(topology.Tiers) != 0 || len(topology.Hops) != 0 || len(topology.Paths) != 0 {
		t.Errorf("a new estate reports a topology with something in it: %+v", topology)
	}
}

// Every read endpoint, not only the three the shell needs. A projection
// reaching for a map or a version an empty estate does not hold would
// panic, and the 500 that follows would be the same darkness as the 503.
func TestEveryDocumentAnswersOverANewEstate(t *testing.T) {
	_, base := start(t, newEstate(t))
	client := signedInAs(t, base, seedEmail)

	for _, path := range []string{
		"/api/v1/objects",
		"/api/v1/estate",
		"/api/v1/drawer",
		"/api/v1/collectors",
		"/api/v1/topology",
		"/api/v1/rollouts",
		"/api/v1/blueprints",
		"/api/v1/catalogue",
		"/api/v1/catalogue/versions",
		"/api/v1/catalogue/entries",
		"/api/v1/activations",
		"/api/v1/governance",
		"/api/v1/endorsements",
	} {
		if body, status := get(t, client, base+path); status != http.StatusOK {
			t.Errorf("%s = %d, want 200: %s", path, status, body)
		}
	}
}

// The boundary of the decision, and the reason newness is not read off the
// rendered tree. An estate that authors Tiers and has lost its rendered
// tree is not new, it is broken, and nothing about it is treated as empty:
// the process refuses to start over it, exactly as it did before.
func TestAnEstateThatLostItsRenderedTreeIsStillRefused(t *testing.T) {
	root := estateCheckout(t)
	if err := os.RemoveAll(filepath.Join(root, "rendered")); err != nil {
		t.Fatal(err)
	}

	err := tryStart(t, root)
	if err == nil {
		t.Fatal("an estate with Tiers and no rendered tree started anyway")
	}
	if !strings.Contains(err.Error(), "rendered") {
		t.Errorf("the refusal does not name the tree that is missing: %v", err)
	}
}

// The same boundary one input further in: an estate with Tiers and no
// requirements library is judged against nothing, which is the refusal
// devenv asserts on and this decision leaves exactly where it was.
func TestAnEstateWithTiersAndNoLibraryIsStillRefused(t *testing.T) {
	root := estateCheckout(t)
	if err := os.RemoveAll(filepath.Join(root, "requirements")); err != nil {
		t.Fatal(err)
	}

	srv := startRefusing(t, root, nil)
	base := "http://" + srv.HTTPAddr()
	client := signedIn(t, base)

	if _, status := get(t, client, base+"/api/v1/estate"); status != http.StatusServiceUnavailable {
		t.Fatalf("/api/v1/estate over an estate with no requirements library = %d, want 503", status)
	}
}

// tryStart starts a server over a checkout and hands back what it said,
// for the estates the product is supposed to refuse.
func tryStart(t *testing.T, root string) error {
	t.Helper()
	srv, err := New(Config{
		Source:        serving.DirSource{Root: root},
		Root:          root,
		HTTPEndpoint:  "127.0.0.1:0",
		OpAMPEndpoint: "",
		FetchInterval: time.Hour,
		Sessions:      sessions(t),
		Logf:          t.Logf,
	})
	if err != nil {
		return err
	}
	if err := srv.Start(context.Background()); err != nil {
		return err
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	})
	return nil
}

// The estate `telecraft init` writes, and the one password that makes it
// reachable. The files come from the seed itself rather than from a
// fixture, so this is the estate the command actually creates.
const (
	seedEmail = "a@example.com"
	seedTeam  = "engineering"
)

func newEstate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	created := seed.Estate{
		Team: ownership.TeamID(seedTeam),
		Administrator: seed.Administrator{
			Email: seedEmail,
			Name:  "A",
			Owner: ownership.OwnerID("a"),
		},
	}
	files, err := created.Files()
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.Write(root, files); err != nil {
		t.Fatal(err)
	}

	// A way in. `telecraft passwd` prints this hash and the reader pastes
	// it into users.yaml, which is the step the guide describes.
	hash, err := auth.HashSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	users := "users:\n" +
		"  - email: " + seedEmail + "\n" +
		"    name: A\n" +
		"    owner: a\n" +
		"    password: " + hash + "\n"
	if err := os.WriteFile(filepath.Join(root, auth.UsersFile), []byte(users), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}
