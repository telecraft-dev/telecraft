package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	seam "github.com/telecraft-dev/telecraft/internal/telemetry"
)

var fixedAsOf = time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)

// newFake builds a provider pointed at an httptest server and pins its
// clock, so every test can assert as_of exactly.
func newFake(t *testing.T, handler http.HandlerFunc) (*Elasticsearch, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	es, err := NewElasticsearch(ElasticsearchConfig{Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	es.now = func() time.Time { return fixedAsOf }
	return es, srv
}

// msearchResponse renders one _msearch body with the given per-signal
// responses, in seam signal order.
func msearchResponse(responses ...string) string {
	return `{"responses":[` + strings.Join(responses, ",") + `]}`
}

const (
	emptyOK  = `{"status":200,"hits":{"total":{"value":0}}}`
	notFound = `{"status":404,"error":{"type":"index_not_found_exception","reason":"no such index"}}`
)

func hitsOK(total int64) string {
	return `{"status":200,"hits":{"total":{"value":` + jsonInt(total) + `}}}`
}

func jsonInt(v int64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestElasticsearchObserveReading(t *testing.T) {
	var captured []byte
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		captured = body
		w.Write([]byte(msearchResponse(
			`{"status":200,"hits":{"total":{"value":120}},"aggregations":{"attr_service_name":{"doc_count":120},"attr_http_request_method":{"doc_count":80}}}`,
			hitsOK(0),
			hitsOK(7),
		)))
	})

	obs := es.Observe(context.Background(), seam.Service{Name: "checkout"}, 15*time.Minute,
		[]string{"service.name", "http.request.method"})

	if !obs.AsOf.Equal(fixedAsOf) {
		t.Errorf("as_of = %v, want the reading instant %v", obs.AsOf, fixedAsOf)
	}
	if obs.Window != 15*time.Minute {
		t.Errorf("window = %v, want 15m", obs.Window)
	}
	if !obs.Known() {
		t.Fatalf("a fully successful reading must be Known: %+v", obs)
	}

	logs := obs.Signals[requirements.Logs]
	if !logs.Present || logs.Volume != 120 {
		t.Errorf("logs = %+v, want present with volume 120", logs)
	}
	if got := logs.AttributeCoverage["service.name"]; got != 1.0 {
		t.Errorf("service.name coverage = %v, want 1.0", got)
	}
	if got := logs.AttributeCoverage["http.request.method"]; got != 80.0/120.0 {
		t.Errorf("http.request.method coverage = %v, want 80/120", got)
	}

	metrics := obs.Signals[requirements.Metrics]
	if metrics.Present || metrics.Volume != 0 {
		t.Errorf("metrics = %+v, want an observed absence", metrics)
	}
	if metrics.AttributeCoverage != nil {
		t.Errorf("metrics coverage measured over zero records: %v: coverage must be omitted, not fabricated", metrics.AttributeCoverage)
	}

	traces := obs.Signals[requirements.Traces]
	if !traces.Present || traces.Volume != 7 {
		t.Errorf("traces = %+v, want present with volume 7", traces)
	}

	// The request itself: strict index resolution, the window as date math,
	// a term filter on the configured service.name field.
	req := string(captured)
	for _, want := range []string{
		`"allow_no_indices":false`,
		`"ignore_unavailable":false`,
		`"now-15m"`,
		`"resource.attributes.service.name":"checkout"`,
		`"track_total_hits":true`,
	} {
		if !strings.Contains(req, want) {
			t.Errorf("search request does not contain %s\nrequest: %s", want, req)
		}
	}
}

// Criterion: an unreachable backend yields Known false readings, never a
// crash, never a fabricated value.
// Criterion (ADR-0033): a reading narrowed to one Environment filters on the
// environment field, so the same Service in two environments yields
// independent readings and cross-environment blending is impossible at the
// source. An unscoped Service carries no environment filter: unchanged
// behaviour for callers that predate the environment axis.
func TestElasticsearchObserveEnvironmentScope(t *testing.T) {
	var captured []byte
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		captured = body
		w.Write([]byte(msearchResponse(hitsOK(1), hitsOK(1), hitsOK(1))))
	})

	es.Observe(context.Background(), seam.Service{Name: "checkout", Environment: "production"}, time.Hour, nil)
	if want := `"resource.attributes.deployment.environment.name":"production"`; !strings.Contains(string(captured), want) {
		t.Errorf("environment-scoped request does not contain %s\nrequest: %s", want, captured)
	}

	es.Observe(context.Background(), seam.Service{Name: "checkout"}, time.Hour, nil)
	if got := string(captured); strings.Contains(got, "deployment.environment.name") {
		t.Errorf("unscoped request must carry no environment filter\nrequest: %s", got)
	}
}

