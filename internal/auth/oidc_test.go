package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// fakeIdP is a local OpenID Connect issuer: discovery, a token endpoint
// and a JWKS over one RSA key. It stands in for the self-hosted identity
// provider of an air-gapped deployment — everything the OIDC provider
// talks to lives on the loopback interface.
type fakeIdP struct {
	key    *rsa.PrivateKey
	srv    *httptest.Server
	issuer string

	// claims is what the next token exchange asserts; tests mutate it.
	claims map[string]any

	// tokenStatus lets a test fail the exchange.
	tokenStatus int

	// wantChallenge, when non-empty, makes the token endpoint perform S256
	// verification: it rejects any code_verifier whose SHA256 does not match.
	wantChallenge string
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	idp := &fakeIdP{key: key, tokenStatus: http.StatusOK}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{
			"issuer":                 idp.issuer,
			"authorization_endpoint": idp.issuer + "/authorize",
			"token_endpoint":         idp.issuer + "/token",
			"jwks_uri":               idp.issuer + "/jwks",
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if idp.tokenStatus != http.StatusOK {
			writeJSON(w, idp.tokenStatus, map[string]string{"error": "server_error"})
			return
		}
		if err := r.ParseForm(); err != nil || r.PostForm.Get("code") == "" || r.PostForm.Get("client_secret") != "s3cret" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
			return
		}
		if idp.wantChallenge != "" {
			got := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
			if base64.RawURLEncoding.EncodeToString(got[:]) != idp.wantChallenge {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]string{"id_token": idp.signJWT(t, idp.claims)})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		pub := &idp.key.PublicKey
		writeJSON(w, http.StatusOK, map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA",
				"kid": "test-key",
				"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1}),
			}},
		})
	})

	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)
	idp.issuer = idp.srv.URL
	return idp
}

func (idp *fakeIdP) signJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "kid": "test-key", "typ": "JWT"})
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	signing := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, idp.key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// goodClaims asserts jo@example.com for one sign-in attempt bound to state.
func (idp *fakeIdP) goodClaims(clientID, state string) map[string]any {
	return map[string]any{
		"iss":   idp.issuer,
		"sub":   "idp-subject-1",
		"aud":   clientID,
		"exp":   time.Now().Add(time.Hour).Unix(),
		"nonce": nonceFrom(state),
		"name":  "Jo Author",
		"email": "jo@example.com",
	}
}

func (idp *fakeIdP) provider() *OIDC {
	return &OIDC{Issuer: idp.issuer, ClientID: "telecraft-console", ClientSecret: "s3cret"}
}

const (
	// testVerifier is a fixed PKCE code verifier for tests that do not
	// exercise the verifier path itself — 43 base64url characters, the
	// RFC 7636 minimum.
	testVerifier = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	testCallback = "http://console.example/api/v1/auth/oidc/callback"
)

func TestOIDCBeginBuildsTheAuthorizationRequest(t *testing.T) {
	idp := newFakeIdP(t)
	o := idp.provider()

	to, err := o.Begin(context.Background(), "state-1", testVerifier, testCallback)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(to)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(to, idp.issuer+"/authorize") {
		t.Fatalf("Begin sent the human to %q", to)
	}
	q := u.Query()
	for key, want := range map[string]string{
		"response_type": "code",
		"client_id":     "telecraft-console",
		"redirect_uri":  testCallback,
		"state":         "state-1",
		"nonce":         nonceFrom("state-1"),
	} {
		if q.Get(key) != want {
			t.Fatalf("authorization request %s = %q, want %q", key, q.Get(key), want)
		}
	}
	if !strings.Contains(q.Get("scope"), "openid") {
		t.Fatalf("scope %q does not request openid", q.Get("scope"))
	}
}

