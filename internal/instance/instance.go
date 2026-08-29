// Package instance is the Instance server (ADR-0067): one process serving
// one estate to the humans who govern it and to the collectors it
// configures.
//
// From one estate source it serves the embedded console bundle with a
// single-page fallback, the documented platform API under /api/v1/, the
// authentication round trips under /api/v1/auth/, and the OpAMP endpoint
// the serving path already owned. Everything under /api/v1/ that is not an
// auth route is mounted behind auth.Handler's Require, which is the gate
// that package was written to be (REQ-017, ADR-0019).
//
// One process, because the Served reading is this process's own wire: the
// OpAMP-direct EstateProvider taps the same server that serves the
// artefacts (ADR-0008), so the API projects its documents from the head the
// matcher is serving and the console cannot report a head the server is
// not on. Humans and collectors arrive on separate addresses, so a
// deployment can expose one and not the other.
//
// The process holds no certificate. Both endpoints speak plain HTTP and TLS
// terminates in front (ADR-0067 §5); the external URL declares what the
// outside sees, and the server refuses to start when that URL names a
// non-loopback host over plain HTTP unless the operator says they mean it.
//
// Storage is ADR-0032's closed list, unchanged. What this process holds is
// the repo snapshot and its selector index, the per-connection layer-1
// digest, the per-connection reading the estate provider keeps, and one
// loseable memo of the API documents, which are a pure function of the
// snapshot and the live readings (ADR-0038). Sessions are signed and
// verified, never looked up. TestStorageInventoryIsTheClosedList holds this
// shape here the way it holds it over the serving path.
package instance

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/telecraft-dev/telecraft/internal/console"
	estateprovider "github.com/telecraft-dev/telecraft/internal/provider/estate"
	"github.com/telecraft-dev/telecraft/internal/readings"
	"github.com/telecraft-dev/telecraft/internal/serving"
	"github.com/telecraft-dev/telecraft/internal/telemetry"
	"github.com/telecraft-dev/telecraft/pkg/auth"
	"github.com/telecraft-dev/telecraft/pkg/forge"
	"github.com/telecraft-dev/telecraft/pkg/licence"
)

// DefaultWindow is the trailing window the arrival readings cover when the
// deployment names none.
const DefaultWindow = 15 * time.Minute

// Config configures one Instance server. Source, Root and HTTPEndpoint are
// required.
type Config struct {
	// Source supplies the repo snapshot: a checkout on disk, or a git URL
	// polled on the fetch interval (ADR-0032 §3).
	Source serving.Source

	// Root is where that source's estate sits on disk: the checkout
	// itself, or the directory the clone is kept in. The API reads its
	// documents from here, at the head the source last reported.
	Root string

	// OpAMPEndpoint is the host:port collectors reach, at /v1/opamp. Empty
	// closes it, which is the shape of an Instance whose collectors are
	// all Foreign.
	OpAMPEndpoint string

	// HTTPEndpoint is the host:port humans reach: the console, the API and
	// the two probes. It is always open, because it carries the probes.
	HTTPEndpoint string

	// ExternalURL is the URL the Instance is reached at. Its scheme
	// decides whether session cookies are marked Secure, and it is the
	// base the redirect round trip is built from.
	ExternalURL string

	// InsecureHTTP admits an external URL that names a non-loopback host
	// over plain HTTP. Without it that combination refuses to start.
	InsecureHTTP bool

	// FetchInterval is the snapshot poll; zero means the serving default.
	FetchInterval time.Duration

	// Window is the trailing window the arrival readings cover; zero means
	// DefaultWindow.
	Window time.Duration

	// Sessions signs and verifies the browser session. The zero value is
	// refused: a key is drawn by the caller, so the caller can say whether
	// it survives a restart.
	Sessions auth.Sessions

	// Secrets resolves the material the estate names, from the directory
	// the deployment filled. Nil resolves nothing, which refuses to start
	// an Instance whose estate names any.
	Secrets auth.Secrets

	// RefuseBasicAuth refuses basic auth whatever the estate declares. It
	// is bootstrap and break-glass for an operator who can reach a shell
	// on the host, so a deployment run for people the operator is not
	// refuses it and says so (ADR-0072 §6).
	RefuseBasicAuth bool

	// LicenceFile is the licence the deployment placed, or empty for the
	// Standard Edition, which is the ordinary case and needs no file. The
	// file is read at start and again whenever it changes, and what it
	// says is reported and never enforced here: no licence state reaches
	// the renderer, the OpAMP endpoint, the readiness probe or the
	// artefact a Served collector fetches (ADR-0070 §4).
	LicenceFile string

	// Telemetry is the arrivals seam. Nil takes no arrival reading, which
	// the console renders as not known with the cause said out loud.
	Telemetry telemetry.Provider

	// RefreshKey reads the key a bare refresh request presents: the
	// deployment's own material, read at each use so that rewriting the
	// file is the whole of rotating it (ADR-0071 §5). Nil, or a key that
	// reads empty, takes no bare refresh request.
	RefreshKey func() (string, error)

	// PushSecret reads the secret the forge signs its deliveries with,
	// on the same terms. Nil, or a secret that reads empty, takes no push
	// notification.
	PushSecret func() (string, error)

	// Forge is where a change proposal leaves through (ADR-0028 §4). Nil
	// is an Instance with no forge credential: the read surface is
	// unaffected and every exit that would propose a change says what is
	// missing rather than failing (ADR-0067 §1).
	Forge forge.Forge

	// Notifications judges a delivery: whether it is a genuine push from
	// the forge this estate is read from. Nil is an estate with no forge
	// behind it, such as a repository reached over the git transport
	// alone, and the bare request is what serves one.
	Notifications forge.Notifications

	// Logf receives operational one-liners. Nil discards them.
	Logf func(format string, args ...any)

	// Now is the clock readings are stamped with; nil means time.Now.
	Now func() time.Time
}

