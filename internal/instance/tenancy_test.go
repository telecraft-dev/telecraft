package instance

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/auth"
	"github.com/telecraft-dev/telecraft/internal/serving"
)

// A deployment that serves several Organisations runs one of these
// processes for each, so the isolation between two of them is the
// isolation between two processes over two estates. This holds the part
// that a rule could otherwise be relied on for: a session is signed with
// one Instance's key, so an Instance that did not issue one cannot verify
// it, and nothing has to be remembered on any route for that to be true
// (ADR-0069 §2, §6).
func TestAnIdentityInOneOrganisationReadsNothingOfAnother(t *testing.T) {
	acme, acmeUser := organisation(t, "operator@acme.example")
	beacon, beaconUser := organisation(t, "operator@beacon.example")

	client := signIn(t, acme, acmeUser)

	// The session really is presented to the other Instance: a cookie is
	// scoped to a host and not to a port, so this is the same browser
	// carrying one Organisation's session to another's address.
	other, err := url.Parse(beacon)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.Jar.Cookies(other)) == 0 {
		t.Fatal("the other Organisation was never shown a session, so nothing below is a refusal")
	}

	// Every read route of the other Organisation, with a session the
	// other Instance never issued.
	for _, path := range []string{
		"/api/v1/me", "/api/v1/estate", "/api/v1/objects", "/api/v1/collectors",
		"/api/v1/topology", "/api/v1/rollouts", "/api/v1/blueprints",
		"/api/v1/catalogue", "/api/v1/catalogue/versions", "/api/v1/catalogue/entries",
		"/api/v1/activations", "/api/v1/governance", "/api/v1/endorsements",
		"/api/v1/edition", "/api/v1/drawer?tier=data-flow/gateway",
	} {
		body, status := get(t, client, beacon+path)
		if status != http.StatusUnauthorized {
			t.Errorf("%s with another Organisation's session = %d, want 401:\n%s", path, status, body)
		}
	}

	// The two user sets are two: a credential one Organisation holds is
	// not a credential in the other.
	if status := login(t, &http.Client{}, beacon, acmeUser); status != http.StatusUnauthorized {
		t.Errorf("signing in to one Organisation with another's credential = %d, want 401", status)
	}

	// And the Organisation that does hold the credential is unaffected:
	// this proves the refusals above are the boundary rather than a
	// broken fixture.
	own := signIn(t, beacon, beaconUser)
	if body, status := get(t, own, beacon+"/api/v1/estate"); status != http.StatusOK {
		t.Errorf("an Organisation's own session = %d, want 200:\n%s", status, body)
	}
}

// organisation starts one Instance over an estate of its own, with its own
// users and its own session key, which is what one Organisation is.
func organisation(t *testing.T, user string) (base, id string) {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(filepath.Join("..", "console", "testdata", "estate"))); err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	users := "users:\n" +
		"  - email: " + user + "\n" +
		"    name: The operator\n" +
		"    owner: gateway-owners\n" +
		"    password: " + hash + "\n"
	if err := os.WriteFile(filepath.Join(root, auth.UsersFile), []byte(users), 0o644); err != nil {
		t.Fatal(err)
	}

	// The session key is per Organisation, so a session cannot travel
	// between two of them.
	key, err := auth.NewSessions([]byte("the session key of "+user+", of at least 32 bytes"), 0)
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(Config{
		Source:        serving.DirSource{Root: root},
		Root:          root,
		HTTPEndpoint:  "127.0.0.1:0",
		OpAMPEndpoint: "",
		FetchInterval: time.Hour,
		Sessions:      key,
		Logf:          t.Logf,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	})

	base = "http://" + srv.HTTPAddr()
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, status := get(t, http.DefaultClient, base+"/api/v1/estate"); status == http.StatusUnauthorized {
			return base, user
		}
		if time.Now().After(deadline) {
			t.Fatal("the first documents never landed")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func login(t *testing.T, client *http.Client, base, user string) int {
	t.Helper()
	body := `{"provider":"basic","username":"` + user + `","secret":"` + secret + `"}`
	res, err := client.Post(base+"/api/v1/auth/login", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	return res.StatusCode
}

func signIn(t *testing.T, base, user string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	if status := login(t, client, base, user); status != http.StatusOK {
		t.Fatalf("signing in to %s as %s = %d", base, user, status)
	}
	return client
}
