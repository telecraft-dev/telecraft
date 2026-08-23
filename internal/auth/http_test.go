package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// testHandler wires the full stack the way a deployment would: the fixture
// tree, a users.yaml with one basic-auth credential, and both first-party
// providers.
func testHandler(t *testing.T, providers ...Provider) *Handler {
	t.Helper()
	users := usersWithPassword(t, "correct horse battery")
	if len(providers) == 0 {
		providers = []Provider{Basic{Users: users}}
	}
	h, err := NewHandler(HandlerConfig{
		Sessions:  testSessions(t),
		Users:     users,
		Tree:      testTree(),
		Providers: providers,
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// client returns a cookie-jarred client against a started handler.
func client(t *testing.T, srv *httptest.Server) *http.Client {
	t.Helper()
	c := srv.Client()
	jar, err := newTestJar()
	if err != nil {
		t.Fatal(err)
	}
	c.Jar = jar
	// Redirects are followed by default; tests that assert on a 302 use
	// their own CheckRedirect.
	return c
}

func postJSON(t *testing.T, c *http.Client, url string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.Post(url, "application/json", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func decodeMe(t *testing.T, res *http.Response) mePayload {
	t.Helper()
	defer res.Body.Close()
	var me mePayload
	if err := json.NewDecoder(res.Body).Decode(&me); err != nil {
		t.Fatal(err)
	}
	return me
}

// Acceptance: login works via basic auth in tests (issue #26), and the
// resulting /api/v1/me carries the ownership-derived edit horizon the
// console gates actions on.
func TestBasicLoginSignsInAndDerivesEditableTeams(t *testing.T) {
	srv := httptest.NewServer(testHandler(t))
	defer srv.Close()
	c := client(t, srv)

	// Unauthenticated, the API refuses.
	res, err := c.Get(srv.URL + "/api/v1/me")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me before sign-in = %d, want 401", res.StatusCode)
	}

	res = postJSON(t, c, srv.URL+"/api/v1/auth/login",
		map[string]string{"provider": "basic", "username": "jo@example.com", "secret": "correct horse battery"})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("login = %d", res.StatusCode)
	}
	me := decodeMe(t, res)
	want := mePayload{ID: "jo@example.com", Name: "Jo Author", Team: "data-flow", EditableTeams: []string{"data-flow", "edge"}}
	if !reflect.DeepEqual(me, want) {
		t.Fatalf("login payload = %+v, want %+v", me, want)
	}

	// The session cookie now answers /api/v1/me identically.
	res, err = c.Get(srv.URL + "/api/v1/me")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("me after sign-in = %d", res.StatusCode)
	}
	if me := decodeMe(t, res); !reflect.DeepEqual(me, want) {
		t.Fatalf("me = %+v, want %+v", me, want)
	}
}

func TestBasicLoginFailsUniformlyOverHTTP(t *testing.T) {
	srv := httptest.NewServer(testHandler(t))
	defer srv.Close()
	c := client(t, srv)

	for name, body := range map[string]map[string]string{
		"a wrong secret":   {"username": "jo@example.com", "secret": "wrong"},
		"an unknown email": {"username": "stranger@example.com", "secret": "correct horse battery"},
	} {
		res := postJSON(t, c, srv.URL+"/api/v1/auth/login", body)
		res.Body.Close()
		if res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s = %d, want the uniform 401", name, res.StatusCode)
		}
	}
}

func TestLogoutEndsTheSession(t *testing.T) {
	srv := httptest.NewServer(testHandler(t))
	defer srv.Close()
	c := client(t, srv)

	postJSON(t, c, srv.URL+"/api/v1/auth/login",
		map[string]string{"username": "jo@example.com", "secret": "correct horse battery"}).Body.Close()
	postJSON(t, c, srv.URL+"/api/v1/auth/logout", map[string]string{}).Body.Close()

	res, err := c.Get(srv.URL + "/api/v1/me")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me after logout = %d, want 401", res.StatusCode)
	}
}