// Server is the Instance server: wiring, the two readers on the serving
// wire, and three rebuildable holdings. Any field added under "storage"
// below must be derivable from git plus live connections, or it is a design
// regression requiring an ADR-0032 amendment.
type Server struct {
	// Wiring: configuration, the listeners, and the two readers on the
	// serving wire. What the readers hold is per-connection and dies with
	// the connection, which is what the closed list permits an off-path
	// reader (ADR-0032 §1).
	cfg        Config
	logf       func(format string, args ...any)
	opamp      *serving.Server
	web        *http.Server
	collectors *estateprovider.OpAMPDirect
	delivery   *readings.DeliveryPaths
	composer   *readings.Composer
	stopPoll   context.CancelFunc
	pollDone   chan struct{}

	// The sign-in a loopback Instance mints for itself when the estate
	// names nobody (ADR-0082). Drawn once because refreshAuth runs on
	// every poll, and said once because a password repeated into a log
	// every thirty seconds is a password in a log.
	bootstrapOnce   sync.Once
	bootstrapSaid   sync.Once
	bootstrapSecret string

	// nudge asks the poll to run now. It is one buffered slot, so a burst
	// of asking coalesces into one fetch, and it holds nothing: what is
	// in it is the fact that somebody asked, and it dies with the process.
	nudge chan struct{}

	// Storage, and every item of it is rebuildable:
	//   1. the head the source last reported, which is a name for the repo
	//      snapshot the serving path holds (ADR-0032 §1 item 1);
	head atomic.Pointer[string]
	//   2. the API documents at that head, a pure function of the snapshot
	//      and the live readings, memoised loseably (ADR-0038): losing
	//      them costs one recomputation and no record;
	docs atomic.Pointer[console.Bundle]
	//   3. the auth handler over the estate's own users, teams and
	//      providers, rebuilt whenever the head moves, so removing a user
	//      revokes them at their next request;
	authz atomic.Pointer[auth.Handler]
	//   4. the licence standing at the file's current contents, a pure
	//      function of that file, the keys in this binary and the clock
	//      (ADR-0070 §4): losing it costs one re-read and no record;
	licence atomic.Pointer[licence.Standing]
	//   5. nothing else.
}

