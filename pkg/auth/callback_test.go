package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// The callback is where a provider's assertion becomes a session, so it is
// the one place where getting it wrong hands somebody else's Organisation
// to whoever asked. These tests are adversarial on purpose: each one is a
// thing an attacker would try, and the assertion is that no session is
// issued.

// stubRedirect is a provider that verifies whatever it is told to. The
// point is to exercise the handler's own checks, which run before and
// after Complete and are the ones an identity provider cannot make for us.
type stubRedirect struct {
	name string

	// completeAs is the identity Complete returns when it is reached.
	completeAs Identity

	// refuse makes Complete fail the way a bad assertion does.
	refuse error

	// sawState and sawVerifier record what the handler passed through, so
	// a test can assert the round trip was not short-circuited.
	sawState    string
	sawVerifier string
	reached     bool
}

func (s *stubRedirect) Name() string { return s.name }

// Begin points at an identity provider that does not exist. It has to be
// somewhere other than the callback: a URL the test client would follow
// straight back here would complete the round trip before the test got to
// choose how to complete it.
func (s *stubRedirect) Begin(_ context.Context, state, _, _ string) (string, error) {
	return "https://idp.invalid/authorize?state=" + url.QueryEscape(state), nil
}

func (s *stubRedirect) Complete(_ context.Context, state, verifier, _ string, _ url.Values) (Identity, error) {
	s.reached = true
	s.sawState, s.sawVerifier = state, verifier
	if s.refuse != nil {
		return Identity{}, s.refuse
	}
	return s.completeAs, nil
}

// begin runs the real start leg so the test holds a genuine state cookie,
// rather than one a test wrote and the handler would never have issued. It
// stops at the redirect rather than following it: what the identity
// provider would do next is exactly what these tests are standing in for.
func begin(t *testing.T, srv *httptest.Server, c *http.Client, provider string) string {
	t.Helper()

	stop := errors.New("stop at the redirect")
	prev := c.CheckRedirect
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return stop }
	defer func() { c.CheckRedirect = prev }()

	res, err := c.Get(srv.URL + "/api/v1/auth/" + provider + "/start")
	if res != nil {
		res.Body.Close()
	}
	if err != nil && !errors.Is(err, stop) {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("start returned nothing")
	}
	location, err := url.Parse(res.Header.Get("Location"))
	if err != nil {
		t.Fatalf("start redirected to an unparseable URL: %v", err)
	}
	state := location.Query().Get("state")
	if state == "" {
		t.Fatal("start redirected without a state, so nothing below is a round trip")
	}
	return state
}

func sessionCookieSet(res *http.Response) bool {
	for _, c := range res.Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			return true
		}
	}
	return false
}

// A callback with no state cookie is somebody arriving with a link, not a
// human finishing a round trip this instance started.
func TestCallbackWithNoStateCookieIssuesNoSession(t *testing.T) {
	p := &stubRedirect{name: "stub", completeAs: Identity{Email: "jo@example.com", Subject: "sub-1"}}
	srv := httptest.NewServer(testHandler(t, p))
	defer srv.Close()

	res, err := srv.Client().Get(srv.URL + "/api/v1/auth/stub/callback?state=anything&code=anything")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", res.StatusCode)
	}
	if sessionCookieSet(res) {
		t.Error("a session was issued to a callback this instance never started")
	}
	if p.reached {
		t.Error("the provider was asked to complete a round trip that had no state")
	}
}

// The state in the URL and the state in the cookie must be the same round
// trip. A mismatch is cross-site request forgery on the sign-in itself.
func TestCallbackWithAMismatchedStateIssuesNoSession(t *testing.T) {
	p := &stubRedirect{name: "stub", completeAs: Identity{Email: "jo@example.com", Subject: "sub-1"}}
	srv := httptest.NewServer(testHandler(t, p))
	defer srv.Close()

	c := client(t, srv)
	_ = begin(t, srv, c, "stub")

	res, err := c.Get(srv.URL + "/api/v1/auth/stub/callback?state=not-the-one&code=anything")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", res.StatusCode)
	}
	if sessionCookieSet(res) {
		t.Error("a session was issued on a state that did not match the cookie")
	}
	if p.reached {
		t.Error("the provider completed a round trip whose state did not match")
	}
}

// A provider that refuses the assertion is the case where the token was
// forged, expired, or signed by the wrong key. The handler must not treat
// a refusal as an anonymous success.
func TestCallbackIssuesNoSessionWhenTheAssertionDoesNotVerify(t *testing.T) {
	p := &stubRedirect{name: "stub", refuse: errors.New("the assertion is signed by an unknown key")}
	srv := httptest.NewServer(testHandler(t, p))
	defer srv.Close()

	c := client(t, srv)
	state := begin(t, srv, c, "stub")

	res, err := c.Get(srv.URL + "/api/v1/auth/stub/callback?state=" +
		url.QueryEscape(state) + "&code=anything")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", res.StatusCode)
	}
	if sessionCookieSet(res) {
		t.Error("a session was issued for an assertion that did not verify")
	}
}

