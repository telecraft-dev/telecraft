package telemetry

// The live-check half of the seam (ADR-0034 §5 and §6): the finding
// records the adopter-deployed live-check tap emitted for one Service,
// read out of the same logs index every other reading comes from, plus
// the liveness leg, read out of the same self-telemetry counters Meter
// reads. No privileged side channel (REQ-053's rule): the tap's findings
// land in the backend as ordinary log records and are read back like any
// other telemetry.
//
// The provider reports each record verbatim, body and attributes as the
// backend recorded them; interpreting the emitted spellings is the one
// platform-owned normaliser's job (internal/livecheck), never this
// package's. The spellings this file does use, the record event name and
// the tap exporter's rendered id, come from that package for the same
// reason: a provider that spelt them itself would be a second opinion.
//
// The liveness leg is deliberately estate-grained: it sums the sent and
// send-failed counters for the tap exporter across every Tier that tees
// to it, because the seam call is scoped to a Service and which gateway
// Tier carried that Service's sample is topology this provider does not
// hold. What it answers is "was the tap fed at all, and did sends fail
// anywhere", which is the question the evaluator asks of it; it does not
// prove this Service's records were in the sample, and the evaluator
// never reads it as though it did.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/telecraft-dev/telecraft/internal/livecheck"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/selftelemetry"
	seam "github.com/telecraft-dev/telecraft/internal/telemetry"
)

// liveCheckFindingsBody builds the findings query: up to liveCheckLimit
// records carrying the live-check event name, filtered to the window and,
// through the carried resource attributes, to the one Service the reading
// answers for.
func (e *Elasticsearch) liveCheckFindingsBody(service seam.Service, window time.Duration) map[string]any {
	filter := []any{
		map[string]any{"range": map[string]any{
			"@timestamp": map[string]any{"gte": "now-" + dateMath(window)},
		}},
		map[string]any{"term": map[string]any{e.eventNameField: livecheck.EventName}},
		map[string]any{"term": map[string]any{"attributes." + livecheck.ServiceAttribute: service.Name}},
	}
	if service.Environment != "" {
		filter = append(filter, map[string]any{"term": map[string]any{"attributes." + livecheck.EnvironmentAttribute: service.Environment}})
	}
	return map[string]any{
		"size":             e.liveCheckLimit,
		"track_total_hits": true,
		"_source":          false,
		"fields":           []string{e.logBodyField, "attributes.*"},
		"query": map[string]any{
			"bool": map[string]any{"filter": filter},
		},
	}
}

// liveCheckLivenessBody builds the liveness query: the sent and
// send-failed counter deltas for the tap exporter over the window, one
// pair per signal, using the same per-incarnation delta shape Meter uses.
func (e *Elasticsearch) liveCheckLivenessBody(window time.Duration) map[string]any {
	aggs := map[string]any{}
	for _, kind := range seam.Signals() {
		counters, ok := selftelemetry.FlowCountersFor(kind)
		if !ok {
			continue
		}
		aggs["sent_"+string(kind)] = e.counterAgg(counters.Sent)
		aggs["failed_"+string(kind)] = e.counterAgg(counters.SendFailed)
	}
	return map[string]any{
		"size":             0,
		"track_total_hits": false,
		"query": map[string]any{
			"bool": map[string]any{
				"filter": []any{
					map[string]any{"range": map[string]any{
						"@timestamp": map[string]any{"gte": "now-" + dateMath(window)},
					}},
					map[string]any{"term": map[string]any{e.exporterField: livecheck.ExporterID}},
				},
			},
		},
		"aggs": aggs,
	}
}

