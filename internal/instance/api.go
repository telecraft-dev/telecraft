package instance

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/console"
)

// routes builds the whole HTTP surface: the two probes, the auth slice open
// to the signed-out, the rest of the API behind Require, and the console
// bundle under everything else.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// The probes sit outside /api/v1/, unauthenticated, and answer a
	// status word and nothing else: no estate content passes an
	// unauthenticated route (ADR-0067 §6).
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("GET /readyz", s.readyz)

	// The auth slice, open to the signed out: how sign-in works here, the
	// round trips themselves, and the signed-in actor. The handler behind
	// it answers its own unknown paths, so an unknown auth route is a 404
	// with a JSON body like every other one.
	mux.Handle("/api/v1/auth/", http.HandlerFunc(s.serveAuth))
	mux.Handle("GET /api/v1/me", http.HandlerFunc(s.serveAuth))

	api := http.NewServeMux()
	for path, answer := range documentRoutes() {
		api.Handle("GET "+path, s.document(answer))
	}
	for path, method := range unanswered {
		api.Handle(method+" "+path, http.HandlerFunc(s.refuse))
	}
	api.HandleFunc("/api/v1/", notFound)
	mux.Handle("/api/v1/", s.require(api))

	mux.Handle("/", s.console())
	return mux
}

// documentRoutes is every read endpoint this server answers, each with the
// projection that answers it from the computed documents (console/README.md
// holds the contract).
func documentRoutes() map[string]func(*console.Bundle, url.Values) (any, bool) {
	return map[string]func(*console.Bundle, url.Values) (any, bool){
		"/api/v1/objects":            objectsDoc,
		"/api/v1/estate":             estateDoc,
		"/api/v1/drawer":             drawerDoc,
		"/api/v1/collectors":         collectorsDoc,
		"/api/v1/topology":           topologyDoc,
		"/api/v1/rollouts":           rolloutsDoc,
		"/api/v1/blueprints":         blueprintsDoc,
		"/api/v1/catalogue":          catalogueDoc,
		"/api/v1/catalogue/versions": catalogueVersionsDoc,
		"/api/v1/catalogue/entries":  catalogueEntriesDoc,
		"/api/v1/activations":        activationsDoc,
		"/api/v1/governance":         governanceDoc,
		"/api/v1/endorsements":       endorsementsDoc,
	}
}

// unanswered is every documented endpoint this build routes but does not
// answer: the change proposals, which leave through the forge adapter, and
// the three evaluators the composing surfaces call. They are routed rather
// than left to the not-found answer, so a caller reaching one is told what
// this instance does not do rather than that the path does not exist.
var unanswered = map[string]string{
	"/api/v1/validate":              http.MethodPost,
	"/api/v1/proposals":             http.MethodPost,
	"/api/v1/claims/preview":        http.MethodPost,
	"/api/v1/claims":                http.MethodPost,
	"/api/v1/governance/proposals":  http.MethodPost,
	"/api/v1/tiers/proposals":       http.MethodPost,
	"/api/v1/activations/proposals": http.MethodPost,
	"/api/v1/setup":                 http.MethodGet,
}

// signInUnavailable is what an instance answers before it has read who may
// sign in, and after a read that failed. The cause is on the operator's
// terminal, where an operator is; what a caller needs is that signing in is
// not possible here yet.
const signInUnavailable = "this instance cannot sign anyone in yet"

// documentsUnavailable is what an instance answers before it has computed a
// document set. It is not an empty estate, and it must not read as one.
const documentsUnavailable = "this instance has not read the estate yet"

// serveAuth hands the request to the auth handler at the current head. The
// handler is rebuilt whenever the estate's users, teams or providers
// change, so this reads the pointer per request rather than closing over
// one handler for the life of the process.
func (s *Server) serveAuth(w http.ResponseWriter, r *http.Request) {
	handler := s.authz.Load()
	if handler == nil {
		writeError(w, http.StatusServiceUnavailable, signInUnavailable)
		return
	}
	handler.ServeHTTP(w, r)
}

// require gates next behind a signed-in, resolved actor, through the auth
// handler at the current head.
func (s *Server) require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handler := s.authz.Load()
		if handler == nil {
			writeError(w, http.StatusServiceUnavailable, signInUnavailable)
			return
		}
		handler.Require(next).ServeHTTP(w, r)
	})
}

// document answers one read endpoint from the memoised documents. A server
// that has not computed a set yet says so rather than answering an empty
// estate, which would read as an estate with nothing in it.
func (s *Server) document(answer func(*console.Bundle, url.Values) (any, bool)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bundle := s.docs.Load()
		if bundle == nil {
			writeError(w, http.StatusServiceUnavailable, documentsUnavailable)
			return
		}
		payload, ok := answer(bundle, r.URL.Query())
		if !ok {
			// Say "cannot know", never fabricate: a version or a Tier this
			// estate does not have is a 404, not an empty answer.
			writeError(w, http.StatusNotFound, "nothing on this estate answers to that")
			return
		}
		writeJSON(w, http.StatusOK, payload)
	})
}

// refuse says what this instance does not answer, in the words the console
// puts in front of a reader.
func (s *Server) refuse(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "this instance does not answer "+r.URL.Path+" yet")
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeWord(w, http.StatusOK, "ok")
}

// readyz is 503 until the first snapshot is held and 200 after it. A
// refresh that fails later keeps the last one and readiness stays green: a
// stale head serves correct configuration for the commit it names, and
// delivery must not stop because a fetch failed.
func (s *Server) readyz(w http.ResponseWriter, _ *http.Request) {
	if s.head.Load() == nil {
		writeWord(w, http.StatusServiceUnavailable, "starting")
		return
	}
	writeWord(w, http.StatusOK, "ready")
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "no such endpoint: "+r.URL.Path)
}

func writeWord(w http.ResponseWriter, status int, word string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(word + "\n"))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// secureCookies reads the external URL's scheme: what is behind TLS gets
// cookies marked Secure, and what is on a loopback address over plain HTTP
// does not, because a browser would then drop them.
func secureCookies(external string) bool {
	return strings.HasPrefix(strings.ToLower(external), "https://")
}
