// Package estate holds the EstateProvider implementations (ADR-0008).
//
// The first is OpAMPDirect: the reading of collectors served by the
// platform's own OpAMP server. It taps the serving wire (serving.Tap),
// keeps the last report per live connection, and answers the seam from
// that cache alone. The cache is what ADR-0032 permits an off-path
// reader: derivable from live connections (a reconnecting collector
// rebuilds it with one full-state report) and dying with them, so a
// collector that disconnects leaves the estate reading rather than going
// quietly stale inside it.
//
// The package also versions the elastic-fleet Mutation profile
// (elasticfleet.go): a reading path's catalogued mutations live with its
// provider, never in core (ADR-0046 §3, ADR-0001).
package estate

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/open-telemetry/opamp-go/protobufs"
	"gopkg.in/yaml.v3"

	seam "github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/serving"
)

// DefaultRefreshCadence is the declared cadence when none is configured:
// the OpAMP default heartbeat, the interval at which a quiet collector
// re-affirms its readings over a live connection.
const DefaultRefreshCadence = 30 * time.Second

// OpAMPDirectConfig configures one OpAMPDirect.
type OpAMPDirectConfig struct {
	// RefreshCadence is the cadence the provider declares (ADR-0036 §3):
	// how often a live collector re-affirms its readings, the OpAMP
	// heartbeat the server is run with. Zero means DefaultRefreshCadence.
	RefreshCadence time.Duration

	// Now is the clock readings are stamped with; nil means time.Now.
	Now func() time.Time
}

// OpAMPDirect is the OpAMP-direct EstateProvider (REQ-044, ADR-0008): the
// reading of every collector currently connected to the platform's own
// server. Wire it as the server's Tap (serving.Config.Tap); it implements
// the seam from what the wire shows it and nothing else.
type OpAMPDirect struct {
	cadence time.Duration
	now     func() time.Time

	mu      sync.Mutex
	records map[any]*record
}

// record is one live connection's last report, merged message over
// message: OpAMP omits what has not changed, so an absent field on a
// later message re-affirms the held reading rather than erasing it.
type record struct {
	// asOf is the instant of the last message on the connection. Every
	// reading held here is current as of that instant: a message that
	// carries nothing new still says "unchanged, as of now".
	asOf     time.Time
	identity map[string]string

	// effective is nil until the collector first reports its config.
	effective *effectiveReport
	// health is nil until the collector first reports component health.
	health *seam.ComponentHealth
	// delivery is nil until the collector first reports remote-config
	// status.
	delivery *deliveryReport
}

// effectiveReport is one parsed effective-config report; a report that
// would not parse is held as its cause, never silently dropped.
type effectiveReport struct {
	pipelines []seam.Pipeline
	cause     string // non-empty means the report was unreadable
}

// deliveryReport is one remote-config-status report, the OpAMP vocabulary
// carried verbatim (ADR-0004).
type deliveryReport struct {
	state seam.DeliveryState
	hash  []byte
	err   string
}

