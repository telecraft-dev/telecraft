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
	"github.com/telecraft-dev/telecraft/pkg/licence"
)

// startWithLicence runs one server over a checkout with a licence file
// named, and waits for its first documents.
func startWithLicence(t *testing.T, root, licenceFile string) (*Server, string) {
	t.Helper()
	var lines []string
	srv, err := New(Config{
		Source:        serving.DirSource{Root: root},
		Root:          root,
		HTTPEndpoint:  "127.0.0.1:0",
		OpAMPEndpoint: "",
		FetchInterval: time.Hour,
		Sessions:      sessions(t),
		LicenceFile:   licenceFile,
		Logf: func(format string, args ...any) {
			lines = append(lines, format)
			t.Logf(format, args...)
		},
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
	return srv, "http://" + srv.HTTPAddr()
}

// An Instance with no licence file is the Standard Edition, and nothing
// about it is wrong: no warning, no banner, no start-up complaint.
func TestAnInstanceWithNoLicenceIsStandardEditionAndSaysSoQuietly(t *testing.T) {
	srv, base := start(t, estateCheckout(t))

	if got := srv.licence.Load(); got == nil || got.State != licence.Absent {
		t.Fatalf("the standing is %+v, want an absent licence", got)
	}

	var edition struct{ Edition, Statement string }
	decode(t, signedIn(t, base), base+"/api/v1/edition", &edition)
	if edition.Edition != string(licence.Standard) || edition.Statement != string(licence.Standard) {
		t.Errorf("/api/v1/edition = %+v, want the Standard Edition line and nothing else", edition)
	}
}

// The Edition is a fact about the reader's session, so it is behind the
// same gate every other read route is behind.
func TestTheEditionIsNotAnsweredSignedOut(t *testing.T) {
	_, base := start(t, estateCheckout(t))
	if _, status := get(t, http.DefaultClient, base+"/api/v1/edition"); status != http.StatusUnauthorized {
		t.Errorf("/api/v1/edition signed out = %d, want 401", status)
	}
}

// A file the operator named and this build cannot accept is loud and
// changes nothing: the server starts, the probes answer, the estate is
// served, and the Edition is the one an absent file would give.
func TestALicenceThatIsNotAcceptedStopsNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acme.licence")
	if err := os.WriteFile(path, []byte("this is not a licence\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, base := startWithLicence(t, estateCheckout(t), path)

	if got := srv.licence.Load(); got == nil || got.State != licence.Unreadable {
		t.Fatalf("the standing is %+v, want an unreadable licence", got)
	}
	if body, status := get(t, http.DefaultClient, base+"/readyz"); status != 200 || strings.TrimSpace(body) != "ready" {
		t.Errorf("/readyz = %d %q: a licence never stops an Instance serving", status, body)
	}

	client := signedIn(t, base)
	if _, status := get(t, client, base+"/api/v1/estate"); status != 200 {
		t.Errorf("the estate = %d, want 200: a licence never stops the estate being read", status)
	}

	var edition struct{ Edition, Statement string }
	decode(t, client, base+"/api/v1/edition", &edition)
	if edition.Edition != string(licence.Standard) {
		t.Errorf("edition = %q, want %q: a file that was not accepted grants what an absent one grants", edition.Edition, licence.Standard)
	}
	if edition.Statement != "Standard Edition, the licence file was not accepted" {
		t.Errorf("the statement reads %q", edition.Statement)
	}
}

// A file this process may not read is a mistake the operator can see, and
// it is still not a reason to stop serving anybody.
func TestALicenceFileThatIsNotThereStopsNothing(t *testing.T) {
	srv, base := startWithLicence(t, estateCheckout(t), filepath.Join(t.TempDir(), "nothing.licence"))
	if got := srv.licence.Load(); got == nil || got.Problem != "there is no file at that path" {
		t.Fatalf("the standing is %+v, want a licence file that is not there", got)
	}
	if body, status := get(t, http.DefaultClient, base+"/healthz"); status != 200 || strings.TrimSpace(body) != "ok" {
		t.Errorf("/healthz = %d %q", status, body)
	}
}

// What a licensed Instance reports. The keys a build ships decide what
// verifies, so the standing is placed here rather than signed: what a
// signature means is pkg/licence's to hold, and what this endpoint
// says about a standing is this package's.
func TestALicensedInstanceNamesItsLicenseeAndDates(t *testing.T) {
	srv, base := start(t, estateCheckout(t))
	client := signedIn(t, base)

	expires, err := licence.ParseDate("2027-03-03")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct {
		state     licence.State
		statement string
	}{
		{licence.InForce, "Enterprise Edition, licensed to Acme Ltd, expires 3 March 2027"},
		{licence.Expired, "Enterprise Edition, licensed to Acme Ltd, expired 3 March 2027"},
	} {
		standing := licence.Standing{
			State: want.state,
			Document: licence.Document{
				Licence:      "tc-2026-0007",
				Licensee:     "Acme Ltd",
				Expires:      expires,
				Entitlements: []licence.Entitlement{licence.ManyOrganisations},
			},
		}
		srv.licence.Store(&standing)

		var edition struct{ Edition, Statement string }
		decode(t, client, base+"/api/v1/edition", &edition)
		if edition.Edition != string(licence.Enterprise) {
			t.Errorf("%s: edition = %q, want %q", want.state, edition.Edition, licence.Enterprise)
		}
		if edition.Statement != want.statement {
			t.Errorf("%s: the statement reads %q, want %q", want.state, edition.Statement, want.statement)
		}
		// An expired licence takes no severity styling and no extra
		// words: it is a fact about the session, like the version above
		// it, and a refusal belongs on the surface that refuses.
		if strings.Contains(strings.ToLower(edition.Statement), "warning") || strings.Contains(edition.Statement, "!") {
			t.Errorf("%s: the statement argues: %q", want.state, edition.Statement)
		}
	}
}
