package forge

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	seam "github.com/telecraft-dev/telecraft/pkg/forge"
)

// What the adapter can do is what the customer granted, read from the
// token they granted it for. A grant that covers everything the App asks
// for is the full rung and withholds nothing.
func TestTheGrantIsReadFromTheTokenItMints(t *testing.T) {
	api := &fakeAPI{t: t}
	srv := api.server(t)
	defer srv.Close()

	forge := testForge(t, srv.URL)
	if got := forge.Capabilities(); !got.Proposals || got.Withheld != "" {
		t.Errorf("before anything was minted the adapter declares %+v, want the forge's own rungs", got)
	}
	if _, err := forge.Propose(t.Context(), testChange()); err != nil {
		t.Fatal(err)
	}
	if got := forge.Capabilities(); !got.Proposals || !got.Annotations || got.Withheld != "" {
		t.Errorf("capabilities = %+v, want the full rung a complete grant allows", got)
	}
}

// A read-only grant is a declared "cannot": the rungs it does not reach
// are false, and the sentence names what is missing and where to change
// it, in the words of somebody who has to go and change it.
func TestAReadOnlyGrantDeclaresWhatItCannotDo(t *testing.T) {
	api := &fakeAPI{t: t, permissions: map[string]string{"contents": "read", "pull_requests": "read", "metadata": "read"}}
	srv := api.server(t)
	defer srv.Close()

	forge := testForge(t, srv.URL)
	// The mint succeeds and the write does not: what fails is the write,
	// and what the surfaces show afterwards is the declaration.
	_, _ = forge.Propose(t.Context(), testChange())

	got := forge.Capabilities()
	if got.Proposals || got.Annotations || got.VerifiedAttribution {
		t.Errorf("capabilities = %+v, want nothing a write needs", got)
	}
	for _, want := range []string{"write files", "open change proposals", "Grant it where Telecraft is installed"} {
		if !strings.Contains(got.Withheld, want) {
			t.Errorf("the sentence does not say %q: %q", want, got.Withheld)
		}
	}
	for _, never := range []string{"pull_requests", "ADR", "forge"} {
		if strings.Contains(got.Withheld, never) {
			t.Errorf("the sentence says %q, which is not the reader's word: %q", never, got.Withheld)
		}
	}
}

// A repository the installation does not cover declares itself
// unreachable, by name, and the remedy is at the git host rather than
// here.
func TestARepositoryOutsideTheInstallationDeclaresItself(t *testing.T) {
	api := &fakeAPI{t: t, refuseToken: true}
	srv := api.server(t)
	defer srv.Close()

	forge := testForge(t, srv.URL)
	if _, err := forge.Propose(t.Context(), testChange()); err == nil {
		t.Fatal("the proposal was opened against a repository the installation does not cover")
	}
	got := forge.Capabilities()
	if got.Proposals {
		t.Errorf("capabilities = %+v, want nothing on a repository that cannot be reached", got)
	}
	if !strings.Contains(got.Withheld, "telecraft-dev/estate-fixture") {
		t.Errorf("the sentence does not name the repository: %q", got.Withheld)
	}
}

// The token is minted for this repository and never for the whole
// installation: one installation may cover repositories that have nothing
// to do with Telecraft, and may serve two Organisations.
func TestATokenIsMintedScopedToTheRepository(t *testing.T) {
	api := &fakeAPI{t: t}
	srv := api.server(t)
	defer srv.Close()

	if _, err := testForge(t, srv.URL).Propose(t.Context(), testChange()); err != nil {
		t.Fatal(err)
	}
	mints := api.sent("POST", "/app/installations/154498501/access_tokens")
	if len(mints) != 1 {
		t.Fatalf("saw %d mints, want one", len(mints))
	}
	repos, ok := mints[0]["repositories"].([]any)
	if !ok || len(repos) != 1 || repos[0] != "estate-fixture" {
		t.Errorf("the mint asked for %v, want the one repository this adapter is for", mints[0]["repositories"])
	}
}

