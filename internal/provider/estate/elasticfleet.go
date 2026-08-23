// ElasticFleet is the second EstateProvider (REQ-044, ADR-0008): the
// foreign-population reading via Elastic Fleet's agent APIs. Elastic Fleet
// is a console, never a source of enforcement (NG-2): its OpAMP support is
// monitoring-only, remote config is unimplemented upstream, and enrolment
// pins the policy revision so none is ever delivered. That permanence is
// structural here: delivery status is declared incapable (ADR-0036 §1),
// so the reading Elastic Fleet can never supply renders "not applicable",
// never as failure; and the provider is read-only by construction, its
// transport refusing anything but GET.
//
// The reading takes two routes. The agent list
// (GET /api/fleet/agents?kuery=type:OPAMP) carries identity, the recursive
// component-health tree and last_checkin for every collector in one call.
// The effective config comes per collector
// (GET /api/fleet/agents/{id}/effective_config): the list response's
// compact pipeline_config fingerprint re-sorts receivers and exporters, so
// it cannot carry the ADR-0004 reading (order must survive verbatim), and
// only the per-collector route preserves it. The roll-up status field is
// never read: Elastic Fleet flattens it from top-level health only, so a
// collector with a dead receiver reads as online there; the tree is the
// reading (ADR-0008).
//
// Every reading is stamped as of the collector's last check-in, not the
// call instant: Elastic Fleet serves a collector's record long after the
// collector goes quiet, and a record's age is the check-in's age. The
// platform's staleness arithmetic (ADR-0036 §3) then demotes a quiet
// collector's readings honestly, where trusting the console's ever-fresh
// response would not.
package estate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	seam "github.com/telecraft-dev/telecraft/internal/estate"
)

// ElasticFleetConfig configures one ElasticFleet provider.
type ElasticFleetConfig struct {
	// Endpoint is the Kibana base URL the Elastic Fleet API is served
	// from. Mandatory.
	Endpoint string

	// APIKey is a Kibana API key, sent as an ApiKey authorization header.
	// Reading needs only the fleet-agents-read privilege; no write
	// privilege exists on any path this provider takes. Optional, for
	// test fixtures with no security.
	APIKey string

	// RefreshCadence is the cadence the provider declares (ADR-0036 §3):
	// how often a collector re-affirms its record, the OpAMP check-in
	// interval the estate's collectors run with, not how often this
	// provider is asked. Zero means DefaultRefreshCadence.
	RefreshCadence time.Duration

	// Timeout bounds each API round trip. Zero means 30s.
	Timeout time.Duration

	// Now is the clock the estate reading is stamped with; nil means
	// time.Now. Collector readings are stamped from last_checkin, not
	// this clock.
	Now func() time.Time
}

// ElasticFleet is the Elastic Fleet EstateProvider (REQ-044, ADR-0008).
type ElasticFleet struct {
	endpoint string
	apiKey   string
	http     *http.Client
	cadence  time.Duration
	now      func() time.Time

	// pageSize is the agent-list page size; tests shrink it to exercise
	// pagination.
	pageSize int
}

// NewElasticFleet validates the config and returns the provider. This is
// the only error the implementation ever produces: after boot, degradation
// is data in the reading, never an error (ADR-0008).
func NewElasticFleet(cfg ElasticFleetConfig) (*ElasticFleet, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("the ElasticFleet provider needs the Kibana base URL")
	}
	if cfg.RefreshCadence <= 0 {
		cfg.RefreshCadence = DefaultRefreshCadence
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &ElasticFleet{
		endpoint: strings.TrimRight(cfg.Endpoint, "/"),
		apiKey:   cfg.APIKey,
		http: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: readOnlyTransport{base: http.DefaultTransport},
		},
		cadence:  cfg.RefreshCadence,
		now:      cfg.Now,
		pageSize: 100,
	}, nil
}

var _ seam.Provider = (*ElasticFleet)(nil)

// readOnlyTransport refuses every request that is not a GET, so no
// enforcement path through Elastic Fleet can exist even by accident:
// enforcement via Elastic Fleet is permanently unavailable, not deferred
// (ADR-0008, NG-2).
type readOnlyTransport struct {
	base http.RoundTripper
}

func (t readOnlyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		return nil, fmt.Errorf("the ElasticFleet provider is read-only and refused %s %s: Telecraft only reads from Elastic Fleet, never writes to it", req.Method, req.URL.Path)
	}
	return t.base.RoundTrip(req)
}

// Name identifies the implementation as runtime data (ADR-0008). It is the
// same name ADR-0046 gives the delivery-path Mutation profile, because they
// name the same coupled behaviour.
func (p *ElasticFleet) Name() string { return "elastic-fleet" }

