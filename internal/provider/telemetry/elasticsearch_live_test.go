package telemetry

// Live verification against a real Elasticsearch (issue #10, criterion 1).
//
// These tests are gated on TELECRAFT_TELEMETRY_LIVE_ENDPOINT and skip
// cleanly when it is unset, so a plain `go test ./...` stays green on a
// machine with no Docker and no Elasticsearch. To run them locally in one
// command (container, seed, CLI demonstration, and this suite) use:
//
//	internal/provider/telemetry/demo.sh
//
// or point the gate at any reachable cluster:
//
//	TELECRAFT_TELEMETRY_LIVE_ENDPOINT=http://localhost:9200 go test ./internal/provider/telemetry/ -run Live -v

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	seam "github.com/telecraft-dev/telecraft/internal/telemetry"
)

const liveGate = "TELECRAFT_TELEMETRY_LIVE_ENDPOINT"

// liveIndices keeps the suite's footprint away from any real logs-*/
// metrics-*/traces-* data the cluster might hold. The traces index is
// deliberately never created: a live missing index must read Known false.
var liveIndices = map[requirements.SignalKind]string{
	requirements.Logs:    "telecraft-live-logs",
	requirements.Metrics: "telecraft-live-metrics",
	requirements.Traces:  "telecraft-live-traces",
}