// A deployment that holds no key of its own is handed a token in a file
// something else keeps current. The file is read at each call, so
// rewriting it before it expires needs nothing here: no restart, no
// coordination, and no mint.
func TestATokenFromAFileIsReadAtEachCall(t *testing.T) {
	api := &fakeAPI{t: t}
	srv := api.server(t)
	defer srv.Close()

	var reads atomic.Int64
	forge, err := New(Config{
		Repo:      "https://github.com/telecraft-dev/estate-fixture",
		APIBase:   srv.URL,
		TokenFrom: func() (string, error) { reads.Add(1); return "ghs_inst_token", nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := forge.Propose(t.Context(), testChange()); err != nil {
		t.Fatal(err)
	}
	if reads.Load() < 2 {
		t.Errorf("the token file was read %d times, want one read per call", reads.Load())
	}
	if mints := api.sent("POST", "/app/installations/154498501/access_tokens"); len(mints) != 0 {
		t.Errorf("a process given a token minted %d of its own", len(mints))
	}
	// It read no grant, so it claims the forge's own rungs and lets a
	// refusal be a fault rather than a declaration.
	if got := forge.Capabilities(); !got.Proposals || got.Withheld != "" {
		t.Errorf("capabilities = %+v, want the forge's own rungs", got)
	}
}

// A process either mints its own tokens or is given them.
func TestOneCredentialOrTheOther(t *testing.T) {
	_, err := New(Config{
		Repo:           "https://github.com/telecraft-dev/estate-fixture",
		AppID:          "Iv23li25p08r8H5525ox",
		InstallationID: "154498501",
		PrivateKeyPEM:  keyPEM(t),
		TokenFrom:      func() (string, error) { return "ghs_inst_token", nil },
	})
	if err == nil {
		t.Error("an adapter was given both a key and a token file")
	}
}

// A push notification is believed only when it is genuine, and what it
// says happened is never read: the fast path means fetch now, and git is
// what says what changed.
func TestAPushNotificationIsJudgedBeforeItIsBelieved(t *testing.T) {
	const secret = "the placed secret"
	body := []byte(`{"ref":"refs/heads/main"}`)
	notifications, ok := Notifications(Config{Repo: "https://github.com/telecraft-dev/estate-fixture"})
	if !ok {
		t.Fatal("no verifier for a repository on a host with an adapter")
	}

	genuine := func(event string) seam.Notification {
		header := http.Header{}
		header.Set(signatureHeader, signatureOver(t, secret, body))
		header.Set(eventHeader, event)
		return seam.Notification{Header: header, Body: body}
	}

	if push, err := notifications.Push(genuine("push"), secret); err != nil || !push {
		t.Errorf("a genuine push was judged %v, %v", push, err)
	}
	// Genuine, and not a push. Nothing happens, and nothing is wrong.
	if push, err := notifications.Push(genuine("installation"), secret); err != nil || push {
		t.Errorf("a genuine delivery that is not a push was judged %v, %v", push, err)
	}
	if _, err := notifications.Push(genuine("push"), "another secret"); err == nil {
		t.Error("a delivery signed with something else was accepted")
	}
	if _, err := notifications.Push(seam.Notification{Header: http.Header{}, Body: body}, secret); err == nil {
		t.Error("an unsigned delivery was accepted")
	}
	if _, err := notifications.Push(genuine("push"), ""); err == nil {
		t.Error("a delivery was accepted with nothing to verify it against")
	}
	// A body that is not the one signed for is not the delivery that was
	// signed.
	replayed := genuine("push")
	replayed.Body = append(replayed.Body, ' ')
	if _, err := notifications.Push(replayed, secret); err == nil {
		t.Error("a delivery whose bytes changed after signing was accepted")
	}

	// A repository on a host no adapter here speaks for has no forge to be
	// notified by, and the endpoint's other caller is what serves it.
	if _, ok := Notifications(Config{Repo: "file:///srv/estates/acme.git"}); ok {
		t.Error("a local repository was given a forge verifier")
	}
}

// signatureOver is the signature a forge would send with these bytes.
func signatureOver(t *testing.T, secret string, body []byte) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}