func TestElasticsearchObserveUnreachableBackend(t *testing.T) {
	es, err := NewElasticsearch(ElasticsearchConfig{Endpoint: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	es.now = func() time.Time { return fixedAsOf }

	obs := es.Observe(context.Background(), seam.Service{Name: "checkout"}, time.Hour, []string{"service.name"})

	if obs.Known() {
		t.Fatal("reading against an unreachable backend must not be Known")
	}
	if len(obs.Signals) != 3 {
		t.Fatalf("degraded reading covers %d signals, want all 3", len(obs.Signals))
	}
	for kind, sig := range obs.Signals {
		if sig.Known {
			t.Errorf("%s: Known = true against an unreachable backend", kind)
		}
		if sig.Cause == "" {
			t.Errorf("%s: degraded reading carries no cause", kind)
		}
		if sig.Present || sig.Volume != 0 || sig.AttributeCoverage != nil {
			t.Errorf("%s: fabricated observation on a degraded reading: %+v", kind, sig)
		}
	}
}

// Criterion: a missing index yields a Known false reading for that signal:
// the provider cannot tell "nothing ever landed" from "wrong index", so it
// never renders the gap as an observed absence. The other signals stay known.
func TestElasticsearchObserveMissingIndex(t *testing.T) {
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(msearchResponse(notFound, hitsOK(3), notFound)))
	})

	obs := es.Observe(context.Background(), seam.Service{Name: "checkout"}, time.Hour, nil)

	logs := obs.Signals[requirements.Logs]
	if logs.Known {
		t.Error("logs: Known = true although the index does not exist")
	}
	if !strings.Contains(logs.Cause, "logs-*") || !strings.Contains(logs.Cause, "wrong index") {
		t.Errorf("logs cause %q should name the index pattern and the ambiguity", logs.Cause)
	}
	if metrics := obs.Signals[requirements.Metrics]; !metrics.Known || metrics.Volume != 3 {
		t.Errorf("metrics = %+v, want a known reading: one signal's missing index says nothing about the others", metrics)
	}
	if traces := obs.Signals[requirements.Traces]; traces.Known {
		t.Error("traces: Known = true although the index does not exist")
	}
	if obs.Known() {
		t.Error("a reading with blind signals must not report Known overall")
	}
}