// Acceptance: login works via OIDC in tests (issue #26) — the code flow
// against a loopback issuer verifies end to end and yields the claims that
// attribute the human.
func TestOIDCCompleteVerifiesTheCodeFlow(t *testing.T) {
	idp := newFakeIdP(t)
	o := idp.provider()
	idp.claims = idp.goodClaims(o.ClientID, "state-1")

	id, err := o.Complete(context.Background(), "state-1", testVerifier, testCallback, url.Values{"code": {"c0de"}, "state": {"state-1"}})
	if err != nil {
		t.Fatal(err)
	}
	want := Identity{Subject: "idp-subject-1", Name: "Jo Author", Email: "jo@example.com"}
	if id != want {
		t.Fatalf("Complete = %+v, want %+v", id, want)
	}
}

func TestOIDCCompleteRejectsBadTokens(t *testing.T) {
	idp := newFakeIdP(t)
	o := idp.provider()

	mutate := func(f func(map[string]any)) map[string]any {
		c := idp.goodClaims(o.ClientID, "state-1")
		f(c)
		return c
	}
	cases := []struct {
		name   string
		claims map[string]any
		want   string
	}{
		{"a foreign issuer", mutate(func(c map[string]any) { c["iss"] = "https://elsewhere.example" }), "issuer"},
		{"a foreign audience", mutate(func(c map[string]any) { c["aud"] = "someone-else" }), "audience"},
		{"an expired token", mutate(func(c map[string]any) { c["exp"] = time.Now().Add(-time.Minute).Unix() }), "expired"},
		{"a replayed nonce", mutate(func(c map[string]any) { c["nonce"] = nonceFrom("some-other-state") }), "nonce"},
		{"claims with no email", mutate(func(c map[string]any) { delete(c, "email") }), "email"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idp.claims = tc.claims
			_, err := o.Complete(context.Background(), "state-1", testVerifier, testCallback, url.Values{"code": {"c0de"}})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Complete = %v, want an error naming the %s", err, tc.want)
			}
		})
	}
}

func TestOIDCCompleteRejectsAForgedSignature(t *testing.T) {
	idp := newFakeIdP(t)
	o := idp.provider()
	// A second key — one the real IdP's JWKS does not carry — signs an
	// otherwise perfect token, and a rogue token endpoint serves it.
	forger := newFakeIdP(t)
	token := forger.signJWT(t, idp.goodClaims(o.ClientID, "state-1"))
	rogue := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"id_token": token})
	}))
	defer rogue.Close()

	o.disc = &oidcDiscovery{
		Issuer:                idp.issuer,
		AuthorizationEndpoint: idp.issuer + "/authorize",
		TokenEndpoint:         rogue.URL,
		JWKSURI:               idp.issuer + "/jwks",
	}
	_, err := o.Complete(context.Background(), "state-1", testVerifier, testCallback, url.Values{"code": {"c0de"}})
	if err == nil || !strings.Contains(err.Error(), "signature") {
		t.Fatalf("Complete = %v, want a signature failure", err)
	}
}

func TestOIDCCompleteRejectsUnsignedAlgorithms(t *testing.T) {
	idp := newFakeIdP(t)
	o := idp.provider()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	payload, _ := json.Marshal(idp.goodClaims(o.ClientID, "state-1"))
	token := header + "." + base64.RawURLEncoding.EncodeToString(payload) + "."

	_, err := o.verifyIDToken(context.Background(), &oidcDiscovery{JWKSURI: idp.issuer + "/jwks"}, token, nonceFrom("state-1"))
	if err == nil || !strings.Contains(err.Error(), "RS256") {
		t.Fatalf("verifyIDToken = %v, want an RS256-only refusal", err)
	}
}

func TestOIDCCompleteSurfacesProviderErrors(t *testing.T) {
	idp := newFakeIdP(t)
	o := idp.provider()
	_, err := o.Complete(context.Background(), "state-1", testVerifier, testCallback,
		url.Values{"error": {"access_denied"}, "error_description": {"the human said no"}})
	if err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("Complete = %v", err)
	}
	if _, err := o.Complete(context.Background(), "state-1", testVerifier, testCallback, url.Values{}); err == nil {
		t.Fatal("Complete accepted a callback with no code")
	}
}