func liveProvider(t *testing.T) *Elasticsearch {
	t.Helper()
	endpoint := envEndpoint(t)
	es, err := NewElasticsearch(ElasticsearchConfig{
		Endpoint: endpoint,
		Indices:  liveIndices,
	})
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func envEndpoint(t *testing.T) string {
	t.Helper()
	endpoint := os.Getenv(liveGate)
	if endpoint == "" {
		t.Skipf("set %s to run against a live Elasticsearch (see demo.sh)", liveGate)
	}
	return endpoint
}

// seedLive creates the logs index (strings mapped as keyword, matching how
// real OTLP ingest maps attribute fields) with three records for the fixture
// Service (two carrying http.request.method, between them two event names)
// plus an empty metrics index.
func seedLive(t *testing.T, endpoint string) {
	t.Helper()
	logs := liveIndices[requirements.Logs]
	metrics := liveIndices[requirements.Metrics]
	for _, idx := range []string{logs, metrics} {
		liveDo(t, http.MethodDelete, endpoint+"/"+idx+"?ignore_unavailable=true", "")
		liveDo(t, http.MethodPut, endpoint+"/"+idx, `{
			"mappings": {
				"dynamic_templates": [
					{"strings_as_keyword": {"match_mapping_type": "string", "mapping": {"type": "keyword"}}}
				],
				"properties": {"@timestamp": {"type": "date"}}
			}
		}`)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	doc := func(method, event string) string {
		attrs := fmt.Sprintf(`{"url.path": "/pay", "event.name": %q`, event)
		if method != "" {
			attrs += fmt.Sprintf(`, "http.request.method": %q`, method)
		}
		attrs += "}"
		return fmt.Sprintf(`{"@timestamp": %q, "resource": {"attributes": {"service.name": "checkout"}}, "attributes": %s}`, now, attrs)
	}
	var bulk bytes.Buffer
	for _, d := range []string{
		doc("POST", "checkout.order.placed"),
		doc("GET", "checkout.cart.updated"),
		doc("", "checkout.cart.updated"),
	} {
		bulk.WriteString(`{"index":{}}` + "\n" + d + "\n")
	}
	// A record for a different Service proves the reading is Service-scoped.
	// It carries a value and an event name nothing about checkout should
	// ever report, which is the misattribution ADR-0034 §4 forbids.
	bulk.WriteString(`{"index":{}}` + "\n")
	bulk.WriteString(fmt.Sprintf(`{"@timestamp": %q, "resource": {"attributes": {"service.name": "somebody-else"}}, "attributes": {"noise": "y", "http.request.method": "TRACE", "event.name": "somebody.else.event"}}`, now) + "\n")
	liveDo(t, http.MethodPost, endpoint+"/"+logs+"/_bulk?refresh=true", bulk.String())
}

func liveDo(t *testing.T, method, url, body string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		t.Fatalf("%s %s: %s: %s", method, url, resp.Status, detail)
	}
}

func TestElasticsearchLiveObserve(t *testing.T) {
	endpoint := envEndpoint(t)
	seedLive(t, endpoint)
	es := liveProvider(t)

	obs := es.Observe(context.Background(), seam.Service{Name: "checkout"}, 15*time.Minute,
		[]string{"resource.attributes.service.name", "attributes.http.request.method"})

	if obs.AsOf.IsZero() {
		t.Error("live reading carries no as_of")
	}

	logs := obs.Signals[requirements.Logs]
	if !logs.Known || !logs.Present || logs.Volume != 3 {
		t.Errorf("logs = %+v, want a known present reading of volume 3 (Service-scoped: the other Service's record is excluded)", logs)
	}
	if got := logs.AttributeCoverage["resource.attributes.service.name"]; got != 1.0 {
		t.Errorf("service.name coverage = %v, want 1.0", got)
	}
	if got := logs.AttributeCoverage["attributes.http.request.method"]; got < 0.66 || got > 0.67 {
		t.Errorf("http.request.method coverage = %v, want 2/3", got)
	}

	metrics := obs.Signals[requirements.Metrics]
	if !metrics.Known || metrics.Present || metrics.Volume != 0 {
		t.Errorf("metrics = %+v, want a known observed absence: the index exists and holds nothing", metrics)
	}

	// Criterion 2, live: the traces index was never created.
	traces := obs.Signals[requirements.Traces]
	if traces.Known {
		t.Errorf("traces = %+v, want Known=false: the index does not exist", traces)
	}
	if !strings.Contains(traces.Cause, liveIndices[requirements.Traces]) {
		t.Errorf("traces cause %q should name the missing index", traces.Cause)
	}
}

func TestElasticsearchLiveAttributeNames(t *testing.T) {
	endpoint := envEndpoint(t)
	seedLive(t, endpoint)
	es := liveProvider(t)

	names := es.AttributeNames(context.Background(), seam.Service{Name: "checkout"}, requirements.Logs, 15*time.Minute)
	if !names.Known {
		t.Fatalf("live names reading not Known: %+v", names)
	}
	if names.AsOf.IsZero() {
		t.Error("live names reading carries no as_of")
	}
	got := strings.Join(names.Names, ",")
	for _, want := range []string{"service.name", "http.request.method", "url.path"} {
		if !strings.Contains(got, want) {
			t.Errorf("names %v do not include %q", names.Names, want)
		}
	}
	if strings.Contains(got, "noise") {
		t.Errorf("names %v include another Service's attribute: the reading is not Service-scoped", names.Names)
	}
	if names.Truncated || names.SampledRecords != 3 || names.TotalRecords != 3 {
		t.Errorf("sampling misreported: %+v, want 3 of 3 records and no truncation", names)
	}

	missing := es.AttributeNames(context.Background(), seam.Service{Name: "checkout"}, requirements.Traces, 15*time.Minute)
	if missing.Known || missing.Cause == "" {
		t.Errorf("names against a missing index = %+v, want Known=false with a cause", missing)
	}
}

// Criterion (ADR-0034 §4): the value set is the Service's own. Another
// Service's records sit in the same index carrying the same attribute, and
// the value they carry must not appear here.
func TestElasticsearchLiveDistinctValues(t *testing.T) {
	endpoint := envEndpoint(t)
	seedLive(t, endpoint)
	es := liveProvider(t)

	got := es.DistinctValues(context.Background(), seam.Service{Name: "checkout"},
		requirements.Logs, "http.request.method", 15*time.Minute)
	if !got.Known {
		t.Fatalf("live value set not Known: %+v", got)
	}
	if got.AsOf.IsZero() {
		t.Error("live value set carries no as_of")
	}
	joined := strings.Join(got.Values, ",")
	if joined != "GET,POST" {
		t.Errorf("values = %v, want GET and POST sorted", got.Values)
	}
	if strings.Contains(joined, "TRACE") {
		t.Errorf("values %v carry another Service's value: the reading is not Service-scoped", got.Values)
	}
	if got.Truncated {
		t.Errorf("a two-value set inside the cap read as Truncated: %+v", got)
	}
	if got.Cap != seam.MaxDistinctValues {
		t.Errorf("cap = %d, want the seam's hard cap %d", got.Cap, seam.MaxDistinctValues)
	}

	missing := es.DistinctValues(context.Background(), seam.Service{Name: "checkout"},
		requirements.Traces, "http.request.method", 15*time.Minute)
	if missing.Known || missing.Cause == "" {
		t.Errorf("values against a missing index = %+v, want Known=false with a cause", missing)
	}
	if len(missing.Values) != 0 {
		t.Errorf("fabricated values against a missing index: %v", missing.Values)
	}
}

// Criterion (ADR-0034 §4): presence per event name, Service-scoped, and a
// missing index reads Known false rather than an empty group set.
func TestElasticsearchLiveGroupNames(t *testing.T) {
	endpoint := envEndpoint(t)
	seedLive(t, endpoint)
	es := liveProvider(t)

	got := es.GroupNames(context.Background(), seam.Service{Name: "checkout"},
		requirements.Logs, 15*time.Minute)
	if !got.Known {
		t.Fatalf("live group set not Known: %+v", got)
	}
	if got.AsOf.IsZero() {
		t.Error("live group set carries no as_of")
	}
	if got.Key != seam.EventName {
		t.Errorf("key = %q, want %q: logs group by event name", got.Key, seam.EventName)
	}
	joined := strings.Join(got.Names, ",")
	if joined != "checkout.cart.updated,checkout.order.placed" {
		t.Errorf("names = %v, want the two seeded event names sorted", got.Names)
	}
	if strings.Contains(joined, "somebody.else") {
		t.Errorf("names %v carry another Service's event: the reading is not Service-scoped", got.Names)
	}
	if got.Truncated {
		t.Errorf("a two-group set inside the cap read as Truncated: %+v", got)
	}

	missing := es.GroupNames(context.Background(), seam.Service{Name: "checkout"},
		requirements.Traces, 15*time.Minute)
	if missing.Known || missing.Cause == "" {
		t.Errorf("groups against a missing index = %+v, want Known=false with a cause", missing)
	}
	if !strings.Contains(missing.Cause, liveIndices[requirements.Traces]) {
		t.Errorf("cause %q should name the missing index", missing.Cause)
	}
}
