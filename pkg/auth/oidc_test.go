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
	"reflect"
	"strings"
	"testing"
	"time"
)

// fakeIdP is a local OpenID Connect issuer: discovery, a token endpoint
// and a JWKS over one RSA key. It stands in for the self-hosted identity
// provider of an air-gapped deployment: everything the OIDC provider
// talks to lives on the loopback interface.
type fakeIdP struct {
	key    *rsa.PrivateKey
	srv    *httptest.Server
	issuer string

	// claims is what the next token exchange asserts; tests mutate it.
	claims map[string]any

	// tokenStatus lets a test fail the exchange.
	tokenStatus int

	// codeChallenge, when set, makes the token endpoint enforce PKCE the
	// way RFC 7636 §4.6 asks: the exchange must present a code_verifier
	// whose S256 transformation is this challenge. Empty means the issuer
	// does not ask for PKCE, which is the case every other test runs in.
	codeChallenge string
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
		if idp.codeChallenge != "" && challengeFor(r.PostForm.Get("code_verifier")) != idp.codeChallenge {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_grant"})
			return
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
	testCallback = "http://console.example/api/v1/auth/oidc/callback"

	// testVerifier and testOtherVerifier stand in for the random material
	// the HTTP layer draws per sign-in: 43 unreserved characters each, the
	// shape RFC 7636 §4.1 asks a verifier to be. Two of them, because a
	// verifier is only meaningful against another attempt's.
	testVerifier      = "D0TcJ7haTijb0XJeD3wgp_XG437krNB0eGHoIPhpTCc"
	testOtherVerifier = "u6uJhM3ehb6CKKNNKY2HmEYGx0FIVrItgyvXCvdtKrc"
)

// legacyVerifierFrom is the derivation issue #124 shipped and issue #145
// removed: the verifier as a labelled hash of the state. It survives here
// only so a test can hold an attacker's own recomputation of it against
// the flow, and prove that recomputation no longer opens a code.
func legacyVerifierFrom(state string) string {
	sum := sha256.Sum256([]byte("telecraft-oidc-verifier." + state))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

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

// Acceptance (issue #124, tightened by #145): the authorization request
// commits to an S256 challenge over the verifier the caller supplies, so
// the code the issuer hands back is bound to that secret and to nothing
// else.
func TestOIDCBeginCarriesAnS256CodeChallenge(t *testing.T) {
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
	q := u.Query()
	if got := q.Get("code_challenge_method"); got != "S256" {
		t.Fatalf("code_challenge_method = %q, want S256", got)
	}
	if got, want := q.Get("code_challenge"), challengeFor(testVerifier); got != want {
		t.Fatalf("code_challenge = %q, want %q", got, want)
	}
	// The challenge is the hash, never the verifier: sending the verifier
	// through the browser would bind the code to a value an interceptor
	// already holds.
	if q.Get("code_challenge") == testVerifier {
		t.Fatal("the authorization request carries the verifier itself, not its S256 challenge")
	}
	// The commitment follows the verifier, not the state: two attempts
	// under the same state commit to different challenges, which is what
	// makes the verifier the thing a code is bound to.
	other, err := o.Begin(context.Background(), "state-1", testOtherVerifier, testCallback)
	if err != nil {
		t.Fatal(err)
	}
	otherQ, err := url.Parse(other)
	if err != nil {
		t.Fatal(err)
	}
	if otherQ.Query().Get("code_challenge") == q.Get("code_challenge") {
		t.Fatal("two verifiers under one state commit to the same challenge")
	}
	// The three artefacts of one sign-in stay distinct values: the nonce
	// derives from the state, the challenge derives from the verifier, and
	// neither discloses the other.
	if q.Get("code_challenge") == q.Get("nonce") || testVerifier == nonceFrom("state-1") {
		t.Fatal("the verifier, the challenge and the nonce are not distinct values")
	}
}

// Acceptance (issue #145): a code cannot be completed with a verifier
// recomputed from the state. The issuer enforces PKCE over the challenge
// this sign-in committed to, and the exchange presents the verifier the
// derivation issue #124 shipped would have produced, which is everything
// an interceptor holding the callback could compute for itself.
func TestOIDCCompleteRefusesAVerifierRecomputedFromTheState(t *testing.T) {
	idp := newFakeIdP(t)
	o := idp.provider()
	idp.codeChallenge = challengeFor(testVerifier)
	idp.claims = idp.goodClaims(o.ClientID, "state-1")

	_, err := o.Complete(context.Background(), "state-1", legacyVerifierFrom("state-1"), testCallback, url.Values{"code": {"c0de"}})
	if err == nil || !strings.Contains(err.Error(), "token exchange") {
		t.Fatalf("Complete = %v, want the token endpoint to refuse a verifier recomputed from the state", err)
	}

	// The real verifier passes the same issuer, which is what shows the
	// refusal above was the verifier and not the enforcement itself.
	if _, err := o.Complete(context.Background(), "state-1", testVerifier, testCallback, url.Values{"code": {"c0de"}}); err != nil {
		t.Fatalf("Complete with the matching verifier = %v", err)
	}
}

// Acceptance (issue #124): an exchange whose verifier does not match the
// challenge is refused. The issuer here enforces PKCE over the challenge
// one sign-in committed to, and the exchange runs with another attempt's
// verifier, so the verifier Complete presents is the wrong one.
func TestOIDCCompleteFailsOnAMismatchedVerifier(t *testing.T) {
	idp := newFakeIdP(t)
	o := idp.provider()
	idp.codeChallenge = challengeFor(testVerifier)
	idp.claims = idp.goodClaims(o.ClientID, "state-1")

	_, err := o.Complete(context.Background(), "state-1", testOtherVerifier, testCallback, url.Values{"code": {"c0de"}})
	if err == nil || !strings.Contains(err.Error(), "token exchange") {
		t.Fatalf("Complete = %v, want the token endpoint to refuse the mismatched verifier", err)
	}

	// The matching verifier passes the same issuer, which is what shows the
	// refusal above was the verifier and not the enforcement itself.
	if _, err := o.Complete(context.Background(), "state-1", testVerifier, testCallback, url.Values{"code": {"c0de"}}); err != nil {
		t.Fatalf("Complete with the matching verifier = %v", err)
	}
}

// Acceptance: login works via OIDC in tests (issue #26): the code flow
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
	if !reflect.DeepEqual(id, want) {
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

// Acceptance (issue #124): iat and nbf are judged when the token asserts
// them, and the clock-skew allowance is what separates a token from an
// issuer whose clock disagrees slightly from one that is genuinely invalid.
func TestOIDCCompleteJudgesTheTimeClaimsWithinTheSkewAllowance(t *testing.T) {
	idp := newFakeIdP(t)
	o := idp.provider()

	// The allowance is bounded, which is what keeps it an allowance. A
	// token whose expiry passed a minute ago stays refused.
	if clockSkew <= 0 || clockSkew >= time.Minute {
		t.Fatalf("clockSkew = %v, want a bounded allowance under a minute", clockSkew)
	}

	mutate := func(f func(map[string]any)) map[string]any {
		c := idp.goodClaims(o.ClientID, "state-1")
		f(c)
		return c
	}
	// Just inside the allowance, and far outside it. The signing time is
	// the moment the exchange runs, so both offsets are read against a
	// clock that has moved on a little since the claims were built.
	near, far := 10*time.Second, 10*time.Minute

	refused := []struct {
		name   string
		claims map[string]any
		want   string
	}{
		{"a token that is not valid yet", mutate(func(c map[string]any) {
			c["nbf"] = time.Now().Add(far).Unix()
		}), "not valid yet"},
		{"a token issued in the future", mutate(func(c map[string]any) {
			c["iat"] = time.Now().Add(far).Unix()
		}), "issued in the future"},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			idp.claims = tc.claims
			_, err := o.Complete(context.Background(), "state-1", testVerifier, testCallback, url.Values{"code": {"c0de"}})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Complete = %v, want an error saying the token is %s", err, tc.want)
			}
		})
	}

	accepted := []struct {
		name   string
		claims map[string]any
	}{
		{"an nbf a little ahead of this clock", mutate(func(c map[string]any) {
			c["nbf"] = time.Now().Add(near).Unix()
		})},
		{"an iat a little ahead of this clock", mutate(func(c map[string]any) {
			c["iat"] = time.Now().Add(near).Unix()
		})},
		{"an exp a little behind this clock", mutate(func(c map[string]any) {
			c["exp"] = time.Now().Add(-near).Unix()
		})},
		{"a token asserting neither iat nor nbf", mutate(func(c map[string]any) {
			delete(c, "iat")
			delete(c, "nbf")
		})},
	}
	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			idp.claims = tc.claims
			if _, err := o.Complete(context.Background(), "state-1", testVerifier, testCallback, url.Values{"code": {"c0de"}}); err != nil {
				t.Fatalf("Complete refused %s: %v", tc.name, err)
			}
		})
	}
}

