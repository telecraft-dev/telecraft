package instance

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/telecraft-dev/telecraft/internal/console"
	"github.com/telecraft-dev/telecraft/internal/licence"
	"github.com/telecraft-dev/telecraft/pkg/forge"
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

	// The refresh endpoint is asked for by machines, so it sits outside
	// the session gate and carries its own: a delivery the forge signed,
	// or the key the deployment placed. Nothing about a session would help
	// a repository's own hook, which has none.
	mux.Handle("POST /api/v1/refresh", http.HandlerFunc(s.serveRefresh))

	api := http.NewServeMux()

	// The Edition is not a reading of the estate, so it is answered from
	// the licence standing rather than from the computed documents. It
	// sits behind the same gate as everything else under /api/v1/: it is
	// a fact about the reader's session, and nobody signed out has one.
	api.Handle("GET /api/v1/edition", http.HandlerFunc(s.serveEdition))

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

// editionPayload is what /api/v1/edition answers: the Edition this
// Instance is running, and the one quiet line a surface that names the
// Edition shows.
//
// The line is composed here rather than by each surface, so the CLI, the
// operator's terminal and the console cannot disagree about what an
// Instance is running (ADR-0070 §5). What is wrong with a file that was
// not accepted is on the operator's terminal, where an operator is, and is
// not carried here.
type editionPayload struct {
	Edition   string `json:"edition"`
	Statement string `json:"statement"`
}

// serveEdition answers what this Instance is running. It answers before
// the first documents exist and after a licence file has gone: an Instance
// with no licence is Standard Edition, which is an answer rather than an
// absence.
func (s *Server) serveEdition(w http.ResponseWriter, r *http.Request) {
	standing := s.licence.Load()
	if standing == nil {
		standing = &licence.Standing{State: licence.Absent}
	}
	writeJSON(w, http.StatusOK, editionPayload{
		Edition:   string(standing.Edition()),
		Statement: standing.Report(),
	})
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

// The refresh endpoint's answers, and the most of a delivery it reads.
const (
	refreshBodyLimit = 1 << 20

	refreshUnavailable = "this instance takes no refresh requests: no refresh key and no push secret were placed"
	refreshNotAccepted = "this refresh request was not accepted"
)

// serveRefresh means "fetch now" (ADR-0073 §5). It fetches nothing itself:
// it says who asked, asks the poll to run, and answers.
//
// The payload is never believed. A refresh triggers a fetch and a
// recompute, and git says what changed, so a forged or replayed delivery
// costs one fetch and can assert nothing (ADR-0003). Nothing durable is
// added: no queue, no delivery record, and no memory of which deliveries
// arrived (ADR-0032).
//
// Two callers, which is what keeps the endpoint from being shaped like one
// forge's webhook: a push notification the forge adapter verifies against
// the secret the deployment placed, and a bare request presenting the
// refresh key, which is what a repository with no forge behind it uses.
func (s *Server) serveRefresh(w http.ResponseWriter, r *http.Request) {
	key, keyPlaced := placed(s.cfg.RefreshKey)
	secret, secretPlaced := placed(s.cfg.PushSecret)
	if !keyPlaced && !(secretPlaced && s.cfg.Notifications != nil) {
		// An absence declares the capability unavailable rather than
		// failing (ADR-0071 §4).
		writeError(w, http.StatusNotImplemented, refreshUnavailable)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, refreshBodyLimit))
	if err != nil {
		writeError(w, http.StatusBadRequest, "this refresh request could not be read")
		return
	}

	if secretPlaced && s.cfg.Notifications != nil {
		push, err := s.cfg.Notifications.Push(forge.Notification{Header: r.Header, Body: body}, secret)
		switch {
		case err != nil:
			// Worth one line: somebody wiring a webhook up needs to know
			// why nothing is happening. It is not a refusal on its own,
			// because the bare key may still be what this request carries.
			s.logf("a delivery to the refresh endpoint was not verified: %v", err)
		case push:
			s.Nudge()
			writeJSON(w, http.StatusAccepted, map[string]string{"status": "fetching"})
			return
		default:
			// Genuine, and not a push. Nothing to do, and nothing wrong.
			writeJSON(w, http.StatusAccepted, map[string]string{"status": "nothing to fetch"})
			return
		}
	}

	presented := presentedKey(r)
	if keyPlaced && presented != "" && subtle.ConstantTimeCompare([]byte(presented), []byte(key)) == 1 {
		s.Nudge()
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "fetching"})
		return
	}
	writeError(w, http.StatusUnauthorized, refreshNotAccepted)
}

// presentedKey is the key a bare request carries, as an ordinary bearer
// credential.
func presentedKey(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if len(header) < len("Bearer ") || !strings.EqualFold(header[:len("Bearer ")], "bearer ") {
		return ""
	}
	return strings.TrimSpace(header[len("Bearer "):])
}

// placed reads one of the deployment's own secrets at the moment it is
// needed, which is what makes rotating it writing the file. A secret
// nothing answers is an absence, and an absence declares.
func placed(read func() (string, error)) (string, bool) {
	if read == nil {
		return "", false
	}
	value, err := read()
	if err != nil || value == "" {
		return "", false
	}
	return value, true
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
