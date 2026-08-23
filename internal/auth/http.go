package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/ownership"
)

// Handler serves the auth slice of the documented platform API
// (console/README.md): how sign-in works on this instance, the sign-in
// round trips themselves, and /api/v1/me — the signed-in actor as the
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

	// Secure marks the cookies Secure — the behind-TLS deployment shape.
	Secure bool
}

// NewHandler validates the wiring and builds the routes.
func NewHandler(cfg HandlerConfig) (*Handler, error) {
	if len(cfg.Providers) == 0 {
		return nil, fmt.Errorf("an instance with no authentication provider signs nobody in (REQ-017)")
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
			return nil, fmt.Errorf("provider %q satisfies neither flow facet", p.Name())
		}
	}

	h := &Handler{cfg: cfg, mux: http.NewServeMux()}
	h.mux.HandleFunc("GET /api/v1/auth/providers", h.providers)
	h.mux.HandleFunc("POST /api/v1/auth/login", h.login)
	h.mux.HandleFunc("POST /api/v1/auth/logout", h.logout)
	h.mux.HandleFunc("GET /api/v1/auth/{provider}/start", h.start)
	h.mux.HandleFunc("GET /api/v1/auth/{provider}/callback", h.callback)
	h.mux.Handle("GET /api/v1/me", h.Require(http.HandlerFunc(h.me)))
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
// session is 401; a session the estate no longer knows is 401 too —
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
		actor, err := Resolve(id, h.cfg.Users, h.cfg.Tree)
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

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
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
	actor, err := Resolve(id, h.cfg.Users, h.cfg.Tree)
	if err != nil {
		// Authenticated but unknown to the estate: indistinguishable from
		// bad credentials on purpose — which emails exist is not an
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

	// The PKCE verifier is separate random material so it is never derivable
	// from the state that travels in the redirect URL (RFC 7636 §4.1).
	vraw := make([]byte, 32)
	if _, err := rand.Read(vraw); err != nil {
		writeError(w, http.StatusInternalServerError, "sign-in failed")
		return
	}
	verifier := base64.RawURLEncoding.EncodeToString(vraw)

	to, err := provider.Begin(r.Context(), state, verifier, h.callbackURL(r, provider.Name()))
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("the identity provider is not reachable: %v", err))
		return
	}

	// The round trip's CSRF anchor: state, PKCE verifier and return path,
	// signed, in a short-lived HttpOnly cookie — nothing stored server-side
	// (ADR-0013 posture). The verifier must stay out of URLs, hence the
	// cookie; HttpOnly keeps it out of scripts too. returnTo is last so that
	// a pipe in a safe return path cannot corrupt the parse: state and
	// verifier are base64url and can never contain a pipe, so two left-to-
	// right Cuts are unambiguous regardless of what returnTo contains.
	blob := state + "|" + verifier + "|" + returnTo
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: blob + "." + h.cfg.Sessions.sign(blob), Path: "/api/v1/auth/",
		MaxAge: 600, HttpOnly: true, Secure: h.cfg.Secure, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, to, http.StatusFound)
}

func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	provider, ok := h.redirectProvider(r.PathValue("provider"))
	if !ok {
		writeError(w, http.StatusNotFound, "no such redirect provider on this instance")
		return
	}
	cookie, err := r.Cookie(stateCookie)
	if err != nil {
		writeError(w, http.StatusBadRequest, "this sign-in attempt did not start here, or took longer than ten minutes")
		return
	}
	state, returnTo, verifier, ok := h.verifyStateCookie(cookie.Value)
	if !ok || state != r.URL.Query().Get("state") {
		writeError(w, http.StatusBadRequest, "this sign-in attempt did not start here, or took longer than ten minutes")
		return
	}

	id, err := provider.Complete(r.Context(), state, verifier, h.callbackURL(r, provider.Name()), r.URL.Query())
	if err != nil {
		writeError(w, http.StatusUnauthorized, fmt.Sprintf("sign-in failed: %v", err))
		return
	}
	actor, err := Resolve(id, h.cfg.Users, h.cfg.Tree)
	if err != nil {
		// The provider vouched for them but the estate has no place for
		// them: name the fix — this is an operator conversation, not a
		// guessing game (the identity is verified, unlike login's).
		writeError(w, http.StatusForbidden, fmt.Sprintf("%s authenticated %q, but no user in %s carries that email — ask an estate owner to add one", provider.Name(), id.Email, UsersFile))
		return
	}
	if err := h.setSession(w, actor.Identity); err != nil {
		writeError(w, http.StatusInternalServerError, "sign-in failed")
		return
	}
	// The state cookie is spent.
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: "", Path: "/api/v1/auth/", MaxAge: -1,
		HttpOnly: true, Secure: h.cfg.Secure, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, returnTo, http.StatusFound)
}

// mePayload is GET /api/v1/me: the signed-in actor plus the teams their
// authoring actions cover — the console offers actions exactly on objects
// owned inside editableTeams (ADR-0019 §2 derived from ADR-0016/0017).
type mePayload struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Team          string   `json:"team"`
	EditableTeams []string `json:"editableTeams"`
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
	teams, err := actor.ActionableTeams(h.cfg.Tree)
	if err != nil {
		// Resolve vouched for the team; an error here means the tree
		// changed under us — offer nothing rather than guess.
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

func (h *Handler) verifyStateCookie(value string) (state, returnTo, verifier string, ok bool) {
	blob, sig, found := strings.Cut(value, ".")
	if !found || !hmac.Equal([]byte(h.cfg.Sessions.sign(blob)), []byte(sig)) {
		return "", "", "", false
	}
	// Blob is state|verifier|returnTo. state and verifier are base64url, so
	// two left-to-right Cuts are unambiguous: returnTo takes the remainder
	// and may contain pipes without corrupting the parse.
	state, rest, found := strings.Cut(blob, "|")
	if !found || state == "" {
		return "", "", "", false
	}
	verifier, returnTo, found = strings.Cut(rest, "|")
	if !found || verifier == "" || !safeReturnTo(returnTo) {
		return "", "", "", false
	}
	return state, returnTo, verifier, true
}

func (h *Handler) callbackURL(r *http.Request, provider string) string {
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
