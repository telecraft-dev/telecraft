package telemetry

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/livecheck"
	seam "github.com/telecraft-dev/telecraft/internal/telemetry"
)

// The live-check primitive against a fake backend (ADR-0034 §6). Every
// case is about one of three things: the reading being the Service's own,
// the records crossing the seam verbatim, and degradation being data with
// a cause rather than a fabricated silence.

// captureAll returns a handler that records the whole request body and
// answers with a fixed document. Unlike capture it drains the body, so a
// request larger than one read is still captured whole.
func captureAll(body *string, response string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		*body = string(raw)
		w.Write([]byte(response))
	}
}

// findingsHit renders one findings hit in the fields shape the query asks
// for: every value an array, the body under body.text.
func findingsHit(body string, attrs map[string]string) string {
	parts := []string{fmt.Sprintf(`"body.text":[%q]`, body)}
	for name, value := range attrs {
		parts = append(parts, fmt.Sprintf(`"attributes.%s":[%q]`, name, value))
	}
	return `{"fields":{` + strings.Join(parts, ",") + `}}`
}

func findingsResponse(total int64, hits ...string) string {
	return `{"status":200,"hits":{"total":{"value":` + jsonInt(total) + `},"hits":[` + strings.Join(hits, ",") + `]}}`
}

// livenessResponse renders the counter-delta aggregations for the tap
// exporter: sent and send-failed totals per signal, in the shapes the
// delta aggregation returns.
func livenessResponse(sentSpans, failedSpans float64) string {
	agg := func(v float64) string {
		return fmt.Sprintf(`{"doc_count":10,"instances":{"sum_other_doc_count":0},"total":{"value":%g}}`, v)
	}
	return `{"status":200,"aggregations":{` +
		`"sent_logs":` + agg(0) + `,"failed_logs":` + agg(0) + `,` +
		`"sent_metrics":` + agg(0) + `,"failed_metrics":` + agg(0) + `,` +
		`"sent_traces":` + agg(sentSpans) + `,"failed_traces":` + agg(failedSpans) + `}}`
}

func TestElasticsearchLiveCheckFindings(t *testing.T) {
	var req string
	es, _ := newFake(t, captureAll(&req, msearchResponse(
		findingsResponse(1, findingsHit(
			"Attribute 'http.request.method' has type 'int', expected 'string'.",
			map[string]string{
				"weaver.finding.id":                              "type_mismatch",
				"weaver.finding.level":                           "violation",
				"weaver.finding.resource.attribute.service.name": "checkout",
			})),
		livenessResponse(3200, 0),
	)))

	got := es.LiveCheckFindings(context.Background(),
		seam.Service{Name: "checkout", Environment: "production"}, time.Hour)

	if !got.Known {
		t.Fatalf("reading not Known: %+v", got)
	}
	if !got.AsOf.Equal(fixedAsOf) {
		t.Errorf("as_of = %v, want %v", got.AsOf, fixedAsOf)
	}
	if len(got.Records) != 1 {
		t.Fatalf("records = %+v, want 1", got.Records)
	}
	rec := got.Records[0]
	if rec.Body != "Attribute 'http.request.method' has type 'int', expected 'string'." {
		t.Errorf("body = %q, want the record body verbatim", rec.Body)
	}
	// The attributes cross the seam verbatim: emitted spellings intact, so
	// the platform-owned normaliser is the one place that interprets them.
	if rec.Attributes["weaver.finding.id"] != "type_mismatch" {
		t.Errorf("attributes = %v, want the emitted spellings verbatim", rec.Attributes)
	}
	if got.Truncated {
		t.Errorf("a complete record set read as Truncated: %+v", got)
	}

	if !got.Liveness.Known {
		t.Fatalf("liveness leg not Known: %+v", got.Liveness)
	}
	if got.Liveness.Sent != 3200 || got.Liveness.SendFailed != 0 {
		t.Errorf("liveness = %+v, want the tap exporter's summed deltas", got.Liveness)
	}
	if !got.Liveness.Fed() {
		t.Error("a tap with sent items does not read as fed")
	}

	// The reading is the Service's own, filtered to the live-check event
	// name through the carried resource attributes; the liveness leg is
	// filtered to the tap exporter's rendered id.
	for _, want := range []string{
		fmt.Sprintf(`"attributes.%s":"checkout"`, livecheck.ServiceAttribute),
		fmt.Sprintf(`"attributes.%s":"production"`, livecheck.EnvironmentAttribute),
		fmt.Sprintf(`"attributes.event.name":%q`, livecheck.EventName),
		fmt.Sprintf(`"attributes.exporter":%q`, livecheck.ExporterID),
		`"metrics.otelcol_exporter_sent_spans"`,
		`"metrics.otelcol_exporter_send_failed_spans"`,
	} {
		if !strings.Contains(req, want) {
			t.Errorf("request does not contain %s\nrequest: %s", want, req)
		}
	}
}

