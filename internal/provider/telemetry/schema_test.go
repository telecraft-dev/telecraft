package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	seam "github.com/telecraft-dev/telecraft/internal/telemetry"
)

// The two ADR-0034 §4 primitives against a fake backend. Every case is
// about one of two things: the reading being the Service's own, and the
// truncation being said out loud.

// capture returns a handler that records the request body and answers with
// a fixed document.
func capture(body *string, response string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw := make([]byte, r.ContentLength)
		r.Body.Read(raw)
		*body = string(raw)
		w.Write([]byte(response))
	}
}

func TestElasticsearchDistinctValues(t *testing.T) {
	var req string
	es, _ := newFake(t, capture(&req, `{"aggregations":{
		"values_0":{"sum_other_doc_count":0,"buckets":[{"key":"POST","doc_count":9},{"key":"GET","doc_count":4}]},
		"values_1":{"sum_other_doc_count":0,"buckets":[{"key":"GET","doc_count":1}]},
		"values_2":{"sum_other_doc_count":0,"buckets":[]}
	}}`))

	got := es.DistinctValues(context.Background(),
		seam.Service{Name: "checkout", Environment: "production"},
		requirements.Logs, "http.request.method", time.Hour)

	if !got.Known {
		t.Fatalf("reading not Known: %+v", got)
	}
	if !got.AsOf.Equal(fixedAsOf) {
		t.Errorf("as_of = %v, want %v", got.AsOf, fixedAsOf)
	}
	if got.Attribute != "http.request.method" {
		t.Errorf("attribute = %q, want the attribute asked for", got.Attribute)
	}
	if want := []string{"GET", "POST"}; !sameStrings(got.Values, want) {
		t.Errorf("values = %v, want %v: sorted, de-duplicated across attribute paths", got.Values, want)
	}
	if got.Truncated {
		t.Errorf("a complete value set read as Truncated: %+v", got)
	}
	if got.Cap != seam.MaxDistinctValues {
		t.Errorf("cap = %d, want the seam's hard cap %d", got.Cap, seam.MaxDistinctValues)
	}

	// The reading is the Service's own: the filter carries the service and
	// the environment term, and one aggregation covers each attribute path.
	for _, want := range []string{
		`"resource.attributes.service.name":"checkout"`,
		`"resource.attributes.deployment.environment.name":"production"`,
		`"attributes.http.request.method"`,
		`"scope.attributes.http.request.method"`,
		fmt.Sprintf(`"size":%d`, seam.MaxDistinctValues),
	} {
		if !strings.Contains(req, want) {
			t.Errorf("values request does not contain %s\nrequest: %s", want, req)
		}
	}
}

// Criterion: hard-capped, with truncation always reported. The aggregation
// says it held terms outside the buckets it returned, and the reading
// carries that through rather than presenting a clipped set as whole.
func TestElasticsearchDistinctValuesReportTruncation(t *testing.T) {
	t.Run("the aggregation held more terms", func(t *testing.T) {
		var req string
		es, _ := newFake(t, capture(&req, `{"aggregations":{"values_0":{"sum_other_doc_count":41,"buckets":[{"key":"POST"}]}}}`))
		got := es.DistinctValues(context.Background(), seam.Service{Name: "checkout"}, requirements.Logs, "enum", time.Hour)
		if !got.Known {
			t.Fatalf("reading not Known: %+v", got)
		}
		if !got.Truncated {
			t.Error("an aggregation reporting other terms must read Truncated: a clipped enum reads as never violated")
		}
	})

	t.Run("the union exceeds the cap", func(t *testing.T) {
		var buckets []string
		for i := 0; i < seam.MaxDistinctValues+7; i++ {
			buckets = append(buckets, fmt.Sprintf(`{"key":"v%03d"}`, i))
		}
		es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"aggregations":{"values_0":{"sum_other_doc_count":0,"buckets":[%s]}}}`,
				strings.Join(buckets, ","))
		})
		got := es.DistinctValues(context.Background(), seam.Service{Name: "checkout"}, requirements.Logs, "enum", time.Hour)
		if len(got.Values) != seam.MaxDistinctValues {
			t.Errorf("returned %d values, want the hard cap %d", len(got.Values), seam.MaxDistinctValues)
		}
		if !got.Truncated {
			t.Error("a set clipped to the cap must read Truncated")
		}
	})
}

// A cap above the seam's own is not a knob: the hard cap is the contract.
func TestElasticsearchDistinctLimitCannotExceedTheSeamCap(t *testing.T) {
	es, err := NewElasticsearch(ElasticsearchConfig{
		Endpoint:      "http://127.0.0.1:1",
		DistinctLimit: seam.MaxDistinctValues * 4,
		GroupLimit:    seam.MaxGroupNames * 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if es.distinctLimit != seam.MaxDistinctValues {
		t.Errorf("distinct limit = %d, want it clamped to %d", es.distinctLimit, seam.MaxDistinctValues)
	}
	if es.groupLimit != seam.MaxGroupNames {
		t.Errorf("group limit = %d, want it clamped to %d", es.groupLimit, seam.MaxGroupNames)
	}
}

// Enum values are not always strings; a boolean or numeric bucket key is
// rendered rather than dropped, because a value the check cannot see is a
// violation it cannot raise.
func TestElasticsearchDistinctValuesRenderNonStringKeys(t *testing.T) {
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"aggregations":{"values_0":{"buckets":[{"key":200},{"key":true},{"key":"ok"}]}}}`))
	})
	got := es.DistinctValues(context.Background(), seam.Service{Name: "checkout"}, requirements.Logs, "code", time.Hour)
	if want := []string{"200", "ok", "true"}; !sameStrings(got.Values, want) {
		t.Errorf("values = %v, want %v", got.Values, want)
	}
}

