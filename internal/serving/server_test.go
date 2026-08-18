package serving

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/open-telemetry/opamp-go/client"
	clienttypes "github.com/open-telemetry/opamp-go/client/types"
	"github.com/open-telemetry/opamp-go/protobufs"
	"github.com/open-telemetry/opamp-go/server/types"
	"gopkg.in/yaml.v3"

	"github.com/telecraft-dev/telecraft/internal/renderer"
)

// startedServer builds and starts a Server over the fixture estate on an
// ephemeral port, stopping it with the test.
func startedServer(t *testing.T, root string) *Server {
	t.Helper()
	s, err := New(Config{
		Source:         DirSource{Root: root},
		ListenEndpoint: "127.0.0.1:0",
		FetchInterval:  time.Hour, // the poll is not under test
		Logf:           testLogger{t}.logf,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Stop(context.Background()); err != nil {
			t.Errorf("stopping the server: %v", err)
		}
	})
	return s
}

// connect runs an OpAMP client — the same library the Supervisor embeds —
// against the server, reporting attrs, and returns a channel of offered
// remote configs.
func connect(t *testing.T, s *Server, attrs map[string]string) chan *protobufs.AgentRemoteConfig {
	t.Helper()
	received := make(chan *protobufs.AgentRemoteConfig, 4)
	c := client.NewWebSocket(clientLogger{t})
	if err := c.SetAgentDescription(description(attrs)); err != nil {
		t.Fatal(err)
	}
	err := c.Start(context.Background(), clienttypes.StartSettings{
		OpAMPServerURL: fmt.Sprintf("ws://%s/v1/opamp", s.Addr()),
		InstanceUid:    clienttypes.InstanceUid{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		Capabilities: protobufs.AgentCapabilities_AgentCapabilities_AcceptsRemoteConfig |
			protobufs.AgentCapabilities_AgentCapabilities_ReportsRemoteConfig |
			protobufs.AgentCapabilities_AgentCapabilities_ReportsEffectiveConfig,
		Callbacks: clienttypes.Callbacks{
			OnMessage: func(_ context.Context, msg *clienttypes.MessageData) {
				if msg.RemoteConfig != nil {
					received <- msg.RemoteConfig
				}
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Stop(context.Background()); err != nil {
			t.Errorf("stopping the client: %v", err)
		}
	})
	return received
}

// await pulls one offered remote config or fails the test.
func await(t *testing.T, received chan *protobufs.AgentRemoteConfig) *protobufs.AgentRemoteConfig {
	t.Helper()
	select {
	case rc := <-received:
		return rc
	case <-time.After(15 * time.Second):
		t.Fatal("no remote config arrived")
		return nil
	}
}

// AC: a collector under supervision, pointed at the server over an estate
// repo, receives its Tier's config with the commit stamp riding inside
// (REQ-040, ADR-0013). The client here is the library the upstream
// Supervisor embeds, over the real WebSocket wire.
func TestServedCollectorReceivesTierArtefactWithCommitStamp(t *testing.T) {
	root, res := fixtureEstate(t)
	s := startedServer(t, root)

	rc := await(t, connect(t, s, gatewayAttrs()))
	body := rc.GetConfig().GetConfigMap()[""].GetBody()
	if want := res.Artefacts["rendered/pipelines/gateway.yaml"]; !bytes.Equal(body, want) {
		t.Errorf("served body differs from the rendered artefact:\n%s", body)
	}

	var doc struct {
		Service struct {
			Telemetry struct {
				Resource map[string]any `yaml:"resource"`
			} `yaml:"telemetry"`
		} `yaml:"service"`
	}
	if err := yaml.Unmarshal(body, &doc); err != nil {
		t.Fatalf("served config is not valid YAML: %v", err)
	}
	if got := doc.Service.Telemetry.Resource[renderer.CommitAttribute]; got != fixtureCommit {
		t.Errorf("commit stamp = %v, want %v — the artefact carries its own identity (ADR-0013)", got, fixtureCommit)
	}
}

// AC: an unmatched collector receives the Unmatched artefact — never an
// empty config map (REQ-042, ADR-0030): stamped, labelled
// governed-by-nobody, no data pipelines.
func TestUnmatchedCollectorReceivesUnmatchedArtefact(t *testing.T) {
	root, res := fixtureEstate(t)
	s := startedServer(t, root)

	rc := await(t, connect(t, s, map[string]string{
		"service.instance.id": "01J0000000000000000000TEST",
		"telecraft.tier":      "nothing-authored-matches-this",
	}))
	files := rc.GetConfig().GetConfigMap()
	if len(files) == 0 {
		t.Fatal("an unmatched collector received an empty config map (ADR-0010 rule 6)")
	}
	body := files[""].GetBody()
	if want := res.Artefacts[renderer.UnmatchedArtefactPath]; !bytes.Equal(body, want) {
		t.Errorf("served body differs from the Unmatched artefact:\n%s", body)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		t.Fatal("the served Unmatched artefact is empty")
	}
}

// The guard in its unit shape: nothing empty crosses remoteConfig, however
// a broken artefact might arrive there (ADR-0010 rule 6). The wire path
// then serves no RemoteConfig at all — visible failure over silent
// success.
func TestServerNeverServesAnEmptyConfigMap(t *testing.T) {
	for _, artefact := range [][]byte{nil, {}, []byte("   \n\t")} {
		if _, err := remoteConfig(artefact); err == nil {
			t.Errorf("remoteConfig(%q) built a config — the server must never serve an empty config map", artefact)
		}
	}

	root, _ := fixtureEstate(t)
	s := testServer(t, root)
	// Doctor the loaded snapshot to hold an empty artefact, simulating
	// corruption past the load-time refusals.
	snap := s.snapshot.Load()
	snap.entries[0].artefact = nil

	resp := s.onMessage(context.Background(), fakeConn{1}, &protobufs.AgentToServer{
		AgentDescription: description(attrsFor(snap.entries[0].selector)),
	})
	if resp.GetRemoteConfig() != nil {
		t.Fatal("an empty artefact was served as remote config (ADR-0010 rule 6)")
	}
}

// A message without the description cannot be matched, and the server
// keeps no per-connection attribute memory (ADR-0032) — so it asks for
// full state rather than guessing or remembering.
func TestMessageWithoutDescriptionRequestsFullState(t *testing.T) {
	root, _ := fixtureEstate(t)
	s := testServer(t, root)

	resp := s.onMessage(context.Background(), fakeConn{1}, &protobufs.AgentToServer{})
	if resp.Flags&uint64(protobufs.ServerToAgentFlags_ServerToAgentFlags_ReportFullState) == 0 {
		t.Error("no ReportFullState flag — a stateless server must ask, not remember")
	}
	if resp.GetRemoteConfig() != nil {
		t.Error("a config was served with nothing to match on")
	}
}

// A collector already running this head's artefact reports its hash back
// and gets no re-offer — the steady state is quiet.
func TestCollectorAlreadyOnHeadGetsNoReOffer(t *testing.T) {
	root, _ := fixtureEstate(t)
	s := testServer(t, root)

	first := s.onMessage(context.Background(), fakeConn{1}, &protobufs.AgentToServer{
		AgentDescription: description(gatewayAttrs()),
	})
	if first.GetRemoteConfig() == nil {
		t.Fatal("no config offered on first contact")
	}
	second := s.onMessage(context.Background(), fakeConn{1}, &protobufs.AgentToServer{
		AgentDescription:   description(gatewayAttrs()),
		RemoteConfigStatus: &protobufs.RemoteConfigStatus{LastRemoteConfigHash: first.GetRemoteConfig().GetConfigHash()},
	})
	if second.GetRemoteConfig() != nil {
		t.Error("the current artefact was re-offered to a collector already running it")
	}
}

// AC: restart loses only the digest, proven here (ADR-0032 §1): a fresh
// instance over the same repo serves byte-identically — artefact choice is
// a pure function of (head, reported attributes) — and the digest rebuilds
// from the collector's own next report, the ordinary cold-start cost of
// one extra parse.
func TestRestartLosesOnlyTheDigest(t *testing.T) {
	root, _ := fixtureEstate(t)
	msg := &protobufs.AgentToServer{
		AgentDescription: description(gatewayAttrs()),
		EffectiveConfig: &protobufs.EffectiveConfig{
			ConfigMap: &protobufs.AgentConfigMap{ConfigMap: map[string]*protobufs.AgentConfigFile{
				"": {Body: []byte("receivers: {}\n")},
			}},
		},
	}

	before := testServer(t, root)
	respBefore := before.onMessage(context.Background(), fakeConn{1}, msg)
	if n := digestCount(before); n != 1 {
		t.Fatalf("digest count = %d before restart, want 1", n)
	}

	// "Restart": a new process holds nothing the old one held.
	after := testServer(t, root)
	if n := digestCount(after); n != 0 {
		t.Fatalf("digest count = %d on a fresh instance, want 0 — only the digest may die (ADR-0032)", n)
	}
	respAfter := after.onMessage(context.Background(), fakeConn{7}, msg)
	if !bytes.Equal(servedBody(respBefore), servedBody(respAfter)) {
		t.Error("a restarted server served different bytes for the same head and attributes")
	}
	if n := digestCount(after); n != 1 {
		t.Errorf("digest count = %d after the collector re-reported, want 1 — the cost of restart is one extra parse", n)
	}
}

// AC: two instances behind a load balancer serve consistently with no
// coordination (ADR-0032 §2): independent snapshots of the same repo,
// nothing shared, identical decisions.
func TestTwoInstancesServeConsistentlyWithoutCoordination(t *testing.T) {
	root, _ := fixtureEstate(t)
	a, b := testServer(t, root), testServer(t, root)

	for name, attrs := range map[string]map[string]string{
		"matched":   gatewayAttrs(),
		"unmatched": {"telecraft.tier": "nothing"},
	} {
		msg := &protobufs.AgentToServer{AgentDescription: description(attrs)}
		fromA := servedBody(a.onMessage(context.Background(), fakeConn{1}, msg))
		fromB := servedBody(b.onMessage(context.Background(), fakeConn{2}, msg))
		if !bytes.Equal(fromA, fromB) {
			t.Errorf("%s: two uncoordinated instances served different configs", name)
		}
	}
}

// The digest is per-connection state in the strictest sense: it dies with
// the connection (ADR-0032 §1).
func TestDigestDiesWithTheConnection(t *testing.T) {
	root, _ := fixtureEstate(t)
	s := testServer(t, root)
	conn := fakeConn{1}

	s.onMessage(context.Background(), conn, &protobufs.AgentToServer{
		EffectiveConfig: &protobufs.EffectiveConfig{
			ConfigMap: &protobufs.AgentConfigMap{ConfigMap: map[string]*protobufs.AgentConfigFile{
				"": {Body: []byte("x")},
			}},
		},
	})
	if n := digestCount(s); n != 1 {
		t.Fatalf("digest count = %d, want 1", n)
	}
	s.onConnectionClose(conn)
	if n := digestCount(s); n != 0 {
		t.Errorf("digest count = %d after close, want 0 — the digest dies with the connection", n)
	}
}

// AC: the storage audit. ADR-0032 §1 states the closed list as a testable
// invariant: the serving path may hold the repo snapshot and the
// per-connection digest, nothing else. Every Server field is enumerated
// here as wiring or storage; adding one fails this test until it is
// classified — and a new storage field requires an ADR-0032 amendment.
func TestStorageInventoryIsTheClosedList(t *testing.T) {
	wiring := map[string]bool{
		"source":      true, // where snapshots come from
		"listen":      true, // the endpoint to open
		"interval":    true, // the poll cadence
		"logf":        true, // operational logging
		"opamp":       true, // the wire-protocol listener
		"stopRefresh": true, // poll shutdown
		"refreshDone": true, // poll shutdown
	}
	storage := map[string]bool{
		"snapshot": true, // ADR-0032 §1 item 1: the repo snapshot
		"digests":  true, // ADR-0032 §1 item 2: per-connection layer-1 digests
	}

	typ := reflect.TypeOf(Server{})
	for i := range typ.NumField() {
		name := typ.Field(i).Name
		if !wiring[name] && !storage[name] {
			t.Errorf("Server holds unclassified field %q — server-side storage is a closed list; classify it here, and if it stores collector data, ADR-0032 needs an amendment", name)
		}
	}
	if typ.NumField() != len(wiring)+len(storage) {
		t.Errorf("Server has %d fields, %d classified — keep the audit exhaustive", typ.NumField(), len(wiring)+len(storage))
	}
}

// testServer builds a Server over root and loads its snapshot without
// opening the listener — the serving decision under test is onMessage.
func testServer(t *testing.T, root string) *Server {
	t.Helper()
	s, err := New(Config{
		Source:         DirSource{Root: root},
		ListenEndpoint: "127.0.0.1:0",
		Logf:           testLogger{t}.logf,
	})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := s.source.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s.snapshot.Store(snap)
	return s
}

// digestCount counts the per-connection digests a server holds.
func digestCount(s *Server) int {
	n := 0
	s.digests.Range(func(any, any) bool { n++; return true })
	return n
}

// servedBody unwraps the single config-map entry of a response.
func servedBody(resp *protobufs.ServerToAgent) []byte {
	return resp.GetRemoteConfig().GetConfig().GetConfigMap()[""].GetBody()
}

// attrsFor builds attributes satisfying a selector exactly.
func attrsFor(selector map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range selector {
		out[k] = v
	}
	return out
}

// fakeConn is a comparable stand-in for a live connection.
type fakeConn struct{ id int }

func (fakeConn) Connection() net.Conn                                 { return nil }
func (fakeConn) Send(context.Context, *protobufs.ServerToAgent) error { return nil }
func (fakeConn) Disconnect() error                                    { return nil }

var _ types.Connection = fakeConn{}

// clientLogger adapts the test log to the wire library's client logger.
type clientLogger struct{ t *testing.T }

func (l clientLogger) Debugf(_ context.Context, format string, v ...any) { l.t.Logf(format, v...) }
func (l clientLogger) Errorf(_ context.Context, format string, v ...any) { l.t.Logf(format, v...) }