// New validates the configuration and builds the server. Nothing is
// fetched, opened or read until Start.
func New(cfg Config) (*Server, error) {
	if cfg.Source == nil {
		return nil, errors.New("no source: the server needs an estate to serve")
	}
	if cfg.Root == "" {
		return nil, errors.New("no estate root: the API reads its documents from the checkout the source keeps")
	}
	if cfg.HTTPEndpoint == "" {
		return nil, errors.New("no HTTP endpoint: the console, the API and the probes have nowhere to listen")
	}
	if cfg.ExternalURL == "" {
		cfg.ExternalURL = "http://" + cfg.HTTPEndpoint
	}
	if err := checkExternalURL(cfg.ExternalURL, cfg.InsecureHTTP); err != nil {
		return nil, err
	}
	if cfg.Window == 0 {
		cfg.Window = DefaultWindow
	}
	if cfg.FetchInterval == 0 {
		cfg.FetchInterval = serving.DefaultFetchInterval
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}

	s := &Server{
		cfg:        cfg,
		logf:       cfg.Logf,
		nudge:      make(chan struct{}, 1),
		collectors: estateprovider.NewOpAMPDirect(estateprovider.OpAMPDirectConfig{Now: cfg.Now}),
		delivery:   &readings.DeliveryPaths{},
	}
	s.composer = &readings.Composer{
		Collectors: s.collectors,
		Delivery:   s.delivery,
		Telemetry:  cfg.Telemetry,
		Window:     cfg.Window,
		Now:        cfg.Now,
	}

	if cfg.OpAMPEndpoint != "" {
		opamp, err := serving.New(serving.Config{
			Source:         cfg.Source,
			ListenEndpoint: cfg.OpAMPEndpoint,
			FetchInterval:  cfg.FetchInterval,
			Logf:           cfg.Logf,
			Tap:            serving.Taps{s.collectors, s.delivery},
			OnSnapshot:     s.observe,
		})
		if err != nil {
			return nil, err
		}
		s.opamp = opamp
	}

	s.web = &http.Server{Addr: cfg.HTTPEndpoint, Handler: s.routes()}
	return s, nil
}

// Start opens both endpoints and begins the poll. It returns once the
// listeners are accepting, which is before the first documents exist:
// readiness is what says when those have landed.
func (s *Server) Start(ctx context.Context) error {
	if s.opamp != nil {
		// The serving path takes the first snapshot itself, and refuses to
		// start without one: serving cannot begin on an estate it could
		// not read.
		if err := s.opamp.Start(ctx); err != nil {
			return err
		}
	} else if snap, err := s.cfg.Source.Snapshot(ctx); err != nil {
		return fmt.Errorf("initial repo snapshot: %w", err)
	} else {
		s.observe(snap)
	}

	// The licence is read before anything is served, so the first surface
	// anybody meets names the Edition it is actually running. A file that
	// was not accepted is loud here and stops nothing: the server starts
	// whatever the licence says and whatever it fails to say.
	s.readLicence()

	// Who may sign in, how, and with what material is read before
	// anything is served. The estate is reviewed and it asserts what this
	// Instance offers, so serving something narrower than the reviewed
	// estate, because a users file was unreadable or a named secret was
	// not placed, would be the Instance lying about its own configuration
	// (ADR-0071 §4).
	if err := s.refreshAuth(); err != nil {
		return err
	}

	listener, err := net.Listen("tcp", s.cfg.HTTPEndpoint)
	if err != nil {
		return err
	}
	s.web.Addr = listener.Addr().String()
	go func() {
		if err := s.web.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logf("the HTTP endpoint stopped: %v", err)
		}
	}()

	pollCtx, cancel := context.WithCancel(context.Background())
	s.stopPoll = cancel
	s.pollDone = make(chan struct{})
	go s.pollLoop(pollCtx)
	return nil
}

// Stop closes both endpoints and ends the poll. Everything held dies here
// by design: the documents recompute, the snapshot re-fetches, and every
// session is signed rather than stored, so a restart signs people out and
// loses no record.
func (s *Server) Stop(ctx context.Context) error {
	if s.stopPoll != nil {
		s.stopPoll()
		<-s.pollDone
	}
	err := s.web.Shutdown(ctx)
	if s.opamp != nil {
		if stopErr := s.opamp.Stop(ctx); err == nil {
			err = stopErr
		}
	}
	return err
}

// HTTPAddr is the address the HTTP endpoint is listening on; empty before
// Start. It lets a caller listen on port 0 and discover the port.
func (s *Server) HTTPAddr() string { return s.web.Addr }

// OpAMPAddr is the address the OpAMP endpoint is listening on; nil when
// the endpoint is closed or Start has not run.
func (s *Server) OpAMPAddr() net.Addr {
	if s.opamp == nil {
		return nil
	}
	return s.opamp.Addr()
}

// observe records the head the source reported. It runs on the serving
// path's own refresh goroutine, so it does no work beyond the pointer swap:
// the documents are rebuilt by the poll below.
func (s *Server) observe(snap *serving.Snapshot) {
	commit := snap.Commit
	s.head.Store(&commit)
}

