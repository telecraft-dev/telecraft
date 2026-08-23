package estate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	seam "github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/estate/estatetest"
)

// fleetFixture is a fixture Elastic Fleet API: the two GET routes the
// provider reads, paginated like the real list endpoint, recording every
// request so the read-only discipline can be asserted over the whole run.
type fleetFixture struct {
	agents  []map[string]any  // list records, in fixture order
	configs map[string]string // agent id -> effective_config JSON ("" = never reported)

	mu       sync.Mutex
	requests []string // "METHOD path" per request seen
}

func (f *fleetFixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	f.mu.Unlock()

	switch {
	case r.URL.Path == "/api/fleet/agents":
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))
		if page < 1 || perPage < 1 {
			http.Error(w, "bad paging", http.StatusBadRequest)
			return
		}
		lo := min((page-1)*perPage, len(f.agents))
		hi := min(lo+perPage, len(f.agents))
		json.NewEncoder(w).Encode(map[string]any{
			"items": f.agents[lo:hi], "total": len(f.agents), "page": page, "perPage": perPage,
		})
	case strings.HasPrefix(r.URL.Path, "/api/fleet/agents/") && strings.HasSuffix(r.URL.Path, "/effective_config"):
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/fleet/agents/"), "/effective_config")
		cfg := f.configs[id]
		if cfg == "" {
			cfg = "null"
		}
		fmt.Fprintf(w, `{"effective_config": %s}`, cfg)
	default:
		http.Error(w, `{"statusCode":404,"error":"Not Found"}`, http.StatusNotFound)
	}
}

// checkinT0 is the fixture collectors' last check-in; the clock the
// provider stamps the estate with sits a little after it.
var (
	checkinT0 = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	readT1    = checkinT0.Add(10 * time.Second)
)

// gatewayEffectiveConfig is the JSON tree the fixture gateway's config
// comes back as: the collector's YAML re-marshalled by the console,
// secret scalars masked, structure and order intact. Pipeline logs sits
// before traces and filelog before otlp, so any resort is caught.
const gatewayEffectiveConfig = `{
  "receivers": {"otlp": {"protocols": {"grpc": {}}}, "filelog": {}},
  "processors": {"batch": {}},
  "exporters": {"otlphttp": {"endpoint": "https://gateway.internal:4318", "api_key": "REDACTED"}},
  "service": {"pipelines": {
    "logs": {"receivers": ["filelog", "otlp"], "exporters": ["otlphttp"]},
    "traces": {"receivers": ["otlp"], "processors": ["batch"], "exporters": ["otlphttp"]}
  }}
}`

// fleetGatewayPipelines is gatewayEffectiveConfig as the seam must carry
// it: document order, component order, nothing resorted (ADR-0004).
func fleetGatewayPipelines() []seam.Pipeline {
	return []seam.Pipeline{
		{Name: "logs", Receivers: []string{"filelog", "otlp"}, Exporters: []string{"otlphttp"}},
		{Name: "traces", Receivers: []string{"otlp"}, Processors: []string{"batch"}, Exporters: []string{"otlphttp"}},
	}
}

// fleetGatewayHealth is the recursive tree the fixture gateway reports.
func fleetGatewayHealth() map[string]any {
	return map[string]any{
		"healthy": true,
		"status":  "StatusOK",
		"component_health_map": map[string]any{
			"pipeline:traces": map[string]any{
				"healthy": true,
				"component_health_map": map[string]any{
					"receiver:otlp": map[string]any{"healthy": true, "status": "StatusOK"},
				},
			},
		},
	}
}

// fleetWantHealth is fleetGatewayHealth converted: the same tree, same
// recursion.
func fleetWantHealth() *seam.ComponentHealth {
	return &seam.ComponentHealth{
		Healthy: true,
		Status:  "StatusOK",
		Components: map[string]seam.ComponentHealth{
			"pipeline:traces": {
				Healthy: true,
				Components: map[string]seam.ComponentHealth{
					"receiver:otlp": {Healthy: true, Status: "StatusOK"},
				},
			},
		},
	}
}

var (
	fleetGatewayIdentity = map[string]string{"service.instance.id": "a", "service.name": "otelcol-contrib"}
	fleetEdgeIdentity    = map[string]string{"service.instance.id": "b"}
)