// Send failures on the tap exporter reach the liveness leg: they are what
// tells the evaluator part of the sample never arrived.
func TestElasticsearchLiveCheckReportsSendFailures(t *testing.T) {
	es, _ := newFake(t, capture(new(string), msearchResponse(
		findingsResponse(0),
		livenessResponse(900, 40),
	)))

	got := es.LiveCheckFindings(context.Background(), seam.Service{Name: "checkout"}, time.Hour)
	if got.Liveness.SendFailed != 40 {
		t.Errorf("send failed = %d, want 40", got.Liveness.SendFailed)
	}
}

// Criterion: truncation is always reported. A window holding more finding
// records than the reading fetched says so, never a silently short set.
func TestElasticsearchLiveCheckReportsTruncation(t *testing.T) {
	es, _ := newFake(t, capture(new(string), msearchResponse(
		findingsResponse(1200, findingsHit("m", map[string]string{"weaver.finding.id": "deprecated"})),
		livenessResponse(100, 0),
	)))

	got := es.LiveCheckFindings(context.Background(), seam.Service{Name: "checkout"}, time.Hour)
	if !got.Truncated {
		t.Errorf("a clipped record set did not read as Truncated: %+v", got)
	}
}

// Criterion: knowledge is per leg. A missing logs index degrades the
// findings leg with a cause and leaves the liveness leg standing, and the
// reverse, because the two live in different indices.
func TestElasticsearchLiveCheckDegradesPerLeg(t *testing.T) {
	t.Run("findings leg unreadable", func(t *testing.T) {
		es, _ := newFake(t, capture(new(string), msearchResponse(
			notFound,
			livenessResponse(100, 0),
		)))
		got := es.LiveCheckFindings(context.Background(), seam.Service{Name: "checkout"}, time.Hour)
		if got.Known {
			t.Errorf("the findings leg read Known off a missing index: %+v", got)
		}
		if got.Cause == "" || !strings.Contains(got.Cause, "logs") {
			t.Errorf("cause = %q, want the missing logs index named", got.Cause)
		}
		if !got.Liveness.Known {
			t.Errorf("a findings-leg failure took the liveness leg with it: %+v", got.Liveness)
		}
	})
	t.Run("liveness leg unreadable", func(t *testing.T) {
		es, _ := newFake(t, capture(new(string), msearchResponse(
			findingsResponse(0),
			notFound,
		)))
		got := es.LiveCheckFindings(context.Background(), seam.Service{Name: "checkout"}, time.Hour)
		if !got.Known {
			t.Errorf("a liveness-leg failure took the findings leg with it: %+v", got)
		}
		if got.Liveness.Known {
			t.Errorf("the liveness leg read Known off a missing index: %+v", got.Liveness)
		}
	})
}

// An unreachable backend degrades the whole reading, cause and all, and
// still stamps as_of: degradation is data, never an error (ADR-0008).
func TestElasticsearchLiveCheckUnreachableBackend(t *testing.T) {
	es, srv := newFake(t, func(w http.ResponseWriter, r *http.Request) {})
	srv.Close()

	got := es.LiveCheckFindings(context.Background(), seam.Service{Name: "checkout"}, time.Hour)
	if got.Known || got.Liveness.Known {
		t.Fatalf("an unreachable backend read as Known: %+v", got)
	}
	if got.Cause == "" || !got.AsOf.Equal(fixedAsOf) {
		t.Errorf("degraded reading = %+v, want a cause and a stamped as_of", got)
	}
}

// An unnamed Service is refused rather than answered: the ask would match
// the whole backend, and the reading would not be one Service's own.
func TestElasticsearchLiveCheckRefusesAnUnnamedService(t *testing.T) {
	called := false
	es, _ := newFake(t, func(w http.ResponseWriter, r *http.Request) { called = true })

	got := es.LiveCheckFindings(context.Background(), seam.Service{}, time.Hour)
	if got.Known || got.Liveness.Known {
		t.Errorf("an unnamed ask read as Known: %+v", got)
	}
	if called {
		t.Error("an unnamed ask reached the backend")
	}
}