func TestElasticsearchDistinctValuesDegraded(t *testing.T) {
	t.Run("missing index", func(t *testing.T) {
		es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":{"type":"index_not_found_exception","reason":"no such index"},"status":404}`))
		})
		got := es.DistinctValues(context.Background(), seam.Service{Name: "checkout"}, requirements.Traces, "enum", time.Hour)
		assertUnknownValues(t, got)
	})

	// An unaggregatable mapping fails the search, and the reading carries
	// the backend's own reason. This is the case the fidelity rule is
	// about: an empty value set here would read as a clean enum.
	t.Run("the field cannot be aggregated", func(t *testing.T) {
		es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"error":{"type":"illegal_argument_exception","reason":"Fielddata is disabled on [attributes.enum]"}}`))
		})
		got := es.DistinctValues(context.Background(), seam.Service{Name: "checkout"}, requirements.Logs, "enum", time.Hour)
		assertUnknownValues(t, got)
		if !strings.Contains(got.Cause, "Fielddata") {
			t.Errorf("cause %q does not carry the backend's own reason", got.Cause)
		}
	})

	t.Run("unreachable backend", func(t *testing.T) {
		es, err := NewElasticsearch(ElasticsearchConfig{Endpoint: "http://127.0.0.1:1"})
		if err != nil {
			t.Fatal(err)
		}
		es.now = func() time.Time { return fixedAsOf }
		assertUnknownValues(t, es.DistinctValues(context.Background(),
			seam.Service{Name: "checkout"}, requirements.Logs, "enum", time.Hour))
	})

	t.Run("no attribute named", func(t *testing.T) {
		es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
			t.Error("an unnamed attribute must not reach the backend")
		})
		assertUnknownValues(t, es.DistinctValues(context.Background(),
			seam.Service{Name: "checkout"}, requirements.Logs, "", time.Hour))
	})
}

func assertUnknownValues(t *testing.T, got seam.DistinctValues) {
	t.Helper()
	if got.Known || got.Cause == "" {
		t.Fatalf("want Known=false with a cause, got %+v", got)
	}
	if len(got.Values) != 0 {
		t.Errorf("fabricated values on a degraded reading: %v", got.Values)
	}
	if !got.AsOf.Equal(fixedAsOf) {
		t.Errorf("degraded reading as_of = %v, want %v", got.AsOf, fixedAsOf)
	}
}

// Span names and event names are field values, so they come from a terms
// aggregation over the whole window, on the field the signal's grouping key
// lands in.
func TestElasticsearchGroupNamesFromValues(t *testing.T) {
	cases := []struct {
		kind  requirements.SignalKind
		key   seam.GroupKey
		field string
	}{
		{requirements.Traces, seam.SpanName, `"field":"name"`},
		{requirements.Logs, seam.EventName, `"field":"attributes.event.name"`},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			var req string
			es, _ := newFake(t, capture(&req, `{"aggregations":{"groups":{"sum_other_doc_count":0,"buckets":[
				{"key":"POST /pay"},{"key":"GET /cart"}]}}}`))

			got := es.GroupNames(context.Background(), seam.Service{Name: "checkout"}, tc.kind, time.Hour)

			if !got.Known {
				t.Fatalf("reading not Known: %+v", got)
			}
			if got.Key != tc.key {
				t.Errorf("key = %q, want %q", got.Key, tc.key)
			}
			if want := []string{"GET /cart", "POST /pay"}; !sameStrings(got.Names, want) {
				t.Errorf("names = %v, want %v sorted", got.Names, want)
			}
			if got.Truncated {
				t.Errorf("a complete group set read as Truncated: %+v", got)
			}
			if got.SampledRecords != 0 || got.TotalRecords != 0 {
				t.Errorf("an aggregated reading reports a sample it never took: %+v", got)
			}
			for _, want := range []string{tc.field, `"resource.attributes.service.name":"checkout"`} {
				if !strings.Contains(req, want) {
					t.Errorf("groups request does not contain %s\nrequest: %s", want, req)
				}
			}
		})
	}
}

func TestElasticsearchGroupNamesReportTruncation(t *testing.T) {
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"aggregations":{"groups":{"sum_other_doc_count":900,"buckets":[{"key":"GET /cart"}]}}}`))
	})
	got := es.GroupNames(context.Background(), seam.Service{Name: "checkout"}, requirements.Traces, time.Hour)
	if !got.Truncated {
		t.Errorf("an aggregation holding groups outside its buckets must read Truncated: %+v", got)
	}
}

