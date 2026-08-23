package telemetry

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/selftelemetry"
	seam "github.com/telecraft-dev/telecraft/internal/telemetry"
)

// meterOK is one signal's metering response: the collapsed counter
// deltas, the per-exporter out split a Hop's throughput reads, the
// freshness timestamp and the incarnation count. The incarnation buckets
// the backend summed the deltas from are present exactly as the backend
// returns them, and deliberately unread.
const meterOK = `{"status":200,"aggregations":{
	"in":{"doc_count":120,"instances":{"sum_other_doc_count":0,"buckets":[
		{"key":"a1","first":{"value":10},"last":{"value":600010},"delta":{"value":600000}},
		{"key":"b2","first":{"value":0},"last":{"value":400000},"delta":{"value":400000}}]},
		"total":{"value":1000000}},
	"refused":{"doc_count":120,"instances":{"sum_other_doc_count":0,"buckets":[]},"total":{"value":12}},
	"send_failed":{"doc_count":120,"instances":{"sum_other_doc_count":0,"buckets":[]},"total":{"value":3}},
	"enqueue_failed":{"doc_count":120,"instances":{"sum_other_doc_count":0,"buckets":[]},"total":{"value":0}},
	"out":{"doc_count":120,"exporters":{"sum_other_doc_count":0,"buckets":[
		{"key":"otlp/gateway","instances":{"sum_other_doc_count":0,"buckets":[]},"total":{"value":90000}},
		{"key":"debug","instances":{"sum_other_doc_count":0,"buckets":[]},"total":{"value":10000}}]},
		"total":{"value":100000}},
	"newest":{"value":1755431940000},
	"incarnations":{"value":24}}}`

func meterResponses(bodies ...string) string { return msearchResponse(bodies...) }

func TestElasticsearchMeterReading(t *testing.T) {
	var captured string
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured = string(body)
		w.Write([]byte(meterResponses(meterOK, meterOK, meterOK)))
	})

	m := es.Meter(context.Background(), "data-flow/gateway", time.Hour)

	if !m.AsOf.Equal(fixedAsOf) || m.Window != time.Hour {
		t.Errorf("reading carries as_of %v window %v", m.AsOf, m.Window)
	}
	if !m.Known() {
		t.Fatalf("a fully successful reading must be Known: %+v", m)
	}
	if len(m.Signals) != 3 {
		t.Fatalf("metering covers the three data signals; got %d", len(m.Signals))
	}

	traces := m.Signals[requirements.Traces]
	if traces.In != 1_000_000 || traces.Out != 100_000 {
		t.Errorf("traces flow = in %d out %d, want 1000000/100000", traces.In, traces.Out)
	}
	if traces.Refused != 12 || traces.SendFailed != 3 || traces.EnqueueFailed != 0 {
		t.Errorf("error-rate readings = %+v", traces)
	}
	if traces.Exporters["otlp/gateway"] != 90_000 || traces.Exporters["debug"] != 10_000 {
		t.Errorf("per-exporter split = %v, want the out-rate each Hop reads", traces.Exporters)
	}
	if traces.Newest.IsZero() {
		t.Error("no freshness timestamp came back: last-known-plus-age renders from the reading")
	}
	if !m.Incarnations.Known || m.Incarnations.Count != 24 {
		t.Errorf("incarnations = %+v, want 24", m.Incarnations)
	}

	// The query is scoped by the Tier stamp, buckets by incarnation
	// because a cumulative counter's delta is only exact within one, and
	// collapses those buckets backend-side (ADR-0040 §5).
	if !strings.Contains(captured, `"resource.attributes.telecraft.tier":"data-flow/gateway"`) {
		t.Error("query does not filter on the Tier stamp")
	}
	if !strings.Contains(captured, `"resource.attributes.service.instance.id"`) {
		t.Error("query does not bucket by incarnation")
	}
	if !strings.Contains(captured, `"sum_bucket"`) {
		t.Error("query does not collapse the incarnation buckets backend-side")
	}
	// Items are the unit (ADR-0040 §2): the counters read are the
	// item-counting helper metrics, per signal.
	for _, counter := range []string{
		"metrics.otelcol_receiver_accepted_spans",
		"metrics.otelcol_exporter_sent_metric_points",
		"metrics.otelcol_receiver_refused_log_records",
	} {
		if !strings.Contains(captured, counter) {
			t.Errorf("query does not read the %s counter", counter)
		}
	}
}