// Acceptance: login works via OIDC in tests (issue #26): the whole
// redirect round trip through the handler against a loopback issuer, with
// nothing beyond this process involved (REQ-006).
func TestOIDCLoginRoundTripsThroughTheHandler(t *testing.T) {
	idp := newFakeIdP(t)
	oidc := idp.provider()
	users := usersWithPassword(t, "correct horse battery")
	h, err := NewHandler(HandlerConfig{
		Sessions:  testSessions(t),
		Users:     users,
		Tree:      testTree(),
		Providers: []Provider{Basic{Users: users}, oidc},
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	c := client(t, srv)
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	// The sign-in surface reads what this instance offers.
	res, err := c.Get(srv.URL + "/api/v1/auth/providers")
	if err != nil {
		t.Fatal(err)
	}
	var infos []providerInfo
	if err := json.NewDecoder(res.Body).Decode(&infos); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	wantInfos := []providerInfo{{Name: "basic", Flow: "password"}, {Name: "oidc", Flow: "redirect"}}
	if !reflect.DeepEqual(infos, wantInfos) {
		t.Fatalf("providers = %+v, want %+v", infos, wantInfos)
	}

	// Start: a 302 to the issuer, with the state anchored in a cookie.
	res, err = c.Get(srv.URL + "/api/v1/auth/oidc/start?return_to=%2Fcompose%3Fobject%3Dblueprint%253Ax")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("start = %d, want 302", res.StatusCode)
	}
	to, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(res.Header.Get("Location"), idp.issuer+"/authorize") {
		t.Fatalf("start redirected to %q", res.Header.Get("Location"))
	}
	state := to.Query().Get("state")
	if state == "" {
		t.Fatal("the authorization request carries no state")
	}

	// The issuer authenticates the human and sends them back with a code.
	idp.claims = idp.goodClaims(oidc.ClientID, state)
	res, err = c.Get(srv.URL + "/api/v1/auth/oidc/callback?code=c0de&state=" + url.QueryEscape(state))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("callback = %d, want 302", res.StatusCode)
	}
	if got := res.Header.Get("Location"); got != "/compose?object=blueprint%3Ax" {
		t.Fatalf("callback returned the human to %q", got)
	}

	// Signed in: me answers with the ownership-derived horizon.
	res, err = c.Get(srv.URL + "/api/v1/me")
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("me after OIDC sign-in = %d", res.StatusCode)
	}
	me := decodeMe(t, res)
	if me.ID != "jo@example.com" || me.Team != "data-flow" {
		t.Fatalf("me = %+v", me)
	}
}

// stateCookieValue reads the state cookie a response sets.
func stateCookieValue(t *testing.T, res *http.Response) string {
	t.Helper()
	for _, c := range res.Cookies() {
		if c.Name == stateCookie {
			return c.Value
		}
	}
	t.Fatal("the response set no state cookie")
	return ""
}

// Acceptance (issue #145): the verifier the authorization request commits
// to is independent crypto/rand material carried in the signed state
// cookie. Nothing the callback carries recomputes it, and the round trip
// completes against an issuer that enforces the challenge.
func TestOIDCStartDrawsAnIndependentVerifierIntoTheStateCookie(t *testing.T) {
	idp := newFakeIdP(t)
	oidc := idp.provider()
	h := testHandler(t, oidc)
	srv := httptest.NewServer(h)
	defer srv.Close()
	c := client(t, srv)
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	// start runs one sign-in as far as the redirect, and reports what the
	// authorization request committed to beside what the cookie holds.
	start := func() (state, challenge, verifier string) {
		t.Helper()
		res, err := c.Get(srv.URL + "/api/v1/auth/oidc/start")
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		to, err := url.Parse(res.Header.Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		state, verifier, returnTo, err := h.verifyStateCookie(stateCookieValue(t, res))
		if err != nil {
			t.Fatalf("the cookie start wrote does not verify: %v", err)
		}
		if returnTo != "/" {
			t.Fatalf("the cookie carries the return path %q, want the default", returnTo)
		}
		if got := to.Query().Get("state"); got != state {
			t.Fatalf("the authorization request carries state %q, the cookie %q", got, state)
		}
		return state, to.Query().Get("code_challenge"), verifier
	}

	state, challenge, verifier := start()
	if len(verifier) != 43 {
		t.Fatalf("the verifier is %d characters, want the 43 RFC 7636 §4.1 asks for", len(verifier))
	}
	if _, err := base64.RawURLEncoding.DecodeString(verifier); err != nil {
		t.Fatalf("the verifier %q is not base64url, so it is not the unreserved set: %v", verifier, err)
	}
	if got := challengeFor(verifier); got != challenge {
		t.Fatalf("code_challenge = %q, want the S256 of the cookie's verifier %q", challenge, got)
	}
	// The point of the change: the callback carries the state, and the
	// state computes nothing. An interceptor holding the code and the
	// state still cannot produce the verifier the exchange needs.
	if verifier == legacyVerifierFrom(state) || challenge == challengeFor(legacyVerifierFrom(state)) {
		t.Fatal("the verifier is recomputable from the state the callback carries")
	}
	if verifier == state || verifier == nonceFrom(state) {
		t.Fatal("the verifier is a value the round trip already publishes")
	}

	// Independent per attempt: two sign-ins share no material at all.
	state2, challenge2, verifier2 := start()
	if verifier2 == verifier || state2 == state || challenge2 == challenge {
		t.Fatal("two sign-in attempts drew the same material")
	}

	// The exchange presents the cookie's verifier: this issuer enforces
	// the challenge the second authorization request committed to.
	idp.codeChallenge = challenge2
	idp.claims = idp.goodClaims(oidc.ClientID, state2)
	res, err := c.Get(srv.URL + "/api/v1/auth/oidc/callback?code=c0de&state=" + url.QueryEscape(state2))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("callback under an issuer enforcing PKCE = %d, want 302", res.StatusCode)
	}
}

