package serving

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/open-telemetry/opamp-go/server"
	"github.com/open-telemetry/opamp-go/server/types"
)

// DefaultFetchInterval is the default repo-snapshot poll — the bounded
// staleness of ADR-0032 §1, and with the webhook fast-path not yet built,
// the one freshness knob: "why is my merge not live yet" has a one-line
// answer.
const DefaultFetchInterval = 30 * time.Second

// serverCapabilities is what this server offers on every response: it
// accepts status, offers remote config, and accepts effective config —
// nothing more, because it stores nothing more (ADR-0013).
const serverCapabilities = uint64(protobufs.ServerCapabilities_ServerCapabilities_AcceptsStatus |
	protobufs.ServerCapabilities_ServerCapabilities_OffersRemoteConfig |
	protobufs.ServerCapabilities_ServerCapabilities_AcceptsEffectiveConfig)

// Tap observes the serving wire for readers off the serving path — the
// OpAMP-direct EstateProvider (ADR-0008) is the intended tap. The server
// calls Report for every message a collector sends, passing the identity
// attributes it flattened for matching (nil when the message carried no
// description), and Closed when the connection ends. The server stores
// nothing on a tap's behalf (ADR-0032): whatever a tap accumulates is the
// tap's own cache, derivable from live connections exactly as the closed
// list demands, and expected to die with them.
type Tap interface {
	// Report receives one collector message as it arrived. conn is an
	// opaque comparable connection key, stable for the connection's life
	// and never surfaced as identity (ADR-0013).
	Report(conn any, identity map[string]string, msg *protobufs.AgentToServer)

	// Closed marks the end of a connection: anything the tap holds under
	// this key describes a collector no longer reporting.
	Closed(conn any)
}

// Config configures one Server. Source and ListenEndpoint are required.
type Config struct {
	// Source supplies the repo snapshot: a GitSource fetching a remote, or
	// a DirSource over a local checkout (ADR-0032 §3).
	Source Source

	// ListenEndpoint is the host:port the OpAMP endpoint listens on; the
	// wire path is the protocol's default /v1/opamp, WebSocket and plain
	// HTTP both accepted.
	ListenEndpoint string

	// FetchInterval is the snapshot poll; zero means DefaultFetchInterval.
	FetchInterval time.Duration

	// Logf receives operational one-liners (refresh failures, refused
	// serves). Nil discards them.
	Logf func(format string, args ...any)

	// Tap, when non-nil, observes every collector message and connection
	// close — the door the OpAMP-direct EstateProvider reads through
	// (ADR-0008). The serving decision is unaffected by it.
	Tap Tap
}

// Server is the stateless OpAMP server (REQ-040): wiring plus the two
// caches of ADR-0032's closed list, and deliberately nothing else — any
// field added under "storage" below must be derivable from git plus live
// connections, or it is a design regression requiring an ADR-0032
// amendment. TestStorageInventoryIsTheClosedList holds this shape.
type Server struct {
	// Wiring — configuration and the wire listener, no collector data.
	source      Source
	listen      string
	interval    time.Duration
	logf        func(format string, args ...any)
	tap         Tap
	opamp       server.OpAMPServer
	stopRefresh context.CancelFunc
	refreshDone chan struct{}

	// Storage, the closed list (ADR-0032 §1):
	//   1. the repo snapshot at last-known head — loss is a re-fetch;
	snapshot atomic.Pointer[Snapshot]
	//   2. the per-connection layer-1 digest of the last-reported
	//      effective config (ADR-0005), keyed by connection, dying with
	//      it — loss is one extra parse on reconnect;
	digests sync.Map // types.Connection → [sha256.Size]byte
	//   3. nothing else.
}