// gatewayRecord is a fully reporting collector; edgeRecord has enrolled
// and identified itself but reported neither config nor health.
func gatewayRecord() map[string]any {
	return map[string]any{
		"id":                     "agent-a",
		"type":                   "OPAMP",
		"identifying_attributes": map[string]any{"service.instance.id": "a", "service.name": "otelcol-contrib"},
		"health":                 fleetGatewayHealth(),
		"last_checkin":           checkinT0.Format(time.RFC3339),
	}
}

func edgeRecord() map[string]any {
	return map[string]any{
		"id":                     "agent-b",
		"type":                   "OPAMP",
		"identifying_attributes": map[string]any{"service.instance.id": "b"},
		"last_checkin":           checkinT0.Format(time.RFC3339),
	}
}

func fleetFixtureEstate() *fleetFixture {
	return &fleetFixture{
		agents:  []map[string]any{gatewayRecord(), edgeRecord()},
		configs: map[string]string{"agent-a": gatewayEffectiveConfig},
	}
}

// fleetProvider builds an ElasticFleet against the fixture, on a pinned
// clock.
func fleetProvider(t *testing.T, f *fleetFixture) *ElasticFleet {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	p, err := NewElasticFleet(ElasticFleetConfig{
		Endpoint: srv.URL,
		Now:      func() time.Time { return readT1 },
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func fleetSeeds() []estatetest.Seed {
	return []estatetest.Seed{
		{Identity: fleetGatewayIdentity, Pipelines: fleetGatewayPipelines(), Health: fleetWantHealth()},
		{Identity: fleetEdgeIdentity},
	}
}

// AC: the ElasticFleet implementation passes the shipped conformance kit
// (ADR-0036 §4).
func TestElasticFleetPassesTheConformanceKit(t *testing.T) {
	p := fleetProvider(t, fleetFixtureEstate())
	estatetest.Run(t, estatetest.Kit{Provider: p, Seeded: fleetSeeds()})
}

// AC: the reading Elastic Fleet can never supply is declared incapable and
// renders as such: absent-with-declaration, never as failure (ADR-0036
// §1). Delivery status stays the zero value on every collector, seen or
// unseen, so no surface can mistake it for a silent gap.
func TestElasticFleetDeliveryStatusIsIncapableNeverFailure(t *testing.T) {
	p := fleetProvider(t, fleetFixtureEstate())
	if p.Declaration().Capable(seam.DeliveryStatusKind) {
		t.Fatal("the declaration claims delivery status: Elastic Fleet is monitoring-only and the capability must be declared never")
	}
	est := p.Estate(context.Background())
	for _, c := range est.Collectors {
		if !reflect.DeepEqual(c.DeliveryStatus, seam.DeliveryStatus{}) {
			t.Errorf("collector %s carries a delivery-status reading %+v: incapable stays absent-with-declaration", seam.Fingerprint(c.Identity), c.DeliveryStatus)
		}
	}
	unknown := est.Lookup(map[string]string{"service.instance.id": "nobody"})
	if !reflect.DeepEqual(unknown.DeliveryStatus, seam.DeliveryStatus{}) {
		t.Error("an unknown collector carries a delivery-status reading: incapable stays zero even for a collector nobody can see")
	}
}

// AC: no enforcement path through Elastic Fleet exists. Structurally: the
// provider's exported surface is exactly the read seam, and its transport
// refuses anything but GET, asserted over the whole fixture run and
// against a direct write attempt.
func TestElasticFleetHasNoEnforcementPath(t *testing.T) {
	f := fleetFixtureEstate()
	p := fleetProvider(t, f)
	p.Estate(context.Background())

	// The exported surface is the EstateProvider seam and nothing more:
	// no enrol, no remove, no apply, no write of any name.
	want := map[string]bool{"Name": true, "Declaration": true, "Estate": true}
	typ := reflect.TypeOf(p)
	for i := 0; i < typ.NumMethod(); i++ {
		if name := typ.Method(i).Name; !want[name] {
			t.Errorf("exported method %q is not part of the read seam: the provider must offer no path that could become enforcement", name)
		}
	}

	f.mu.Lock()
	requests := append([]string(nil), f.requests...)
	f.mu.Unlock()
	if len(requests) == 0 {
		t.Fatal("the fixture saw no requests")
	}
	for _, req := range requests {
		if !strings.HasPrefix(req, http.MethodGet+" ") {
			t.Errorf("the provider sent %q: every request must be a GET", req)
		}
	}

	// And the transport refuses a write outright, so even future code
	// inside the provider cannot open the path by accident.
	if _, err := p.http.Post(p.endpoint+"/api/fleet/agents/agent-a/remove_collector", "application/json", nil); err == nil {
		t.Error("a POST went through the provider's transport: the read-only transport failed")
	} else if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("the refusal %q does not name the read-only discipline", err)
	}
}

// The list endpoint pages; the reading walks every page, so an estate
// larger than one page is read whole.
func TestElasticFleetPaginatesTheAgentList(t *testing.T) {
	p := fleetProvider(t, fleetFixtureEstate())
	p.pageSize = 1
	est := p.Estate(context.Background())
	if got := len(est.Collectors); got != 2 {
		t.Fatalf("estate holds %d collectors across pages, want 2", got)
	}
}

// An unreadable console is an empty estate reading (still a statement
// with a timestamp) and every lookup against it is honestly unknown with
// a cause, never an error (ADR-0008).
func TestElasticFleetUnreachableConsoleIsAnEmptyReading(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	p, err := NewElasticFleet(ElasticFleetConfig{Endpoint: srv.URL, Now: func() time.Time { return readT1 }})
	if err != nil {
		t.Fatal(err)
	}

	est := p.Estate(context.Background())
	if est.AsOf.IsZero() {
		t.Error("the empty estate carries no as_of")
	}
	if len(est.Collectors) != 0 {
		t.Fatalf("estate holds %d collectors, want 0", len(est.Collectors))
	}
	c := est.Lookup(fleetGatewayIdentity)
	if c.Effective.Known || c.Effective.Cause == "" {
		t.Errorf("lookup against the empty estate = %+v, want Known false with a cause", c.Effective)
	}
}

// A record with no identifying attributes is no collector reading: nothing
// could match it, and a reading nothing can match belongs to nobody
// (ADR-0036 §2).
func TestElasticFleetRecordWithoutIdentityIsNotACollector(t *testing.T) {
	f := fleetFixtureEstate()
	f.agents = append(f.agents, map[string]any{"id": "agent-anon", "type": "OPAMP", "last_checkin": checkinT0.Format(time.RFC3339)})
	p := fleetProvider(t, f)
	if got := len(p.Estate(context.Background()).Collectors); got != 2 {
		t.Fatalf("estate holds %d collectors, want 2: the unidentified record must be left out", got)
	}
}

// A record whose age cannot be established never feeds a verdict-shaped
// reading: both capable readings come back Known false with the cause
// naming the unreadable check-in (ADR-0036 §2).
func TestElasticFleetRecordWithoutCheckinIsUnknown(t *testing.T) {
	f := fleetFixtureEstate()
	f.agents[0]["last_checkin"] = ""
	p := fleetProvider(t, f)

	c := p.Estate(context.Background()).Lookup(fleetGatewayIdentity)
	for _, r := range []struct {
		kind  string
		known bool
		cause string
		asOf  time.Time
	}{
		{"effective", c.Effective.Known, c.Effective.Cause, c.Effective.AsOf},
		{"health", c.Health.Known, c.Health.Cause, c.Health.AsOf},
	} {
		if r.known {
			t.Errorf("%s reads Known with no readable last_checkin: an unverifiable age never feeds a verdict", r.kind)
		}
		if !strings.Contains(r.cause, "last_checkin") {
			t.Errorf("%s cause %q does not name the unreadable check-in", r.kind, r.cause)
		}
		if r.asOf.IsZero() {
			t.Errorf("%s carries no as_of: 'we cannot see' is still a statement with a timestamp", r.kind)
		}
	}
}

// An effective config that cannot be read becomes Known false with the
// parse failure as the cause, never a guess, never a silent drop.
func TestElasticFleetUnreadableEffectiveConfigIsUnknownWithCause(t *testing.T) {
	f := fleetFixtureEstate()
	f.configs["agent-a"] = `{"service": {"pipelines": ["not", "a", "mapping"]}}`
	p := fleetProvider(t, f)

	c := p.Estate(context.Background()).Lookup(fleetGatewayIdentity)
	if c.Effective.Known {
		t.Fatal("an unreadable config reads as Known")
	}
	if !strings.Contains(c.Effective.Cause, "could not be read") {
		t.Errorf("cause %q does not name the unreadable report", c.Effective.Cause)
	}
}

// Re-enrolment leaves the old record behind under the same identifying
// attributes; the estate carries one entry, the newest check-in.
func TestElasticFleetStaleRecordLosesToNewestCheckin(t *testing.T) {
	f := fleetFixtureEstate()
	old := gatewayRecord()
	old["id"] = "agent-a-old"
	old["last_checkin"] = checkinT0.Add(-time.Hour).Format(time.RFC3339)
	delete(old, "health")
	f.agents = append([]map[string]any{old}, f.agents...)
	p := fleetProvider(t, f)

	est := p.Estate(context.Background())
	if got := len(est.Collectors); got != 2 {
		t.Fatalf("estate holds %d collectors, want 2: one per identity, the stale re-enrolment duplicate dropped", got)
	}
	c := est.Lookup(fleetGatewayIdentity)
	if !c.Health.Known || !c.Effective.AsOf.Equal(checkinT0) {
		t.Errorf("the stale record won: health known=%v, as_of=%v, want the newest record's readings", c.Health.Known, c.Effective.AsOf)
	}
}

// AC, end to end against the fixture: the estate reading populates the
// evaluation. Inside the staleness horizon the Effective pipelines feed
// ForEvaluation intact; once the collector has been quiet past the horizon
// the same reading demotes to Known false with its payload gone: stale
// data may inform a human, never a verdict (ADR-0036 §3).
func TestElasticFleetEstatePopulatesTheEvaluation(t *testing.T) {
	p := fleetProvider(t, fleetFixtureEstate())
	est := p.Estate(context.Background())

	fresh := est.ForEvaluation(readT1).Lookup(fleetGatewayIdentity)
	if !fresh.Effective.Known {
		t.Fatalf("the evaluation view lost the effective reading: %+v", fresh.Effective)
	}
	if !reflect.DeepEqual(fresh.Effective.Pipelines, fleetGatewayPipelines()) {
		t.Errorf("the evaluation sees pipelines %+v, want the reported wiring verbatim %+v", fresh.Effective.Pipelines, fleetGatewayPipelines())
	}
	if !fresh.Health.Known || !reflect.DeepEqual(fresh.Health.Component, *fleetWantHealth()) {
		t.Errorf("the evaluation sees health %+v, want the reported tree verbatim", fresh.Health)
	}

	quiet := checkinT0.Add(p.Declaration().RefreshCadence*seam.StaleTolerance + time.Second)
	stale := est.ForEvaluation(quiet).Lookup(fleetGatewayIdentity)
	if stale.Effective.Known || len(stale.Effective.Pipelines) != 0 {
		t.Errorf("a quiet collector's config survived into evaluation: %+v: the console serves the record forever, the platform's arithmetic must demote it", stale.Effective)
	}
}

// The pinned redaction rules (ADR-0046 §3) behave as the upstream code
// they mirror: substring match, case-insensitive, with the exact-match
// exemption checked first. The live contract test holds this same pin
// against the real API.
func TestElasticFleetRedactionRulesArePinned(t *testing.T) {
	for key, want := range map[string]bool{
		"api_key":       true, // the intended catch
		"password":      true,
		"Authorization": true,  // substring "auth", case-insensitive
		"routekey":      false, // exempted upstream, exact match
		"Routekey":      true,  // the exemption is case-sensitive
		"endpoint":      false,
		"receivers":     false,
	} {
		if got := ElasticFleetRedacts(key); got != want {
			t.Errorf("ElasticFleetRedacts(%q) = %v, want %v", key, got, want)
		}
	}
	if ElasticFleetRedactedValue != "REDACTED" {
		t.Errorf("placeholder = %q, want the exact upstream constant", ElasticFleetRedactedValue)
	}
}