// Acceptance (issue #145): a state cookie this instance did not write in
// this format is refused rather than read loosely, and the deploy-window
// case is named. A sign-in in flight when the server was updated carries a
// payload the current build signs but cannot complete, because the
// verifier its authorization request committed to was never in it.
func TestOIDCCallbackRefusesAMalformedOrOldFormatStateCookie(t *testing.T) {
	idp := newFakeIdP(t)
	h := testHandler(t, idp.provider())
	srv := httptest.NewServer(h)
	defer srv.Close()

	signed := func(blob string) string { return blob + "." + h.cfg.Sessions.sign(blob) }

	cases := []struct {
		name   string
		cookie string
		want   string
	}{
		{"the state-and-return-path payload an older build wrote", signed("state-1|/compose"), "before the server was updated"},
		{"an older build's payload whose return path holds a pipe", signed("state-1|/a|/b"), "before the server was updated"},
		{"a payload with no separator at all", signed("state-1"), "did not start here"},
		{"a payload with an empty state", signed("v2||" + testVerifier + "|/"), "did not start here"},
		{"a payload with an empty verifier", signed("v2|state-1||/"), "did not start here"},
		{"a payload with no return path", signed("v2|state-1|" + testVerifier), "did not start here"},
		{"a payload with an off-site return path", signed("v2|state-1|" + testVerifier + "|https://elsewhere.example"), "did not start here"},
		{"a payload nothing signed", "v2|state-1|" + testVerifier + "|/", "did not start here"},
		{"a payload whose signature is somebody else's", "v2|state-1|" + testVerifier + "|/.bm90LWEtc2lnbmF0dXJl", "did not start here"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/auth/oidc/callback?code=c0de&state=state-1", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.AddCookie(&http.Cookie{Name: stateCookie, Value: tc.cookie})
			res, err := srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusBadRequest {
				t.Fatalf("callback with %s = %d, want 400", tc.name, res.StatusCode)
			}
			var body map[string]string
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(body["error"], tc.want) {
				t.Fatalf("callback with %s answered %q, want a message naming %q", tc.name, body["error"], tc.want)
			}
		})
	}
}

func TestOIDCCallbackRefusesAForeignOrMissingState(t *testing.T) {
	idp := newFakeIdP(t)
	h := testHandler(t, idp.provider())
	srv := httptest.NewServer(h)
	defer srv.Close()

	// No state cookie at all: the attempt did not start here.
	res, err := srv.Client().Get(srv.URL + "/api/v1/auth/oidc/callback?code=c0de&state=x")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("callback without a state cookie = %d, want 400", res.StatusCode)
	}

	// A started attempt, but the callback carries a different state.
	c := client(t, srv)
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err = c.Get(srv.URL + "/api/v1/auth/oidc/start")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	res, err = c.Get(srv.URL + "/api/v1/auth/oidc/callback?code=c0de&state=not-the-state")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("callback with a foreign state = %d, want 400", res.StatusCode)
	}
}

