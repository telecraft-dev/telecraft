package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/telecraft-dev/telecraft/pkg/ownership"
)

// Handler serves the auth slice of the documented platform API
// (console/README.md): how sign-in works on this instance, the sign-in
// round trips themselves, and /api/v1/me, the signed-in actor as the
// console consumes it. Require is the gate the rest of the API mounts
// behind.
type Handler struct {
	cfg HandlerConfig
	mux *http.ServeMux
}

// HandlerConfig wires the handler to the seams it composes.
type HandlerConfig struct {
	Sessions Sessions
	Users    Users
	Tree     ownership.Tree

	// Providers is what this instance offers, in sign-in surface order.
	// Each must satisfy PasswordProvider or RedirectProvider.
	Providers []Provider

	// Groups is the estate's mapping from an asserted group to the Owner
	// its members act as, empty where the estate authored none.
	Groups Groups

	// Secure marks the cookies Secure, the behind-TLS deployment shape.
	Secure bool

	// ExternalURL is the address a browser reaches this instance at, when
	// the deployment declares one. The redirect round trip is built from
	// it, so a provider returns the human to the address they arrived on
	// rather than to whatever a terminator forwarded (ADR-0067 §5). Empty
	// builds the callback from the request, which is the loopback shape
	// with nothing in front.
	ExternalURL string
}

// NewHandler validates the wiring and builds the routes.
func NewHandler(cfg HandlerConfig) (*Handler, error) {
	if len(cfg.Providers) == 0 {
		return nil, fmt.Errorf("no authentication provider is configured, so nobody could sign in")
	}
	seen := map[string]bool{}
	for _, p := range cfg.Providers {
		if seen[p.Name()] {
			return nil, fmt.Errorf("two providers are named %q", p.Name())
		}
		seen[p.Name()] = true
		switch p.(type) {
		case PasswordProvider, RedirectProvider:
		default:
			return nil, fmt.Errorf("provider %q is neither a password provider nor a redirect provider", p.Name())
		}
		if postsCallback(p) && !cfg.Secure {
			return nil, fmt.Errorf("provider %q returns people here by a form post from the identity provider, and the cookie that carries a sign-in attempt across that post is only sent over HTTPS. Serve this instance over HTTPS and give it an external URL that begins with https://", p.Name())
		}
	}

	h := &Handler{cfg: cfg, mux: http.NewServeMux()}
	h.mux.HandleFunc("GET /api/v1/auth/providers", h.providers)
	h.mux.HandleFunc("POST /api/v1/auth/login", h.login)
	h.mux.HandleFunc("POST /api/v1/auth/logout", h.logout)
	h.mux.HandleFunc("GET /api/v1/auth/{provider}/start", h.start)
	h.mux.HandleFunc("GET /api/v1/auth/{provider}/callback", h.callback)
	// The assertion consumer binding: a redirect provider whose identity
	// provider returns the human by submitting a form to this address.
	h.mux.HandleFunc("POST /api/v1/auth/{provider}/callback", h.callback)
	h.mux.Handle("GET /api/v1/me", h.Require(http.HandlerFunc(h.me)))
	// Anything else beneath the auth prefix is a 404 with a JSON body,
	// like every other unknown API path. It is answered here because a
	// server mounting this handler under the prefix cannot see inside it.
	h.mux.HandleFunc("/api/v1/auth/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint: "+r.URL.Path)
	})
	return h, nil
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.mux.ServeHTTP(w, r) }

const (
	sessionCookie = "telecraft_session"
	stateCookie   = "telecraft_auth_state"
)

type actorKey struct{}

// ActorFrom returns the actor Require resolved for this request.
func ActorFrom(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok
}

// Require gates next behind a signed-in, resolved actor: a missing or bad
// session is 401; a session the estate no longer knows is 401 too, because
// removal from users.yaml revokes at the next request. The resolved actor
// rides the context.
func (h *Handler) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "sign in to use this API")
			return
		}
		id, err := h.cfg.Sessions.Verify(cookie.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "sign in to use this API")
			return
		}
		actor, err := Resolve(id, h.cfg.Users, h.cfg.Groups, h.cfg.Tree)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "this identity is no longer known to the estate")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey{}, actor)))
	})
}

// providerInfo is one entry of GET /api/v1/auth/providers: enough for the
// sign-in surface to render the right control, nothing about the provider
// beyond its flow shape.
type providerInfo struct {
	Name string `json:"name"`
	Flow string `json:"flow"` // "password" | "redirect"
}