func TestOIDCDiscoveryFailuresAreNamed(t *testing.T) {
	o := &OIDC{Issuer: "", ClientID: ""}
	if _, err := o.Begin(context.Background(), "s", testVerifier, testCallback); err == nil {
		t.Fatal("Begin ran with no issuer configured")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	o = &OIDC{Issuer: srv.URL, ClientID: "c"}
	_, err := o.Begin(context.Background(), "s", testVerifier, testCallback)
	if err == nil || !strings.Contains(err.Error(), "discovery") {
		t.Fatalf("Begin = %v, want a discovery error", err)
	}
}

// TestOIDCBeginCarriesPKCEChallenge ensures the authorisation request
// includes an S256 code_challenge that is the SHA256 of the supplied verifier.
func TestOIDCBeginCarriesPKCEChallenge(t *testing.T) {
	idp := newFakeIdP(t)
	o := idp.provider()

	const verifier = "test-verifier-one-BBBBBBBBBBBBBBBBBBBBBBBBB"
	to, err := o.Begin(context.Background(), "state-1", verifier, testCallback)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(to)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" {
		t.Fatalf("code_challenge_method = %q, want S256", q.Get("code_challenge_method"))
	}
	sum := sha256.Sum256([]byte(verifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if q.Get("code_challenge") != wantChallenge {
		t.Fatalf("code_challenge = %q, want %q", q.Get("code_challenge"), wantChallenge)
	}
}

// TestOIDCCompletePKCEVerifierMustMatch proves that a code_verifier whose
// SHA256 does not match the challenge registered at authorisation time is
// rejected by the token endpoint. This is the core PKCE security property:
// an attacker who holds the code but not the verifier cannot redeem it.
func TestOIDCCompletePKCEVerifierMustMatch(t *testing.T) {
	idp := newFakeIdP(t)
	o := idp.provider()
	idp.claims = idp.goodClaims(o.ClientID, "state-1")

	// verifier1 was used in Begin; its S256 challenge is what the IdP stored.
	const verifier1 = "CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC"
	sum := sha256.Sum256([]byte(verifier1))
	idp.wantChallenge = base64.RawURLEncoding.EncodeToString(sum[:])

	// verifier2 produces a different challenge — the IdP must reject it.
	const verifier2 = "DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD"
	_, err := o.Complete(context.Background(), "state-1", verifier2, testCallback, url.Values{"code": {"c0de"}})
	if err == nil || !strings.Contains(err.Error(), "token exchange") {
		t.Fatalf("Complete = %v, want a token exchange failure for a mismatched code verifier", err)
	}
}

// TestOIDCCompleteRejectsNotYetValidToken checks that a token whose nbf is
// further into the future than the skew window is refused.
func TestOIDCCompleteRejectsNotYetValidToken(t *testing.T) {
	idp := newFakeIdP(t)
	o := idp.provider()
	claims := idp.goodClaims(o.ClientID, "state-1")
	claims["nbf"] = time.Now().Add(5 * time.Minute).Unix()
	idp.claims = claims

	_, err := o.Complete(context.Background(), "state-1", testVerifier, testCallback, url.Values{"code": {"c0de"}})
	if err == nil || !strings.Contains(err.Error(), "not yet valid") {
		t.Fatalf("Complete = %v, want a not-yet-valid error", err)
	}
}

// TestOIDCCompleteAcceptsTokenWithinSkewWindow checks that a token whose
// exp is just barely in the past but within the clock-skew window is still
// accepted — an issuer whose clock runs slightly ahead would produce this.
func TestOIDCCompleteAcceptsTokenWithinSkewWindow(t *testing.T) {
	idp := newFakeIdP(t)
	o := idp.provider()
	claims := idp.goodClaims(o.ClientID, "state-1")
	claims["exp"] = time.Now().Add(-10 * time.Second).Unix()
	idp.claims = claims

	if _, err := o.Complete(context.Background(), "state-1", testVerifier, testCallback, url.Values{"code": {"c0de"}}); err != nil {
		t.Fatalf("Complete = %v, want acceptance within the skew window", err)
	}
}

// The compiler holds the seam promise: OIDC is a RedirectProvider, Basic a
// PasswordProvider — the two facets a SAML implementation would slot into.
var (
	_ RedirectProvider = (*OIDC)(nil)
	_ PasswordProvider = Basic{}
)