func TestElasticsearchObserveDegradations(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"http error status", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error":"security_exception"}`, http.StatusUnauthorized)
		}},
		{"undecodable response", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"responses":[`))
		}},
		{"response count mismatch", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(msearchResponse(hitsOK(1))))
		}},
		{"per-signal query failure", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(msearchResponse(
				`{"status":400,"error":{"type":"search_phase_execution_exception","reason":"bad field"}}`,
				`{"status":400,"error":{"type":"search_phase_execution_exception","reason":"bad field"}}`,
				`{"status":400,"error":{"type":"search_phase_execution_exception","reason":"bad field"}}`,
			)))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			es, _ := newFake(t, tc.handler)
			obs := es.Observe(context.Background(), seam.Service{Name: "checkout"}, time.Hour, nil)
			if obs.Known() {
				t.Fatalf("degraded backend (%s) produced a Known reading", tc.name)
			}
			for kind, sig := range obs.Signals {
				if sig.Known || sig.Cause == "" {
					t.Errorf("%s: want Known=false with a cause, got %+v", kind, sig)
				}
				if sig.Present || sig.Volume != 0 {
					t.Errorf("%s: fabricated observation: %+v", kind, sig)
				}
			}
		})
	}
}

// Criterion: every reading carries as_of, the degraded paths included.
func TestReadingsAlwaysCarryAsOf(t *testing.T) {
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	obs := es.Observe(context.Background(), seam.Service{Name: "checkout"}, time.Hour, nil)
	if !obs.AsOf.Equal(fixedAsOf) {
		t.Errorf("degraded Observed as_of = %v, want %v", obs.AsOf, fixedAsOf)
	}

	names := es.AttributeNames(context.Background(), seam.Service{Name: "checkout"}, requirements.Logs, time.Hour)
	if !names.AsOf.Equal(fixedAsOf) {
		t.Errorf("degraded AttributeNames as_of = %v, want %v", names.AsOf, fixedAsOf)
	}
}

func TestElasticsearchAttributeNames(t *testing.T) {
	var captured []byte
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		captured = body
		w.Write([]byte(`{"hits":{"total":{"value":5},"hits":[
			{"fields":{"resource.attributes.service.name":["checkout"],"attributes.http.request.method":["GET"],"attributes.http.request.method.keyword":["GET"]}},
			{"fields":{"resource.attributes.service.name":["checkout"],"attributes.url.path":["/pay"],"unrelated.field":["x"]}}
		]}}`))
	})

	names := es.AttributeNames(context.Background(), seam.Service{Name: "checkout"}, requirements.Logs, time.Hour)

	if !names.Known {
		t.Fatalf("reading not Known: %+v", names)
	}
	want := []string{"http.request.method", "service.name", "url.path"}
	if len(names.Names) != len(want) {
		t.Fatalf("names = %v, want %v (prefix-stripped, .keyword collapsed, unrelated fields dropped)", names.Names, want)
	}
	for i := range want {
		if names.Names[i] != want[i] {
			t.Errorf("names[%d] = %q, want %q (sorted)", i, names.Names[i], want[i])
		}
	}
	if names.SampledRecords != 2 || names.TotalRecords != 5 || !names.Truncated {
		t.Errorf("sampling not reported honestly: %+v: 2 of 5 records inspected must read Truncated", names)
	}

	req := string(captured)
	for _, wantFrag := range []string{`"_source":false`, `"attributes.*"`, `"resource.attributes.service.name":"checkout"`} {
		if !strings.Contains(req, wantFrag) {
			t.Errorf("names request does not contain %s\nrequest: %s", wantFrag, req)
		}
	}
}

func TestElasticsearchAttributeNamesComplete(t *testing.T) {
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"hits":{"total":{"value":1},"hits":[{"fields":{"attributes.k":["v"]}}]}}`))
	})
	names := es.AttributeNames(context.Background(), seam.Service{Name: "checkout"}, requirements.Logs, time.Hour)
	if !names.Known || names.Truncated {
		t.Errorf("every record inspected must read Known and not Truncated: %+v", names)
	}
}

func TestElasticsearchAttributeNamesDegraded(t *testing.T) {
	t.Run("missing index", func(t *testing.T) {
		es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":{"type":"index_not_found_exception","reason":"no such index"},"status":404}`))
		})
		names := es.AttributeNames(context.Background(), seam.Service{Name: "checkout"}, requirements.Traces, time.Hour)
		if names.Known || names.Cause == "" {
			t.Errorf("want Known=false with a cause, got %+v", names)
		}
		if len(names.Names) != 0 {
			t.Errorf("fabricated names on a degraded reading: %v", names.Names)
		}
	})
	t.Run("unreachable backend", func(t *testing.T) {
		es, err := NewElasticsearch(ElasticsearchConfig{Endpoint: "http://127.0.0.1:1"})
		if err != nil {
			t.Fatal(err)
		}
		es.now = func() time.Time { return fixedAsOf }
		names := es.AttributeNames(context.Background(), seam.Service{Name: "checkout"}, requirements.Logs, time.Hour)
		if names.Known || names.Cause == "" {
			t.Errorf("want Known=false with a cause, got %+v", names)
		}
	})
}

func TestElasticsearchRequiresEndpoint(t *testing.T) {
	if _, err := NewElasticsearch(ElasticsearchConfig{}); err == nil {
		t.Error("a provider with no endpoint must be a boot error, not an estate that reads all-unknown forever")
	}
}

func TestNeutralFactory(t *testing.T) {
	p, err := New(Config{Endpoint: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name() == "" {
		t.Error("factory-built provider carries no name")
	}
}

func TestDateMath(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{24 * time.Hour, "1d"},
		{48 * time.Hour, "2d"},
		{time.Hour, "1h"},
		{90 * time.Minute, "90m"},
		{15 * time.Minute, "15m"},
		{45 * time.Second, "45s"},
	}
	for _, tc := range cases {
		if got := dateMath(tc.in); got != tc.want {
			t.Errorf("dateMath(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