func TestOIDCCompleteRejectsAForgedSignature(t *testing.T) {
	idp := newFakeIdP(t)
	o := idp.provider()
	// A second key, one the real IdP's JWKS does not carry, signs an
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

	_, _, err := o.verifyIDToken(context.Background(), &oidcDiscovery{JWKSURI: idp.issuer + "/jwks"}, token, nonceFrom("state-1"))
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

// The compiler holds the seam promise: OIDC is a RedirectProvider, Basic a
// PasswordProvider: the two facets a SAML implementation would slot into.
var (
	_ RedirectProvider = (*OIDC)(nil)
	_ PasswordProvider = Basic{}
)

// The groups claim is read only where the estate named one, and it is read
// out of the same verified bytes the rest of the claims came from, never a
// second parse of an unverified token.
func TestOIDCReadsTheGroupsClaimTheEstateNames(t *testing.T) {
	cases := map[string]struct {
		claim  string
		value  any
		want   []string
		absent bool
	}{
		"a list of groups":              {"groups", []string{"platform-engineering", "everyone"}, []string{"platform-engineering", "everyone"}, false},
		"one space-separated string":    {"groups", "platform-engineering everyone", []string{"platform-engineering", "everyone"}, false},
		"a claim under another name":    {"roles", []string{"platform-engineering"}, []string{"platform-engineering"}, false},
		"the estate named none":         {"", []string{"platform-engineering"}, nil, false},
		"the issuer released none":      {"groups", nil, nil, true},
		"a claim of an unusable shape":  {"groups", map[string]any{"a": 1}, nil, false},
		"a claim the issuer left empty": {"groups", "", nil, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			idp := newFakeIdP(t)
			o := idp.provider()
			o.GroupsClaim = tc.claim
			claims := idp.goodClaims(o.ClientID, "state-1")
			if !tc.absent {
				key := tc.claim
				if key == "" {
					key = "groups"
				}
				claims[key] = tc.value
			}
			idp.claims = claims

			id, err := o.Complete(context.Background(), "state-1", testVerifier, testCallback,
				url.Values{"code": {"c0de"}, "state": {"state-1"}})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(id.Groups, tc.want) {
				t.Fatalf("groups = %v, want %v", id.Groups, tc.want)
			}
		})
	}
}