// Declaration is the static capability declaration (ADR-0036 §1).
func (p *ElasticFleet) Declaration() seam.Declaration {
	return seam.Declaration{
		Readings: map[seam.ReadingKind]bool{
			seam.EffectiveKind: true,
			seam.HealthKind:    true,

			// Elastic Fleet can never report delivery status: it is
			// monitoring-only, with no GA commitment and no "enforcement
			// later" (ADR-0008). Incapable is a declaration: the reading
			// renders "not applicable", never as failure (ADR-0036 §1).
			seam.DeliveryStatusKind: false,
		},
		RefreshCadence: p.cadence,
	}
}

// Estate reads the whole estate in one call (ADR-0008): every collector
// Elastic Fleet can currently see. Collectors must opt in to Elastic Fleet
// with an enrolment key; it has no discovery, so a genuinely unconnected
// collector is invisible here, absent from the reading rather than
// misreported in it. An unreadable console yields an empty estate: still a
// statement with a timestamp, and every lookup against it is honestly
// unknown with a cause, never an error.
func (p *ElasticFleet) Estate(ctx context.Context) seam.Estate {
	est := seam.Estate{Declaration: p.Declaration(), AsOf: p.now()}
	records, err := p.agents(ctx)
	if err != nil {
		return est
	}

	// One entry per identity: re-enrolment leaves the old record behind
	// with the same identifying attributes, and the newest check-in wins,
	// the same discipline as a reconnect on the OpAMP-direct wire.
	newest := map[string]agentRecord{}
	for _, r := range records {
		identity := identityOf(r.IdentifyingAttributes)
		if len(identity) == 0 {
			// A reading nothing can match belongs to nobody (ADR-0036 §2).
			continue
		}
		key := seam.Fingerprint(identity)
		if held, ok := newest[key]; !ok || r.checkin().After(held.checkin()) {
			newest[key] = r
		}
	}
	keys := make([]string, 0, len(newest))
	for key := range newest {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		r := newest[key]
		est.Collectors = append(est.Collectors, p.collectorOf(ctx, identityOf(r.IdentifyingAttributes), r))
	}
	return est
}

// collectorOf builds one collector's reading from its Elastic Fleet record.
// Every reading is stamped as of the record's last check-in; a record whose
// age cannot be established never feeds a verdict-shaped reading (ADR-0036
// §2), so both capable readings come back Known false with the cause
// naming exactly that.
func (p *ElasticFleet) collectorOf(ctx context.Context, identity map[string]string, r agentRecord) seam.Collector {
	c := seam.Collector{Identity: identity}

	asOf := r.checkin()
	if asOf.IsZero() {
		cause := fmt.Sprintf("the Elastic Fleet record carries no readable last_checkin (%q), so the age of this reading cannot be checked and it cannot feed a verdict", r.LastCheckin)
		now := p.now()
		c.Effective = seam.Effective{Known: false, Cause: cause, AsOf: now}
		c.Health = seam.Health{Known: false, Cause: cause, AsOf: now}
		return c
	}

	pipelines, cause := p.effective(ctx, r.ID)
	if cause != "" {
		c.Effective = seam.Effective{Known: false, Cause: cause, AsOf: asOf}
	} else {
		// An empty pipeline list is a collector reporting a config that
		// runs no pipelines: a reading, not a blind spot (ADR-0008).
		c.Effective = seam.Effective{Known: true, AsOf: asOf, Pipelines: pipelines}
	}

	if r.Health == nil {
		c.Health = seam.Health{Known: false, AsOf: asOf,
			Cause: "the collector has not reported component health to Elastic Fleet"}
	} else {
		c.Health = seam.Health{Known: true, AsOf: asOf, Component: healthRecordOf(*r.Health)}
	}

	// DeliveryStatus stays zero: declared incapable, absent-with-
	// declaration, never a silent gap, never a failure (ADR-0036 §1).
	return c
}