// NewOpAMPDirect builds the provider. Wire it into the server via
// serving.Config.Tap before Start; it holds nothing until collectors
// report.
func NewOpAMPDirect(cfg OpAMPDirectConfig) *OpAMPDirect {
	if cfg.RefreshCadence <= 0 {
		cfg.RefreshCadence = DefaultRefreshCadence
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &OpAMPDirect{
		cadence: cfg.RefreshCadence,
		now:     cfg.Now,
		records: map[any]*record{},
	}
}

var (
	_ seam.Provider = (*OpAMPDirect)(nil)
	_ serving.Tap   = (*OpAMPDirect)(nil)
)

// Name identifies the implementation as runtime data (ADR-0008).
func (p *OpAMPDirect) Name() string { return "opamp-direct" }

// Declaration is the static capability declaration (ADR-0036 §1): the
// wire carries all three readings, so all three are declared capable;
// a connected collector that reports none of them is capable-but-unknown
// per reading, with the cause naming what was never reported.
func (p *OpAMPDirect) Declaration() seam.Declaration {
	return seam.Declaration{
		Readings: map[seam.ReadingKind]bool{
			seam.EffectiveKind:      true,
			seam.HealthKind:         true,
			seam.DeliveryStatusKind: true,
		},
		RefreshCadence: p.cadence,
	}
}

// Report is the serving.Tap inbound: merge what the message carried into
// the connection's record. OpAMP compression means absence is "unchanged",
// so the whole record's asOf advances with every message: the collector
// just re-affirmed everything it is not re-sending.
func (p *OpAMPDirect) Report(conn any, identity map[string]string, msg *protobufs.AgentToServer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r := p.records[conn]
	if r == nil {
		r = &record{}
		p.records[conn] = r
	}
	r.asOf = p.now()
	if identity != nil {
		r.identity = identity
	}
	if ec := msg.GetEffectiveConfig(); ec != nil {
		pipelines, err := pipelinesOf(ec)
		if err != nil {
			r.effective = &effectiveReport{cause: fmt.Sprintf("the reported effective config could not be read: %v", err)}
		} else {
			r.effective = &effectiveReport{pipelines: pipelines}
		}
	}
	if h := msg.GetHealth(); h != nil {
		tree := healthOf(h)
		r.health = &tree
	}
	if st := msg.GetRemoteConfigStatus(); st != nil {
		r.delivery = &deliveryReport{
			state: deliveryStateOf(st.GetStatus()),
			hash:  st.GetLastRemoteConfigHash(),
			err:   st.GetErrorMessage(),
		}
	}
}

// Closed drops the connection's record: the cache dies with the
// connection (ADR-0032). A collector gone from the wire is gone from the
// reading: asking for it afterwards yields Known false, never a stale
// impression of presence.
func (p *OpAMPDirect) Closed(conn any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.records, conn)
}

// Estate reads the whole estate in one call (ADR-0008): every identified
// collector on a live connection. A connection that has not yet reported
// identity attributes is not a collector reading: nothing could match
// it, and the server has already asked it for full state.
func (p *OpAMPDirect) Estate(context.Context) seam.Estate {
	est := seam.Estate{Declaration: p.Declaration(), AsOf: p.now()}

	p.mu.Lock()
	records := make([]*record, 0, len(p.records))
	for _, r := range p.records {
		if len(r.identity) > 0 {
			records = append(records, r)
		}
	}
	p.mu.Unlock()

	// One entry per identity: a reconnect can briefly leave two live
	// connections for one collector, and the newer report wins.
	newest := map[string]*record{}
	for _, r := range records {
		key := seam.Fingerprint(r.identity)
		if held, ok := newest[key]; !ok || r.asOf.After(held.asOf) {
			newest[key] = r
		}
	}
	keys := make([]string, 0, len(newest))
	for key := range newest {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		est.Collectors = append(est.Collectors, collectorOf(newest[key]))
	}
	return est
}

// collectorOf builds one collector's reading from its record. A reading
// the collector never sent is Known false with the cause naming exactly
// that: capable-but-unknown, loud, never a silent gap (ADR-0036 §1).
func collectorOf(r *record) seam.Collector {
	c := seam.Collector{Identity: r.identity}

	switch {
	case r.effective == nil:
		c.Effective = seam.Effective{Known: false, AsOf: r.asOf,
			Cause: "the collector has not reported its effective config over this connection"}
	case r.effective.cause != "":
		c.Effective = seam.Effective{Known: false, AsOf: r.asOf, Cause: r.effective.cause}
	default:
		// An empty pipeline list is a collector reporting an empty
		// config: a reading, not a blind spot (ADR-0008).
		c.Effective = seam.Effective{Known: true, AsOf: r.asOf, Pipelines: r.effective.pipelines}
	}

	if r.health == nil {
		c.Health = seam.Health{Known: false, AsOf: r.asOf,
			Cause: "the collector does not report component health over this connection"}
	} else {
		c.Health = seam.Health{Known: true, AsOf: r.asOf, Component: *r.health}
	}

	if r.delivery == nil {
		c.DeliveryStatus = seam.DeliveryStatus{Known: false, AsOf: r.asOf,
			Cause: "the collector has not reported remote-config status over this connection"}
	} else {
		c.DeliveryStatus = seam.DeliveryStatus{Known: true, AsOf: r.asOf,
			State: r.delivery.state, ConfigHash: r.delivery.hash, Error: r.delivery.err}
	}
	return c
}