// ADR-0040 §6: a metering query the backend cannot answer declares the
// incapability. A missing index is never a metered zero.
func TestElasticsearchMeterMissingIndexReadsUnknown(t *testing.T) {
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(meterResponses(notFound, notFound, notFound)))
	})

	m := es.Meter(context.Background(), "data-flow/gateway", time.Hour)

	if m.Known() {
		t.Fatal("a missing index came back Known: metering never invents")
	}
	for kind, sig := range m.Signals {
		if sig.In != 0 || sig.Out != 0 {
			t.Errorf("%s carries numbers under an unknown reading: %+v", kind, sig)
		}
		if !strings.Contains(sig.Cause, "no index matches") {
			t.Errorf("%s cause = %q, want the missing index named", kind, sig.Cause)
		}
	}
	if m.Incarnations.Known {
		t.Errorf("incarnations = %+v, want unknown when nothing could be read", m.Incarnations)
	}
}

func TestElasticsearchMeterUnreachableBackend(t *testing.T) {
	es, srv := newFake(t, func(w http.ResponseWriter, r *http.Request) {})
	srv.Close()

	m := es.Meter(context.Background(), "data-flow/gateway", time.Hour)

	if m.Known() || m.Incarnations.Known {
		t.Fatal("an unreachable backend came back Known")
	}
	if !m.AsOf.Equal(fixedAsOf) {
		t.Errorf("as_of = %v: a degraded reading is still a statement with a timestamp", m.AsOf)
	}
	for kind, sig := range m.Signals {
		if !strings.Contains(sig.Cause, "unreachable") {
			t.Errorf("%s cause = %q, want the transport failure named", kind, sig.Cause)
		}
	}
}

// A capped fan-out is reported, never a quietly low throughput.
func TestElasticsearchMeterReportsTruncation(t *testing.T) {
	truncated := strings.Replace(meterOK,
		`"exporters":{"sum_other_doc_count":0`,
		`"exporters":{"sum_other_doc_count":7`, 1)
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(meterResponses(truncated, meterOK, meterOK)))
	})

	m := es.Meter(context.Background(), "data-flow/gateway", time.Hour)

	if !m.Signals[requirements.Logs].Truncated {
		t.Error("exporters beyond the cap were dropped silently")
	}
	if m.Signals[requirements.Metrics].Truncated {
		t.Error("an untruncated signal was reported truncated")
	}
}

// A negative delta cannot happen on a monotonic counter within one
// incarnation, and if the backend ever returns one it reads as zero
// rather than as negative throughput.
func TestElasticsearchMeterNeverReportsNegativeThroughput(t *testing.T) {
	negative := strings.Replace(meterOK, `"total":{"value":1000000}`, `"total":{"value":-5}`, 1)
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(meterResponses(negative, meterOK, meterOK)))
	})

	m := es.Meter(context.Background(), "data-flow/gateway", time.Hour)

	if got := m.Signals[requirements.Logs].In; got != 0 {
		t.Errorf("in = %d, want 0", got)
	}
}

// The seam is the only door: the provider satisfies it whole, metering
// included.
func TestElasticsearchSatisfiesTheSeam(t *testing.T) {
	var _ seam.Provider = (*Elasticsearch)(nil)
}

// Service-grain freshness rides the Observed reading (ADR-0040 §4): the
// age of the newest landed record, read rather than guessed.
func TestElasticsearchObserveCarriesNewestRecord(t *testing.T) {
	var captured []byte
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured = body
		w.Write([]byte(msearchResponse(
			`{"status":200,"hits":{"total":{"value":12}},"aggregations":{"newest":{"value":1755431940000}}}`,
			`{"status":200,"hits":{"total":{"value":0}},"aggregations":{"newest":{"value":null}}}`,
			hitsOK(3),
		)))
	})

	obs := es.Observe(context.Background(), seam.Service{Name: "checkout"}, time.Hour, nil)

	if !strings.Contains(string(captured), `"newest"`) {
		t.Error("query asks for no newest-record timestamp")
	}
	logs := obs.Signals[requirements.Logs]
	if logs.Newest.IsZero() {
		t.Error("a landed signal came back with no newest-record timestamp")
	}
	if want := time.UnixMilli(1755431940000).UTC(); !logs.Newest.Equal(want) {
		t.Errorf("newest = %v, want %v", logs.Newest, want)
	}
	if !obs.Signals[requirements.Metrics].Newest.IsZero() {
		t.Error("an empty window produced a timestamp: nothing landed is a reading, 1970 is a fabrication")
	}
}

// The metering query shape stays valid JSON per sub-body, which is what
// _msearch requires.
func TestMeterBodyIsWellFormed(t *testing.T) {
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {})
	counters, ok := selftelemetry.FlowCountersFor(requirements.Traces)
	if !ok {
		t.Fatal("traces carry no flow counters")
	}
	raw, err := json.Marshal(es.meterBody("data-flow/gateway", time.Hour, counters))
	if err != nil {
		t.Fatalf("metering body does not marshal: %v", err)
	}
	var round map[string]any
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatalf("metering body does not round-trip: %v", err)
	}
}