// New builds a Server. Nothing is fetched or opened until Start.
func New(cfg Config) (*Server, error) {
	if cfg.Source == nil {
		return nil, errors.New("no source — the server is stateless transport over a repo snapshot, so a source is what it serves (ADR-0013)")
	}
	if cfg.ListenEndpoint == "" {
		return nil, errors.New("no listen endpoint")
	}
	if cfg.FetchInterval == 0 {
		cfg.FetchInterval = DefaultFetchInterval
	}
	if cfg.Logf == nil {
		cfg.Logf = func(string, ...any) {}
	}
	s := &Server{
		source:   cfg.Source,
		listen:   cfg.ListenEndpoint,
		interval: cfg.FetchInterval,
		logf:     cfg.Logf,
		tap:      cfg.Tap,
	}
	s.opamp = server.New(opampLogger{s.logf})
	return s, nil
}

// Start takes the initial snapshot — serving cannot begin without one —
// then opens the OpAMP endpoint and begins the refresh poll. It returns
// once the listener is accepting connections.
func (s *Server) Start(ctx context.Context) error {
	snap, err := s.source.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("initial repo snapshot: %w", err)
	}
	s.snapshot.Store(snap)

	err = s.opamp.Start(server.StartSettings{
		Settings: server.Settings{
			Callbacks: types.Callbacks{
				OnConnecting: func(*http.Request) types.ConnectionResponse {
					return types.ConnectionResponse{
						Accept: true,
						ConnectionCallbacks: types.ConnectionCallbacks{
							OnMessage:         s.onMessage,
							OnConnectionClose: s.onConnectionClose,
						},
					}
				},
			},
		},
		ListenEndpoint: s.listen,
	})
	if err != nil {
		return err
	}

	refreshCtx, cancel := context.WithCancel(context.Background())
	s.stopRefresh = cancel
	s.refreshDone = make(chan struct{})
	go s.refreshLoop(refreshCtx)

	s.logf("serving head %s on %s, fetch interval %s", headName(snap), s.Addr(), s.interval)
	return nil
}

// Stop ends the refresh poll and closes the endpoint and every connection.
// Everything held dies here by design: the snapshot re-fetches and each
// digest rebuilds from its collector's next report (ADR-0032).
func (s *Server) Stop(ctx context.Context) error {
	if s.stopRefresh != nil {
		s.stopRefresh()
		<-s.refreshDone
	}
	return s.opamp.Stop(ctx)
}

// Addr is the address the endpoint is listening on; nil before Start.
// Lets a caller listen on port 0 and discover the port.
func (s *Server) Addr() net.Addr {
	return s.opamp.Addr()
}

// refreshLoop polls the source. A failed fetch keeps the previous
// snapshot: staleness stays bounded by the next successful poll, and a
// flaky remote must never take serving down (ADR-0032 §1).
func (s *Server) refreshLoop(ctx context.Context) {
	defer close(s.refreshDone)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			snap, err := s.source.Snapshot(ctx)
			if err != nil {
				s.logf("repo snapshot refresh failed, serving the previous head: %v", err)
				continue
			}
			if prev := s.snapshot.Swap(snap); prev == nil || prev.Commit != snap.Commit {
				s.logf("repo snapshot now at head %s", headName(snap))
			}
		}
	}
}

// onMessage is the serving loop (ADR-0013): read what the collector
// reports, decide from the snapshot, remember nothing beyond the layer-1
// digest. The response always carries the server's capabilities, so any
// message doubles as the handshake.
func (s *Server) onMessage(_ context.Context, conn types.Connection, msg *protobufs.AgentToServer) *protobufs.ServerToAgent {
	resp := &protobufs.ServerToAgent{
		InstanceUid:  msg.GetInstanceUid(),
		Capabilities: serverCapabilities,
	}

	if ec := msg.GetEffectiveConfig(); ec != nil {
		digest := digestOf(ec)
		if prev, ok := s.digests.Load(conn); !ok || prev != digest {
			// Layer 1 changed: this branch is where the drift path's
			// one-parse-per-changed-collector hangs when it lands
			// (ADR-0005 layer 2). The digest itself is all that is kept.
			s.digests.Store(conn, digest)
		}
	}

	desc := msg.GetAgentDescription()
	var attrs map[string]string
	if desc != nil {
		attrs = attributesOf(desc)
	}
	if s.tap != nil {
		s.tap.Report(conn, attrs, msg)
	}
	if desc == nil {
		// Stateless means no per-connection attribute memory (ADR-0032):
		// a message without the description cannot be matched, so ask for
		// full state — the layer-1 digest keeps the re-report cheap.
		resp.Flags = uint64(protobufs.ServerToAgentFlags_ServerToAgentFlags_ReportFullState)
		return resp
	}

	match := s.snapshot.Load().Match(attrs)
	remote, err := remoteConfig(match.Artefact)
	if err != nil {
		// Never an empty config map (REQ-042, ADR-0010 rule 6): serving
		// nothing at all is a visible failure, while an applied empty map
		// reads as healthy success running nothing.
		s.logf("refusing to serve %s: %v", servedName(match), err)
		return resp
	}
	if bytes.Equal(msg.GetRemoteConfigStatus().GetLastRemoteConfigHash(), remote.ConfigHash) {
		// The collector already runs this head's artefact.
		return resp
	}
	resp.RemoteConfig = remote
	return resp
}