// effective fetches and parses one collector's reported effective config.
// A non-empty cause means the reading is Known false: not delivered, not
// readable, or not reachable. Degradation is data, never an error.
func (p *ElasticFleet) effective(ctx context.Context, agentID string) ([]seam.Pipeline, string) {
	var body struct {
		EffectiveConfig json.RawMessage `json:"effective_config"`
	}
	path := "/api/fleet/agents/" + url.PathEscape(agentID) + "/effective_config"
	if err := p.getJSON(ctx, path, &body); err != nil {
		return nil, fmt.Sprintf("Elastic Fleet could not return the effective config: %v", err)
	}
	raw := bytes.TrimSpace(body.EffectiveConfig)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, "the collector has not reported its effective config to Elastic Fleet"
	}
	// Elastic Fleet re-marshals the collector's YAML as JSON, structure
	// intact: redaction masks secret scalars but recurses into maps rather
	// than replacing them, so the pipeline wiring survives. JSON is a YAML
	// subset, so the wire-shared walk applies unchanged, with component order
	// carried verbatim, never resorted (ADR-0004).
	pipelines, err := servicePipelines(raw)
	if err != nil {
		return nil, fmt.Sprintf("the reported effective config could not be read: %v", err)
	}
	return pipelines, ""
}

// agentRecord is the slice of an Elastic Fleet agent record this provider
// reads. Collector records are ordinary agent records discriminated by
// type OPAMP; the collector-specific fields carry the OpAMP readings.
type agentRecord struct {
	ID                    string         `json:"id"`
	LastCheckin           string         `json:"last_checkin"`
	Health                *healthRecord  `json:"health"`
	IdentifyingAttributes map[string]any `json:"identifying_attributes"`
}

// checkin parses the record's last check-in; zero when absent or
// unreadable.
func (r agentRecord) checkin() time.Time {
	t, err := time.Parse(time.RFC3339Nano, r.LastCheckin)
	if err != nil {
		return time.Time{}
	}
	return t
}

// healthRecord is the recursive OpAMP ComponentHealth tree as the
// Elastic Fleet API returns it: arbitrarily deep, per pipeline and per
// component.
type healthRecord struct {
	Healthy    bool                    `json:"healthy"`
	Status     string                  `json:"status"`
	LastError  string                  `json:"last_error"`
	Components map[string]healthRecord `json:"component_health_map"`
}

// healthRecordOf converts the reported tree recursively: the full shape,
// never the flattened roll-up (ADR-0008).
func healthRecordOf(h healthRecord) seam.ComponentHealth {
	out := seam.ComponentHealth{
		Healthy:   h.Healthy,
		Status:    h.Status,
		LastError: h.LastError,
	}
	if len(h.Components) > 0 {
		out.Components = make(map[string]seam.ComponentHealth, len(h.Components))
		for name, child := range h.Components {
			out.Components[name] = healthRecordOf(child)
		}
	}
	return out
}

// agentPage is one page of the agent list response.
type agentPage struct {
	Items   []agentRecord `json:"items"`
	Total   int           `json:"total"`
	PerPage int           `json:"perPage"`
}

// agents lists every collector record, walking pages until the reported
// total is in hand. showInactive is deliberate: hiding a quiet collector
// is the console's freshness claim, and freshness is the platform's
// arithmetic, never the provider's claim (ADR-0036 §3). The record comes
// back and its last_checkin lets staleness demote it honestly.
func (p *ElasticFleet) agents(ctx context.Context) ([]agentRecord, error) {
	var out []agentRecord
	for page := 1; ; page++ {
		q := url.Values{
			"kuery":        {"type:OPAMP"},
			"perPage":      {strconv.Itoa(p.pageSize)},
			"page":         {strconv.Itoa(page)},
			"showInactive": {"true"},
		}
		var body agentPage
		if err := p.getJSON(ctx, "/api/fleet/agents?"+q.Encode(), &body); err != nil {
			return nil, err
		}
		out = append(out, body.Items...)
		if len(body.Items) < p.pageSize || len(out) >= body.Total {
			return out, nil
		}
	}
}

// getJSON performs one authenticated GET and decodes the response.
// Numbers decode as json.Number so an identifying attribute like a port
// survives as its literal, never a float approximation.
func (p *ElasticFleet) getJSON(ctx context.Context, path string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint+path, nil)
	if err != nil {
		return err
	}
	if p.apiKey != "" {
		req.Header.Set("Authorization", "ApiKey "+p.apiKey)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("GET %s: %s: %s", path, resp.Status, bytes.TrimSpace(detail))
	}
	dec := json.NewDecoder(resp.Body)
	dec.UseNumber()
	return dec.Decode(into)
}

// identityOf renders the record's identifying attributes as the identity
// selectors match on. The wire permits string and number values; numbers
// carry their literal form. A value that is neither cannot be matched on
// and is left out.
func identityOf(attrs map[string]any) map[string]string {
	if len(attrs) == 0 {
		return nil
	}
	out := make(map[string]string, len(attrs))
	for k, v := range attrs {
		switch v := v.(type) {
		case string:
			out[k] = v
		case json.Number:
			out[k] = v.String()
		case bool:
			out[k] = strconv.FormatBool(v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