// The email address is the whole of the join into users.yaml, so an
// address the issuer will not vouch for is an address somebody may have
// typed. An issuer that says nothing is a different case from one that has
// looked and said no, and only the second is refused.
func TestOIDCCompleteRefusesAnEmailTheIssuerSaysIsUnverified(t *testing.T) {
	idp := newFakeIdP(t)
	o := idp.provider()

	mutate := func(f func(map[string]any)) map[string]any {
		c := idp.goodClaims(o.ClientID, "state-1")
		f(c)
		return c
	}

	t.Run("email_verified false is refused", func(t *testing.T) {
		idp.claims = mutate(func(c map[string]any) { c["email_verified"] = false })
		_, err := o.Complete(context.Background(), "state-1", testVerifier, testCallback, url.Values{"code": {"c0de"}})
		if err == nil || !strings.Contains(err.Error(), "not verified") {
			t.Fatalf("Complete = %v, want a refusal naming the unverified address", err)
		}
	})

	t.Run("email_verified true signs in", func(t *testing.T) {
		idp.claims = mutate(func(c map[string]any) { c["email_verified"] = true })
		id, err := o.Complete(context.Background(), "state-1", testVerifier, testCallback, url.Values{"code": {"c0de"}})
		if err != nil {
			t.Fatalf("Complete = %v, want the identity", err)
		}
		if id.Email == "" {
			t.Error("no email on an identity the issuer verified")
		}
	})

	// A small or self-hosted issuer that never sends the claim is not
	// making a negative assertion, and refusing it would break deployments
	// ADR-0019 §1 supports for a signal they never sent.
	t.Run("an absent claim is not a refusal", func(t *testing.T) {
		idp.claims = mutate(func(c map[string]any) { delete(c, "email_verified") })
		if _, err := o.Complete(context.Background(), "state-1", testVerifier, testCallback, url.Values{"code": {"c0de"}}); err != nil {
			t.Fatalf("Complete = %v, want an issuer that says nothing to be accepted", err)
		}
	})
}