// onConnectionClose is where the second cache honours its lifetime: the
// digest dies with the connection (ADR-0032 §1) — and any tap learns its
// own per-connection holdings are now about a collector gone quiet.
func (s *Server) onConnectionClose(conn types.Connection) {
	s.digests.Delete(conn)
	if s.tap != nil {
		s.tap.Closed(conn)
	}
}

// remoteConfig wraps one rendered artefact for the wire, refusing an empty
// body outright — the never-serve-empty rule in its one enforceable place
// (REQ-042, ADR-0010 rule 6). The config hash is the artefact digest, so
// an unchanged artefact round-trips as "already applied".
func remoteConfig(artefact []byte) (*protobufs.AgentRemoteConfig, error) {
	if len(bytes.TrimSpace(artefact)) == 0 {
		return nil, errors.New("the artefact is empty — an empty config map applied cleanly is silent nothing (ADR-0010 rule 6)")
	}
	hash := sha256.Sum256(artefact)
	return &protobufs.AgentRemoteConfig{
		Config: &protobufs.AgentConfigMap{
			ConfigMap: map[string]*protobufs.AgentConfigFile{
				"": {Body: artefact, ContentType: "text/yaml"},
			},
		},
		ConfigHash: hash[:],
	}, nil
}

// attributesOf flattens the reported description to the string attributes
// selectors match on. Identifying attributes win a key collision; only
// string values participate — a selector is string equality (ADR-0007).
func attributesOf(desc *protobufs.AgentDescription) map[string]string {
	out := map[string]string{}
	add := func(kvs []*protobufs.KeyValue) {
		for _, kv := range kvs {
			if v, ok := kv.GetValue().GetValue().(*protobufs.AnyValue_StringValue); ok {
				out[kv.GetKey()] = v.StringValue
			}
		}
	}
	add(desc.GetNonIdentifyingAttributes())
	add(desc.GetIdentifyingAttributes())
	return out
}

// digestOf computes the layer-1 digest: raw bytes of the reported
// effective-config map, keys in sorted order so the digest is a function
// of content alone (ADR-0005 layer 1 — one hash, no parse).
func digestOf(ec *protobufs.EffectiveConfig) [sha256.Size]byte {
	files := ec.GetConfigMap().GetConfigMap()
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write(files[k].GetBody())
		h.Write([]byte{0})
	}
	var out [sha256.Size]byte
	copy(out[:], h.Sum(nil))
	return out
}

// headName names a snapshot's head for log lines.
func headName(s *Snapshot) string {
	if s.Commit == "" {
		return "(untracked directory)"
	}
	return s.Commit
}

// servedName names a match's destination for log lines.
func servedName(m Match) string {
	if m.Unmatched {
		return "the Unmatched artefact"
	}
	return "tier " + m.Tier
}

// opampLogger adapts the server's log func to the wire library's logger:
// errors surface, debug chatter stays quiet.
type opampLogger struct {
	logf func(format string, args ...any)
}

func (l opampLogger) Debugf(context.Context, string, ...any) {}

func (l opampLogger) Errorf(_ context.Context, format string, v ...any) {
	l.logf(format, v...)
}