// deliveryStateOf carries the wire vocabulary across verbatim, with no
// invented delivery states (ADR-0004).
func deliveryStateOf(s protobufs.RemoteConfigStatuses) seam.DeliveryState {
	switch s {
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLIED:
		return seam.DeliveryApplied
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_APPLYING:
		return seam.DeliveryApplying
	case protobufs.RemoteConfigStatuses_RemoteConfigStatuses_FAILED:
		return seam.DeliveryFailed
	default:
		return seam.DeliveryUnset
	}
}

// healthOf converts the reported tree recursively: the full shape, never
// the flattened roll-up (ADR-0008).
func healthOf(h *protobufs.ComponentHealth) seam.ComponentHealth {
	out := seam.ComponentHealth{
		Healthy:   h.GetHealthy(),
		Status:    h.GetStatus(),
		LastError: h.GetLastError(),
	}
	if children := h.GetComponentHealthMap(); len(children) > 0 {
		out.Components = make(map[string]seam.ComponentHealth, len(children))
		for name, child := range children {
			out.Components[name] = healthOf(child)
		}
	}
	return out
}

// pipelinesOf parses the reported config map into pipelines with
// component order preserved (ADR-0004). Files are read in sorted-name
// order; a pipeline re-declared by a later file replaces the earlier one
// in place, keeping first-seen document order.
func pipelinesOf(ec *protobufs.EffectiveConfig) ([]seam.Pipeline, error) {
	files := ec.GetConfigMap().GetConfigMap()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []seam.Pipeline
	index := map[string]int{}
	for _, name := range names {
		body := files[name].GetBody()
		if len(bytes.TrimSpace(body)) == 0 {
			continue
		}
		pipelines, err := servicePipelines(body)
		if err != nil {
			return nil, fmt.Errorf("config %q: %w", name, err)
		}
		for _, p := range pipelines {
			if i, seen := index[p.Name]; seen {
				out[i] = p
				continue
			}
			index[p.Name] = len(out)
			out = append(out, p)
		}
	}
	return out, nil
}

// servicePipelines walks one otelcol document to service.pipelines,
// preserving the document's pipeline order and each pipeline's component
// order. A document without the section is a config running no pipelines:
// valid, empty. A section that is not the expected shape is an error:
// a config we cannot read must become Known false, never a guess.
func servicePipelines(body []byte) ([]seam.Pipeline, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("not valid YAML: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return nil, nil
	}
	section := mappingValue(mappingValue(doc.Content[0], "service"), "pipelines")
	if section == nil {
		return nil, nil
	}
	if section.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("service.pipelines is not a mapping")
	}
	var out []seam.Pipeline
	for i := 0; i+1 < len(section.Content); i += 2 {
		var parsed struct {
			Receivers  []string `yaml:"receivers"`
			Processors []string `yaml:"processors"`
			Exporters  []string `yaml:"exporters"`
		}
		if err := section.Content[i+1].Decode(&parsed); err != nil {
			return nil, fmt.Errorf("pipeline %q: %w", section.Content[i].Value, err)
		}
		out = append(out, seam.Pipeline{
			Name:       section.Content[i].Value,
			Receivers:  parsed.Receivers,
			Processors: parsed.Processors,
			Exporters:  parsed.Exporters,
		})
	}
	return out, nil
}

// mappingValue finds one key's value node in a mapping; nil when the node
// is not a mapping or the key is absent.
func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