// A metric's name is a document field name rather than a value, so metric
// groups come from a bounded sample of the Service's own records, and the
// window holding more than the sample is reported as truncation.
func TestElasticsearchGroupNamesForMetricsSampleTheRecords(t *testing.T) {
	var req string
	es, _ := newFake(t, capture(&req, `{"hits":{"total":{"value":900},"hits":[
		{"fields":{"metrics.http.server.request.duration":[3],"resource.attributes.service.name":["checkout"]}},
		{"fields":{"metrics.system.cpu.time":[1],"metrics.http.server.request.duration":[4]}}
	]}}`))

	got := es.GroupNames(context.Background(), seam.Service{Name: "checkout"}, requirements.Metrics, time.Hour)

	if !got.Known {
		t.Fatalf("reading not Known: %+v", got)
	}
	if got.Key != seam.MetricName {
		t.Errorf("key = %q, want %q", got.Key, seam.MetricName)
	}
	if want := []string{"http.server.request.duration", "system.cpu.time"}; !sameStrings(got.Names, want) {
		t.Errorf("names = %v, want %v: the value prefix stripped, non-metric fields dropped", got.Names, want)
	}
	if got.SampledRecords != 2 || got.TotalRecords != 900 || !got.Truncated {
		t.Errorf("sampling not reported honestly: %+v: 2 of 900 records inspected must read Truncated", got)
	}
	for _, want := range []string{`"_source":false`, `"metrics.*"`, `"resource.attributes.service.name":"checkout"`} {
		if !strings.Contains(req, want) {
			t.Errorf("metric groups request does not contain %s\nrequest: %s", want, req)
		}
	}
}

func TestElasticsearchGroupNamesForMetricsComplete(t *testing.T) {
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"hits":{"total":{"value":1},"hits":[{"fields":{"metrics.up":[1]}}]}}`))
	})
	got := es.GroupNames(context.Background(), seam.Service{Name: "checkout"}, requirements.Metrics, time.Hour)
	if !got.Known || got.Truncated {
		t.Errorf("every record inspected must read Known and not Truncated: %+v", got)
	}
}

func TestElasticsearchGroupNamesDegraded(t *testing.T) {
	assert := func(t *testing.T, got seam.GroupNames, kind requirements.SignalKind) {
		t.Helper()
		if got.Known || got.Cause == "" {
			t.Fatalf("want Known=false with a cause, got %+v", got)
		}
		if len(got.Names) != 0 {
			t.Errorf("fabricated names on a degraded reading: %v", got.Names)
		}
		if got.Key != seam.GroupKeyFor(kind) {
			t.Errorf("key = %q, want %q: a degraded reading still says which dimension it could not read",
				got.Key, seam.GroupKeyFor(kind))
		}
		if !got.AsOf.Equal(fixedAsOf) {
			t.Errorf("degraded reading as_of = %v, want %v", got.AsOf, fixedAsOf)
		}
	}

	t.Run("missing index", func(t *testing.T) {
		es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":{"type":"index_not_found_exception","reason":"no such index"},"status":404}`))
		})
		assert(t, es.GroupNames(context.Background(), seam.Service{Name: "checkout"}, requirements.Traces, time.Hour),
			requirements.Traces)
	})

	t.Run("missing metrics index", func(t *testing.T) {
		es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error":{"type":"index_not_found_exception","reason":"no such index"},"status":404}`))
		})
		assert(t, es.GroupNames(context.Background(), seam.Service{Name: "checkout"}, requirements.Metrics, time.Hour),
			requirements.Metrics)
	})

	t.Run("a signal with no grouping key", func(t *testing.T) {
		es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
			t.Error("a signal with no grouping key must not reach the backend")
		})
		got := es.GroupNames(context.Background(), seam.Service{Name: "checkout"},
			requirements.SignalKind("profiles"), time.Hour)
		if got.Known || got.Cause == "" {
			t.Fatalf("want Known=false with a cause, got %+v", got)
		}
	})
}

// sameStrings compares two string sets in order.
func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// Criterion (ADR-0034 §4): a reading that cannot be scoped to one Service
// is Known false with a cause, never the index-scoped answer. An unnamed
// Service is the case that would otherwise reach the backend as a filter
// matching everything the index holds.
func TestElasticsearchRefusesAnUnnamedService(t *testing.T) {
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("an unnamed Service must not reach the backend: the answer would be the index's, not a Service's")
	})

	values := es.DistinctValues(context.Background(), seam.Service{}, requirements.Logs, "enum", time.Hour)
	if values.Known || !strings.Contains(values.Cause, "service.name") {
		t.Errorf("want Known=false naming what was missing, got %+v", values)
	}
	names := es.GroupNames(context.Background(), seam.Service{}, requirements.Logs, time.Hour)
	if names.Known || !strings.Contains(names.Cause, "service.name") {
		t.Errorf("want Known=false naming what was missing, got %+v", names)
	}
}