func (h *Handler) providers(w http.ResponseWriter, r *http.Request) {
	out := make([]providerInfo, 0, len(h.cfg.Providers))
	for _, p := range h.cfg.Providers {
		info := providerInfo{Name: p.Name(), Flow: "redirect"}
		if _, ok := p.(PasswordProvider); ok {
			info.Flow = "password"
		}
		out = append(out, info)
	}
	writeJSON(w, http.StatusOK, out)
}

// sameOrigin refuses a request a cross-origin page could have made.
//
// A browser form can POST to another origin without a preflight, as long as
// the content type is one a form can produce. It cannot set
// `application/json`: asking for that content type makes the request
// non-simple, which sends a preflight this server does not answer. So
// requiring the header is what makes these routes reachable only by
// something already running on this origin.
//
// The JSON decoder does not enforce it on its own. `Decode` reads one value
// and ignores what follows, so the `=` and newline a `text/plain` form
// appends to its body parse as a valid login.
//
// SameSite=Lax on the session cookie does not cover this either: SameSite
// governs when a cookie is sent, never whether a cross-site response may
// set one.
func sameOrigin(r *http.Request) bool {
	// Sec-Fetch-Site is the direct answer where the browser sends it.
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "cross-site", "same-site":
		return false
	}
	// Otherwise the content type is the gate, because a form cannot set it.
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && media == "application/json"
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		// Signing somebody's browser into an account they did not choose
		// is not a lesser thing than signing in as them: every change they
		// go on to propose is attributed to whoever owns that account.
		writeError(w, http.StatusForbidden, "sign-in must come from this instance")
		return
	}
	var req struct {
		Provider string `json:"provider"`
		Username string `json:"username"`
		Secret   string `json:"secret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "the login body is not JSON")
		return
	}
	provider, ok := h.passwordProvider(req.Provider)
	if !ok {
		writeError(w, http.StatusBadRequest, "no such password provider on this instance")
		return
	}
	id, err := provider.Authenticate(r.Context(), req.Username, req.Secret)
	if err != nil {
		if errors.Is(err, ErrBadCredentials) {
			writeError(w, http.StatusUnauthorized, ErrBadCredentials.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "sign-in failed")
		return
	}
	actor, err := Resolve(id, h.cfg.Users, h.cfg.Groups, h.cfg.Tree)
	if err != nil {
		// Authenticated but unknown to the estate: indistinguishable from
		// bad credentials on purpose: which emails exist is not an
		// unauthenticated caller's to enumerate.
		writeError(w, http.StatusUnauthorized, ErrBadCredentials.Error())
		return
	}
	if err := h.setSession(w, actor.Identity); err != nil {
		writeError(w, http.StatusInternalServerError, "sign-in failed")
		return
	}
	writeJSON(w, http.StatusOK, h.mePayload(actor))
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	// Forced sign-out is the same shape of request and is refused the same
	// way, though it costs the reader only their session.
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "sign-out must come from this instance")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: h.cfg.Secure, SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request) {
	provider, ok := h.redirectProvider(r.PathValue("provider"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such redirect provider on this instance")
		return
	}
	returnTo := r.URL.Query().Get("return_to")
	if !safeReturnTo(returnTo) {
		returnTo = "/"
	}

	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		writeError(w, http.StatusInternalServerError, "sign-in failed")
		return
	}
	state := base64.RawURLEncoding.EncodeToString(raw)

	// The verifier is drawn separately from the state, never derived from
	// it: the state travels in the redirect URL and comes back in the
	// callback query, so a verifier computed from it would be recomputable
	// by whoever holds an intercepted code, which is the attack RFC 7636
	// exists for (issue #145). Thirty-two random bytes encode to the 43
	// unreserved characters RFC 7636 §4.1 asks a verifier to be.
	rawVerifier := make([]byte, 32)
	if _, err := rand.Read(rawVerifier); err != nil {
		writeError(w, http.StatusInternalServerError, "sign-in failed")
		return
	}
	verifier := base64.RawURLEncoding.EncodeToString(rawVerifier)

	to, err := provider.Begin(r.Context(), state, verifier, h.callbackURL(r, provider.Name()))
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("the identity provider is not reachable: %v", err))
		return
	}

	// The round trip's anchor: state, verifier and return path, signed, in
	// a short-lived cookie, nothing stored server-side (ADR-0013 posture).
	// HttpOnly is what keeps the verifier a secret the browser carries but
	// no script in it can read.
	blob := stateCookieBlob(state, verifier, returnTo)
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: blob + "." + h.cfg.Sessions.sign(blob), Path: "/api/v1/auth/",
		MaxAge: 600, HttpOnly: true, Secure: h.cfg.Secure, SameSite: stateCookieSameSite(provider),
	})
	http.Redirect(w, r, to, http.StatusFound)
}

func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	provider, ok := h.redirectProvider(r.PathValue("provider"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such redirect provider on this instance")
		return
	}
	params, err := callbackParams(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	cookie, err := r.Cookie(stateCookie)
	if err != nil {
		writeError(w, http.StatusBadRequest, errStateCookie.Error())
		return
	}
	state, verifier, returnTo, err := h.verifyStateCookie(cookie.Value)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if state != returnedState(params) {
		writeError(w, http.StatusBadRequest, errStateCookie.Error())
		return
	}

	id, err := provider.Complete(r.Context(), state, verifier, h.callbackURL(r, provider.Name()), params)
	if err != nil {
		writeError(w, http.StatusUnauthorized, fmt.Sprintf("sign-in failed: %v", err))
		return
	}
	actor, err := Resolve(id, h.cfg.Users, h.cfg.Groups, h.cfg.Tree)
	if err != nil {
		// The provider vouched for them but the estate has no place for
		// them: name the fix. This is an operator conversation, not a
		// guessing game (the identity is verified, unlike login's).
		writeError(w, http.StatusForbidden, fmt.Sprintf("%s signed in %q, but %s has no user with that email. Ask an estate owner to add one", provider.Name(), id.Email, UsersFile))
		return
	}
	if err := h.setSession(w, actor.Identity); err != nil {
		writeError(w, http.StatusInternalServerError, "sign-in failed")
		return
	}
	// The state cookie is spent.
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: "", Path: "/api/v1/auth/", MaxAge: -1,
		HttpOnly: true, Secure: h.cfg.Secure, SameSite: stateCookieSameSite(provider),
	})
	http.Redirect(w, r, returnTo, http.StatusFound)
}

// mePayload is GET /api/v1/me: the signed-in actor plus the teams their
// authoring actions cover. The console offers actions exactly on objects
// owned inside editableTeams (ADR-0019 §2 derived from ADR-0016/0017).
type mePayload struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Team          string   `json:"team"`
	EditableTeams []string `json:"editableTeams"`

	// Operator says whether this actor may activate an imported version.
	// It is derived from the tree like every other permission (ADR-0019
	// §2): an operator is an actor at a root, whose horizon is the whole
	// Estate, because that is what an activation changes.
	Operator bool `json:"operator"`
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	actor, ok := ActorFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "sign in to use this API")
		return
	}
	writeJSON(w, http.StatusOK, h.mePayload(actor))
}

func (h *Handler) mePayload(actor Actor) mePayload {
	payload := mePayload{
		ID:            actor.Identity.Email,
		Name:          actor.Identity.Name,
		Team:          string(actor.Team),
		EditableTeams: []string{},
	}
	if team, known := h.cfg.Tree.Teams[actor.Team]; known {
		payload.Operator = team.Parent == ""
	}
	teams, err := actor.ActionableTeams(h.cfg.Tree)
	if err != nil {
		// Resolve vouched for the team; an error here means the tree
		// changed under us, so offer nothing rather than guess.
		return payload
	}
	for _, t := range teams {
		payload.EditableTeams = append(payload.EditableTeams, string(t))
	}
	return payload
}

func (h *Handler) setSession(w http.ResponseWriter, id Identity) error {
	token, err := h.cfg.Sessions.Issue(id)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/",
		HttpOnly: true, Secure: h.cfg.Secure, SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// stateCookieVersion tags the state cookie's payload. The tag is what
// makes a cookie an earlier build wrote recognisable rather than merely
// short: a sign-in already in flight across a deploy is refused for what
// it is, instead of being read under a layout it was never written in.
const stateCookieVersion = "v2"

// stateCookieBlob encodes the round trip's anchor as the signed payload
// version|state|verifier|returnTo.
//
// The pipe is safe against the values it separates because only the last
// of them can contain one. The version is this package's constant, and
// the state and the verifier are base64url, whose alphabet is A to Z,
// a to z, 0 to 9, "-" and "_". So the three separators this function
// writes are the first three pipes in the payload, whatever the return
// path holds, and verifyStateCookie's three left-to-right cuts land on
// exactly them: the return path takes the remainder verbatim, pipes and
// all, and no value can smuggle a field boundary into another.
func stateCookieBlob(state, verifier, returnTo string) string {
	return stateCookieVersion + "|" + state + "|" + verifier + "|" + returnTo
}

// errStateCookie is what every unusable state cookie says: absent,
// unsigned, malformed, or outlived by its own ten-minute life. They read
// as one message on purpose, the way a failed session does: which check
// refused the cookie is nothing a forger needs.
var errStateCookie = fmt.Errorf("this sign-in attempt did not start here, or took longer than ten minutes")

// errStateCookieOldFormat is the deploy-window case: a payload this
// instance signed, so it is genuinely one of ours, but written before the
// verifier joined the cookie. It cannot be completed, because the verifier
// the authorization request committed to did not survive the restart, so
// it fails closed and says why rather than reading the older layout as if
// it were this one.
var errStateCookieOldFormat = fmt.Errorf("this sign-in started before the server was updated, so it can no longer be completed: sign in again")

// verifyStateCookie checks the signature and takes the payload apart.
// Every field is judged before any of them is returned, so a payload that
// is malformed in any way is refused whole rather than half-read.
func (h *Handler) verifyStateCookie(value string) (state, verifier, returnTo string, err error) {
	blob, sig, found := strings.Cut(value, ".")
	if !found || !hmac.Equal([]byte(h.cfg.Sessions.sign(blob)), []byte(sig)) {
		return "", "", "", errStateCookie
	}
	version, rest, found := strings.Cut(blob, "|")
	if !found {
		return "", "", "", errStateCookie
	}
	if version != stateCookieVersion {
		// The signature already vouched for the payload, so a tag that is
		// not this one is a cookie an older build of this instance wrote.
		return "", "", "", errStateCookieOldFormat
	}
	state, rest, found = strings.Cut(rest, "|")
	if !found || state == "" {
		return "", "", "", errStateCookie
	}
	verifier, returnTo, found = strings.Cut(rest, "|")
	if !found || verifier == "" || !safeReturnTo(returnTo) {
		return "", "", "", errStateCookie
	}
	return state, verifier, returnTo, nil
}

func (h *Handler) callbackURL(r *http.Request, provider string) string {
	if base := strings.TrimRight(h.cfg.ExternalURL, "/"); base != "" {
		return fmt.Sprintf("%s/api/v1/auth/%s/callback", base, provider)
	}
	scheme := "http"
	if h.cfg.Secure || r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/api/v1/auth/%s/callback", scheme, r.Host, provider)
}

func (h *Handler) passwordProvider(name string) (PasswordProvider, bool) {
	for _, p := range h.cfg.Providers {
		pp, ok := p.(PasswordProvider)
		if ok && (name == "" || name == p.Name()) {
			return pp, true
		}
	}
	return nil, false
}

func (h *Handler) redirectProvider(name string) (RedirectProvider, bool) {
	for _, p := range h.cfg.Providers {
		if rp, ok := p.(RedirectProvider); ok && name == p.Name() {
			return rp, true
		}
	}
	return nil, false
}

// callbackParams is what the identity provider sent back, whichever way it
// sent it: the query of a top-level navigation, or the body of the form it
// asked the browser to submit here.
func callbackParams(r *http.Request) (url.Values, error) {
	if r.Method != http.MethodPost {
		return r.URL.Query(), nil
	}
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("the callback body is not a form")
	}
	return r.PostForm, nil
}

// returnedState is the round trip's anchor coming back, under whichever
// name the binding gives it: an authorization code flow echoes `state`,
// and an assertion carries the same value as `RelayState`. One value, two
// spellings, and the cookie is what either is judged against.
func returnedState(params url.Values) string {
	if state := params.Get("state"); state != "" {
		return state
	}
	return params.Get("RelayState")
}

// postsCallback reports whether this provider returns people here by a
// cross-site form post.
func postsCallback(p Provider) bool {
	pc, ok := p.(PostCallbackProvider)
	return ok && pc.PostsCallback()
}

// stateCookieSameSite is how far the state cookie has to travel. Lax is
// the default and covers a top-level navigation back from an issuer. A
// cross-site form post is not a navigation the browser will send a Lax
// cookie on, so a provider that returns people that way needs None, which
// browsers only honour together with Secure; NewHandler refuses such a
// provider on a deployment that is not behind TLS.
func stateCookieSameSite(p Provider) http.SameSite {
	if postsCallback(p) {
		return http.SameSiteNoneMode
	}
	return http.SameSiteLaxMode
}

// safeReturnTo admits only same-origin absolute paths: the open-redirect
// guard on the sign-in round trip.
func safeReturnTo(p string) bool {
	return strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") && !strings.HasPrefix(p, "/\\")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