// LiveCheckFindings issues one _msearch with two legs: the finding records
// for one Service from the logs index, and the tap exporter's send
// counters from the self-telemetry metrics index.
//
// Knowledge is per leg, because the two live in different indices and can
// degrade independently: a missing logs index says nothing about the
// counters, and the reverse. Index resolution is strict on both, like
// every other reading here: a pattern matching nothing is Known false
// with a cause, never an observed silence, because "no finding has ever
// landed" and "pointed at the wrong index" look identical from here and
// only one of them is clean.
func (e *Elasticsearch) LiveCheckFindings(ctx context.Context, service seam.Service, window time.Duration) seam.LiveCheckFindings {
	asOf := e.now()
	if service.Name == "" {
		return seam.LiveCheckUnknown(asOf, window,
			seam.NotServiceScoped(service, "no service.name was given, so the reading would not be one Service's"))
	}

	var body bytes.Buffer
	for _, part := range []struct {
		index string
		query map[string]any
	}{
		{e.indices[requirements.Logs], e.liveCheckFindingsBody(service, window)},
		{e.indices[requirements.Metrics], e.liveCheckLivenessBody(window)},
	} {
		header, _ := json.Marshal(map[string]any{
			"index":              part.index,
			"allow_no_indices":   false,
			"ignore_unavailable": false,
		})
		query, _ := json.Marshal(part.query)
		body.Write(header)
		body.WriteByte('\n')
		body.Write(query)
		body.WriteByte('\n')
	}

	raw, err := e.send(ctx, http.MethodPost, "/_msearch", body.Bytes(), "application/x-ndjson")
	if err != nil {
		return seam.LiveCheckUnknown(asOf, window, err.Error())
	}

	var resp struct {
		Responses []json.RawMessage `json:"responses"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return seam.LiveCheckUnknown(asOf, window, fmt.Sprintf("undecodable search response: %v", err))
	}
	if len(resp.Responses) != 2 {
		return seam.LiveCheckUnknown(asOf, window, fmt.Sprintf("search returned %d responses, expected 2", len(resp.Responses)))
	}

	reading := seam.LiveCheckFindings{AsOf: asOf, Window: window}
	e.readLiveCheckRecords(resp.Responses[0], &reading)
	reading.Liveness = e.readLiveCheckLiveness(resp.Responses[1])
	return reading
}

// readLiveCheckRecords fills the findings leg from the logs response.
func (e *Elasticsearch) readLiveCheckRecords(raw json.RawMessage, reading *seam.LiveCheckFindings) {
	var resp struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				Fields map[string]json.RawMessage `json:"fields"`
			} `json:"hits"`
		} `json:"hits"`
		Error json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		reading.Cause = fmt.Sprintf("undecodable search response: %v", err)
		return
	}
	if len(resp.Error) > 0 {
		reading.Cause = e.signalCause(requirements.Logs, resp.Error)
		return
	}

	reading.Known = true
	for _, hit := range resp.Hits.Hits {
		rec := seam.LiveCheckRecord{}
		attrs := map[string]string{}
		for field, value := range hit.Fields {
			if field == e.logBodyField {
				rec.Body = firstValue(value)
				continue
			}
			if strings.HasPrefix(field, "attributes.") {
				// Dynamic string mappings expose a .keyword multi-field
				// beside the field itself; collapse it back onto the
				// attribute name rather than reporting a phantom.
				name := strings.TrimSuffix(strings.TrimPrefix(field, "attributes."), ".keyword")
				attrs[name] = firstValue(value)
			}
		}
		if len(attrs) > 0 {
			rec.Attributes = attrs
		}
		reading.Records = append(reading.Records, rec)
	}
	reading.Truncated = resp.Hits.Total.Value > int64(len(resp.Hits.Hits))
}

// readLiveCheckLiveness fills the liveness leg from the metrics response,
// summing the per-signal deltas: the leg answers for the tap, not for one
// signal, because a tap is fed by whatever the teed branch carries.
func (e *Elasticsearch) readLiveCheckLiveness(raw json.RawMessage) seam.LiveCheckLiveness {
	var resp struct {
		Aggregations map[string]counterResult `json:"aggregations"`
		Error        json.RawMessage          `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return seam.LiveCheckLiveness{Known: false, Cause: fmt.Sprintf("undecodable search response: %v", err)}
	}
	if len(resp.Error) > 0 {
		return seam.LiveCheckLiveness{Known: false, Cause: e.signalCause(requirements.Metrics, resp.Error)}
	}

	live := seam.LiveCheckLiveness{Known: true}
	for _, kind := range seam.Signals() {
		live.Sent += items(resp.Aggregations["sent_"+string(kind)].Total)
		live.SendFailed += items(resp.Aggregations["failed_"+string(kind)].Total)
	}
	return live
}

// firstValue renders the first value of one fields entry as a string. The
// fields API returns every value as a JSON array; live-check attributes
// are single valued, and a numeric or boolean value is rendered in its
// literal form rather than dropped, because verbatim is the contract.
func firstValue(raw json.RawMessage) string {
	var values []any
	if err := json.Unmarshal(raw, &values); err != nil || len(values) == 0 {
		return ""
	}
	switch v := values[0].(type) {
	case string:
		return v
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", v), "0"), ".")
	case bool:
		return fmt.Sprintf("%t", v)
	default:
		out, _ := json.Marshal(v)
		return string(out)
	}
}