// pollLoop rebuilds the documents. It runs on the fetch interval alongside
// the serving path's own poll, because the documents depend on the live
// readings as well as on the head: an estate that has not moved still has
// collectors arriving and leaving.
//
// When the OpAMP endpoint is closed there is no serving path to poll the
// source, so this loop does it: one fetch either way (ADR-0067 §2).
func (s *Server) pollLoop(ctx context.Context) {
	defer close(s.pollDone)
	s.refresh(ctx)
	ticker := time.NewTicker(s.cfg.FetchInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.readLicence()
			// The serving path polls the source on its own interval
			// where the OpAMP endpoint is open, so asking again here
			// would fetch twice for one interval.
			if s.opamp == nil {
				s.fetch(ctx)
			}
			s.refresh(ctx)
		case <-s.nudge:
			// Somebody said the estate moved. The poll is unchanged and
			// still running, so this costs one fetch and saves waiting
			// for the next tick.
			s.fetch(ctx)
			s.refresh(ctx)
		}
	}
}

// fetch brings the source to its current head. The serving path owns the
// fetch where there is one, because it is the process's one connection to
// the estate; where the OpAMP endpoint is closed there is nothing else to
// do it, so this loop does (ADR-0067 §2).
func (s *Server) fetch(ctx context.Context) {
	var err error
	if s.opamp != nil {
		err = s.opamp.Refresh(ctx)
	} else {
		var snap *serving.Snapshot
		if snap, err = s.cfg.Source.Snapshot(ctx); err == nil {
			s.observe(snap)
		}
	}
	if err != nil {
		s.logf("repo snapshot refresh failed, keeping the previous head: %v", err)
	}
}

// Nudge asks the poll to fetch and recompute now, and returns without
// waiting for it. A nudge already waiting is the whole of the coalescing:
// a burst of them is one fetch, and none of it survives the process.
func (s *Server) Nudge() {
	select {
	case s.nudge <- struct{}{}:
	default:
	}
}

// refresh recomputes the documents and the auth wiring at the current head.
// A failure keeps the previous answers and says why: a stale document set
// describes the commit it names, and the console must not go dark because
// one recomputation failed (ADR-0010's discipline, applied to the read
// path).
func (s *Server) refresh(ctx context.Context) {
	if s.head.Load() == nil {
		return
	}
	if err := s.refreshAuth(); err != nil {
		s.logf("reading the estate's users and providers failed, keeping the previous ones: %v", err)
	}
	bundle, err := s.build(ctx)
	if err != nil {
		s.logf("rebuilding the API documents failed, keeping the previous ones: %v", err)
		return
	}
	s.docs.Store(&bundle)
}

// readLicence re-reads the licence file and holds what it says. The check
// runs at start and again on the poll, so a file the deployment replaced
// takes effect without a restart, and the result is held in memory and
// dies with the process.
//
// A change is one line on the operator's terminal, and a file that was not
// accepted names itself and what is wrong with it. Nothing else happens: a
// licence never stops this process starting, serving, or answering a
// probe.
func (s *Server) readLicence() {
	standing := licence.Read(s.cfg.LicenceFile)
	previous := s.licence.Load()
	s.licence.Store(&standing)
	if previous != nil && previous.Same(standing) {
		return
	}
	if standing.State == licence.Unreadable {
		s.logf("the licence file %s was not accepted: %s", standing.Path, standing.Problem)
	}
	if standing.State != licence.Absent || previous != nil {
		s.logf("this instance is running %s", standing.Report())
	}
}

// checkExternalURL fails closed on the one combination that would put a
// password on a network in clear text (ADR-0067 §5). Everything else the
// operator declares is theirs to declare.
func checkExternalURL(raw string, insecure bool) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("the external URL %q is not a URL. Name the address a browser reaches this instance at, like https://telecraft.example", raw)
	}
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
	default:
		return fmt.Errorf("the external URL %q names the scheme %q. It is http or https", raw, parsed.Scheme)
	}
	if insecure || isLoopback(parsed.Hostname()) {
		return nil
	}
	return fmt.Errorf("the external URL %q sends passwords and sessions across a network in clear text. Terminate TLS in front and name the https URL, or pass -insecure-http to say that plain HTTP is meant here", raw)
}

// isLoopback reports whether a host is one nothing sits between: a loopback
// address, or the name for one.
func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