// The gate this whole conversation is about: the provider vouches for
// somebody the estate has never heard of. Authenticating is not authority.
func TestCallbackIssuesNoSessionForAnIdentityTheEstateDoesNotKnow(t *testing.T) {
	p := &stubRedirect{name: "stub", completeAs: Identity{
		Email: "stranger@example.com", Subject: "sub-stranger", Name: "A Stranger",
	}}
	srv := httptest.NewServer(testHandler(t, p))
	defer srv.Close()

	c := client(t, srv)
	state := begin(t, srv, c, "stub")

	res, err := c.Get(srv.URL + "/api/v1/auth/stub/callback?state=" +
		url.QueryEscape(state) + "&code=anything")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
	if sessionCookieSet(res) {
		t.Error("a session was issued to an identity users.yaml does not name")
	}
	if !p.reached {
		t.Fatal("the provider was never reached, so this proved nothing")
	}
}

// The state cookie is spent once. Replaying a callback that already
// succeeded must not mint a second session from the same round trip.
func TestAReplayedCallbackIssuesNoSecondSession(t *testing.T) {
	p := &stubRedirect{name: "stub", completeAs: Identity{
		Email: "jo@example.com", Subject: "sub-jo", Name: "Jo",
	}}
	srv := httptest.NewServer(testHandler(t, p))
	defer srv.Close()

	c := client(t, srv)
	state := begin(t, srv, c, "stub")
	callback := srv.URL + "/api/v1/auth/stub/callback?state=" +
		url.QueryEscape(state) + "&code=anything"

	// Stop at the success redirect rather than following it to a path this
	// handler does not serve.
	stop := errors.New("stop at the redirect")
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return stop }

	first, err := c.Get(callback)
	if first != nil {
		first.Body.Close()
	}
	if err != nil && !errors.Is(err, stop) {
		t.Fatal(err)
	}
	if first.StatusCode != http.StatusFound {
		t.Fatalf("the first callback returned %d rather than a redirect, so the replay proves nothing", first.StatusCode)
	}
	if !sessionCookieSet(first) {
		t.Fatal("the first callback issued no session, so the replay proves nothing")
	}

	// The client's jar now holds the cleared state cookie, exactly as a
	// browser would after the first exchange.
	second, err := c.Get(callback)
	if second != nil {
		defer second.Body.Close()
	}
	if err != nil && !errors.Is(err, stop) {
		t.Fatal(err)
	}

	if second.StatusCode < 400 {
		t.Errorf("a replayed callback was accepted with %d", second.StatusCode)
	}
}

// A provider name that is not wired is not a hint about what is.
func TestCallbackForAnUnknownProviderSaysNothingAboutWhatExists(t *testing.T) {
	srv := httptest.NewServer(testHandler(t))
	defer srv.Close()

	res, err := srv.Client().Get(srv.URL + "/api/v1/auth/not-a-provider/callback?state=x&code=y")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
	if sessionCookieSet(res) {
		t.Error("a session was issued by a provider that does not exist")
	}
}

// The verifier committed to at Begin is what reaches Complete. If the
// handler passed a different one, the back-channel exchange would be
// unbound from the front-channel round trip.
func TestTheVerifierFromBeginIsWhatCompleteIsGiven(t *testing.T) {
	p := &stubRedirect{name: "stub", completeAs: Identity{Email: "jo@example.com", Subject: "sub-jo"}}
	srv := httptest.NewServer(testHandler(t, p))
	defer srv.Close()

	c := client(t, srv)
	state := begin(t, srv, c, "stub")

	res, err := c.Get(srv.URL + "/api/v1/auth/stub/callback?state=" + url.QueryEscape(state) + "&code=anything")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()

	if p.sawState != state {
		t.Errorf("Complete saw state %q, want %q", p.sawState, state)
	}
	if strings.TrimSpace(p.sawVerifier) == "" {
		t.Error("Complete was given an empty verifier, so the exchange is unbound from the round trip")
	}
}

// they are closed already.
func TestLoginRefusesARequestACrossOriginFormCouldHaveMade(t *testing.T) {
	srv := httptest.NewServer(testHandler(t))
	defer srv.Close()

	// What an auto-submitting form on another origin produces: a content
	// type a form can set, and a body whose trailing separator the JSON
	// decoder would otherwise ignore.
	body := `{"provider":"basic","username":"jo@example.com","secret":"correct horse battery"}=` + "\r\n"

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/login", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "text/plain;charset=UTF-8")

	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
	if sessionCookieSet(res) {
		t.Error("a cross-origin form signed this browser in")
	}
}

// Sec-Fetch-Site is the browser saying so directly, and it is believed
// ahead of the content type.
func TestLoginRefusesWhatTheBrowserCallsCrossSite(t *testing.T) {
	srv := httptest.NewServer(testHandler(t))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/v1/auth/login",
		strings.NewReader(`{"provider":"basic","username":"jo@example.com","secret":"correct horse battery"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "cross-site")

	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403", res.StatusCode)
	}
	if sessionCookieSet(res) {
		t.Error("a cross-site request signed this browser in")
	}
}

// The console's own call still works, which is the half that makes the
// refusal above a gate rather than an outage.
func TestLoginStillWorksFromThisInstance(t *testing.T) {
	srv := httptest.NewServer(testHandler(t))
	defer srv.Close()

	c := client(t, srv)
	res := postJSON(t, c, srv.URL+"/api/v1/auth/login",
		map[string]string{"provider": "basic", "username": "jo@example.com", "secret": "correct horse battery"})
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if !sessionCookieSet(res) {
		t.Error("a same-origin sign-in issued no session")
	}
}