func TestOIDCCallbackNamesTheFixWhenTheEstateDoesNotKnowTheEmail(t *testing.T) {
	idp := newFakeIdP(t)
	oidc := idp.provider()
	h := testHandler(t, oidc)
	srv := httptest.NewServer(h)
	defer srv.Close()
	c := client(t, srv)
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	res, err := c.Get(srv.URL + "/api/v1/auth/oidc/start")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	state, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	claims := idp.goodClaims(oidc.ClientID, state.Query().Get("state"))
	claims["email"] = "verified-stranger@example.com"
	idp.claims = claims

	res, err = c.Get(srv.URL + "/api/v1/auth/oidc/callback?code=c0de&state=" + url.QueryEscape(state.Query().Get("state")))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("callback = %d, want 403: the identity is verified, the estate just has no place for it", res.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body["error"], UsersFile) {
		t.Fatalf("the 403 %q does not name the fix", body["error"])
	}
}

func TestStartSanitisesTheReturnPath(t *testing.T) {
	idp := newFakeIdP(t)
	h := testHandler(t, idp.provider())
	srv := httptest.NewServer(h)
	defer srv.Close()

	for _, evil := range []string{"https://elsewhere.example/", "//elsewhere.example", "/\\elsewhere.example"} {
		c := client(t, srv)
		c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		res, err := c.Get(srv.URL + "/api/v1/auth/oidc/start?return_to=" + url.QueryEscape(evil))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		state, _ := url.Parse(res.Header.Get("Location"))
		idp.claims = idp.goodClaims(idp.provider().ClientID, state.Query().Get("state"))
		res, err = c.Get(srv.URL + "/api/v1/auth/oidc/callback?code=c0de&state=" + url.QueryEscape(state.Query().Get("state")))
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if got := res.Header.Get("Location"); got != "/" {
			t.Fatalf("return_to %q escaped to %q", evil, got)
		}
	}
}

func TestNewHandlerRefusesBadWiring(t *testing.T) {
	if _, err := NewHandler(HandlerConfig{}); err == nil {
		t.Fatal("NewHandler accepted an instance nobody can sign in to")
	}
	users := writeUsers(t, goodUsers)
	_, err := NewHandler(HandlerConfig{
		Providers: []Provider{Basic{Users: users}, Basic{Users: users}},
	})
	if err == nil || !strings.Contains(err.Error(), "named") {
		t.Fatalf("NewHandler = %v, want a duplicate-name refusal", err)
	}
	_, err = NewHandler(HandlerConfig{Providers: []Provider{facetless{}}})
	if err == nil || !strings.Contains(err.Error(), "neither a password provider") {
		t.Fatalf("NewHandler = %v, want a refusal naming the missing flow", err)
	}
}

type facetless struct{}

func (facetless) Name() string { return "facetless" }

func TestSessionSurvivesOnlyWhileTheEstateKnowsTheUser(t *testing.T) {
	// Two handlers over the same sessions key: the second's users.yaml no
	// longer holds jo. The session cookie still verifies, but Require
	// refuses, which is how removal from users.yaml revokes.
	sessions := testSessions(t)
	usersWith := usersWithPassword(t, "correct horse battery")
	usersWithout := writeUsers(t, `
users:
  - email: sam@example.com
    name: Sam Guardian
    owner: pii-guardians
`)
	before, err := NewHandler(HandlerConfig{Sessions: sessions, Users: usersWith, Tree: testTree(), Providers: []Provider{Basic{Users: usersWith}}})
	if err != nil {
		t.Fatal(err)
	}
	after, err := NewHandler(HandlerConfig{Sessions: sessions, Users: usersWithout, Tree: testTree(), Providers: []Provider{Basic{Users: usersWithout}}})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(before)
	c := client(t, srv)
	postJSON(t, c, srv.URL+"/api/v1/auth/login",
		map[string]string{"username": "jo@example.com", "secret": "correct horse battery"}).Body.Close()
	res, err := c.Get(srv.URL + "/api/v1/me")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("me before removal = %d", res.StatusCode)
	}
	srv.Close()

	srv2 := httptest.NewServer(after)
	defer srv2.Close()
	// Reuse the cookie the first server set.
	req, err := http.NewRequest(http.MethodGet, srv2.URL+"/api/v1/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range c.Jar.Cookies(u) {
		req.AddCookie(cookie)
	}
	res, err = srv2.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me after removal from %s = %d, want 401", UsersFile, res.StatusCode)
	}
}

// newTestJar is the flow tests' cookie jar.
func newTestJar() (http.CookieJar, error) {
	return cookiejar.New(nil)
}
