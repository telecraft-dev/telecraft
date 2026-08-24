package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	seam "github.com/telecraft-dev/telecraft/internal/telemetry"
)

// The two schema-conformance primitives that join AttributeNames on the
// seam (ADR-0034 §4): DistinctValues, the value set one attribute carries,
// and GroupNames, the values of the grouping key a signal is grouped by.
//
// Both are service-scoped the way every other reading here is: the filter
// carries the Service's service.name term and, where the reading is
// narrowed to one Environment, the environment term too (ADR-0033). The
// index-scoped shortcuts are available and are deliberately not taken. A
// terms aggregation with no service filter, or the field-capabilities API,
// would answer faster and would answer a different question: what the
// index holds, not what this Service emitted. ADR-0034 §4 sanctions that
// union as a screening fast path with a service-scoped follow-up, never as
// the reading itself, so nothing here returns one.

// unnamedService is why an unnamed Service is refused rather than answered.
// The filter these primitives build is a term on the service-name field, and
// a term on the empty string matches nothing while its absence would match
// the whole index. Neither is this Service's reading, and one of them is the
// index-scoped union ADR-0034 §4 forbids being passed off as one.
const unnamedService = "no service.name was given, so the reading would be the whole index rather than one Service's"

// DistinctValues reads the values one attribute carries for one Service,
// signal and window, through one terms aggregation per configured
// attribute path.
//
// Where the value set is longer than the cap, or the aggregation reports
// values outside the buckets it returned, the reading says Truncated: a
// clipped set presented as whole would turn a missing enum value into a
// pass (ADR-0034 §4).
//
// Aggregating requires an aggregatable mapping, which is what OTLP ingest
// in OTel-native mode produces for attributes. A cluster that maps them as
// full text instead fails the search, and the reading is Known false
// carrying the backend's own reason. That is the honest answer: this
// provider cannot tell an unaggregatable field from an empty one without
// asking, and an empty value set is the shape a conformance check reads as
// "the enum was never violated".
func (e *Elasticsearch) DistinctValues(ctx context.Context, service seam.Service, kind requirements.SignalKind, attribute string, window time.Duration) seam.DistinctValues {
	asOf := e.now()
	unknown := func(cause string) seam.DistinctValues {
		reading := seam.DistinctValuesUnknown(asOf, window, attribute, cause)
		reading.Cap = e.distinctLimit
		return reading
	}
	if attribute == "" {
		return unknown("no attribute was named: a value set of nothing is not a reading")
	}
	if service.Name == "" {
		return unknown(seam.NotServiceScoped(service, unnamedService))
	}
	if _, ok := e.indices[kind]; !ok {
		return unknown(fmt.Sprintf("no index is configured for %s", kind))
	}

	body := e.observeBody(service, window, nil)
	aggs := map[string]any{}
	for i, prefix := range e.attributePaths {
		aggs[fmt.Sprintf("values_%d", i)] = map[string]any{
			"terms": map[string]any{"field": prefix + attribute, "size": e.distinctLimit},
		}
	}
	body["aggs"] = aggs

	resp, cause := e.terms(ctx, kind, body)
	if cause != "" {
		return unknown(cause)
	}

	values := map[string]bool{}
	truncated := false
	for _, agg := range resp.Aggregations {
		if agg.SumOtherDocCount > 0 {
			truncated = true
		}
		for _, bucket := range agg.Buckets {
			values[bucketKey(bucket.Key)] = true
		}
	}

	reading := seam.DistinctValues{
		Known:     true,
		AsOf:      asOf,
		Window:    window,
		Attribute: attribute,
		Cap:       e.distinctLimit,
	}
	reading.Values, reading.Truncated = capped(values, e.distinctLimit, truncated)
	return reading
}

// GroupNames reads which spans, metrics or events arrived for one Service,
// signal and window.
//
// Two mechanisms, because the grouping key lands two ways. Span names and
// event names are field values, so they come from a terms aggregation over
// the whole window, exact up to the cap. A metric's name is a document
// field name rather than a value, so metric groups come from the same
// bounded sample of records AttributeNames reads, and the reading says
// Truncated whenever the window holds more records than the sample did.
// Either way the truncation is reported, and either way the filter is the
// Service's own.
func (e *Elasticsearch) GroupNames(ctx context.Context, service seam.Service, kind requirements.SignalKind, window time.Duration) seam.GroupNames {
	asOf := e.now()
	key := seam.GroupKeyFor(kind)
	unknown := func(cause string) seam.GroupNames {
		return seam.GroupNamesUnknown(asOf, window, kind, cause)
	}
	if key == "" {
		return unknown(fmt.Sprintf("%s has no grouping key", kind))
	}
	if service.Name == "" {
		return unknown(seam.NotServiceScoped(service, unnamedService))
	}
	if _, ok := e.indices[kind]; !ok {
		return unknown(fmt.Sprintf("no index is configured for %s", kind))
	}

	if kind == requirements.Metrics {
		return e.metricGroupNames(ctx, service, window, asOf, key, unknown)
	}

	field := e.spanNameField
	if kind == requirements.Logs {
		field = e.eventNameField
	}
	body := e.observeBody(service, window, nil)
	body["aggs"] = map[string]any{
		"groups": map[string]any{
			"terms": map[string]any{"field": field, "size": e.groupLimit},
		},
	}

	resp, cause := e.terms(ctx, kind, body)
	if cause != "" {
		return unknown(cause)
	}

	names := map[string]bool{}
	truncated := false
	for _, agg := range resp.Aggregations {
		if agg.SumOtherDocCount > 0 {
			truncated = true
		}
		for _, bucket := range agg.Buckets {
			names[bucketKey(bucket.Key)] = true
		}
	}

	reading := seam.GroupNames{Known: true, AsOf: asOf, Window: window, Key: key}
	reading.Names, reading.Truncated = capped(names, e.groupLimit, truncated)
	return reading
}

// metricGroupNames unions the metric-value field names carried by up to
// sampleSize records, which is where a metric's name lives when the
// datapoint's value is stored under a field named after it.
func (e *Elasticsearch) metricGroupNames(ctx context.Context, service seam.Service, window time.Duration, asOf time.Time, key seam.GroupKey, unknown func(string) seam.GroupNames) seam.GroupNames {
	body := e.observeBody(service, window, nil)
	body["size"] = e.sampleSize
	body["_source"] = false
	body["fields"] = []string{e.metricValuePrefix + "*"}
	delete(body, "aggs")

	raw, cause := e.search(ctx, requirements.Metrics, body)
	if cause != "" {
		return unknown(cause)
	}

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
		return unknown(fmt.Sprintf("undecodable search response: %v", err))
	}
	if len(resp.Error) > 0 {
		return unknown(e.signalCause(requirements.Metrics, resp.Error))
	}

	names := map[string]bool{}
	for _, hit := range resp.Hits.Hits {
		for field := range hit.Fields {
			if !strings.HasPrefix(field, e.metricValuePrefix) {
				continue
			}
			name := strings.TrimPrefix(field, e.metricValuePrefix)
			if name != "" {
				names[name] = true
			}
		}
	}

	reading := seam.GroupNames{
		Known:          true,
		AsOf:           asOf,
		Window:         window,
		Key:            key,
		SampledRecords: int64(len(resp.Hits.Hits)),
		TotalRecords:   resp.Hits.Total.Value,
	}
	sampled := reading.TotalRecords > reading.SampledRecords
	reading.Names, reading.Truncated = capped(names, e.groupLimit, sampled)
	return reading
}

// termsResponse is the shape both value-set and group-set aggregations come
// back in. sum_other_doc_count is the honesty channel: non-zero means the
// aggregation held terms outside the buckets it returned.
type termsResponse struct {
	Aggregations map[string]struct {
		Buckets []struct {
			Key json.RawMessage `json:"key"`
		} `json:"buckets"`
		SumOtherDocCount int64 `json:"sum_other_doc_count"`
	} `json:"aggregations"`
	Error json.RawMessage `json:"error"`
}

// terms issues one aggregation search and decodes it, returning a cause
// rather than an error: degradation is data in the reading (ADR-0008).
func (e *Elasticsearch) terms(ctx context.Context, kind requirements.SignalKind, body map[string]any) (termsResponse, string) {
	var resp termsResponse
	raw, cause := e.search(ctx, kind, body)
	if cause != "" {
		return resp, cause
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return resp, fmt.Sprintf("undecodable search response: %v", err)
	}
	if len(resp.Error) > 0 {
		return resp, e.signalCause(kind, resp.Error)
	}
	return resp, ""
}

// search posts one _search for one signal with strict index resolution, and
// renders any failure as a reading cause.
func (e *Elasticsearch) search(ctx context.Context, kind requirements.SignalKind, body map[string]any) ([]byte, string) {
	raw, _ := json.Marshal(body)
	path := fmt.Sprintf("/%s/_search?allow_no_indices=false&ignore_unavailable=false", e.indices[kind])
	out, err := e.send(ctx, http.MethodPost, path, raw, "application/json")
	if err != nil {
		// A missing index surfaces as a whole-request 404 here rather than
		// as a per-response error; give it the same cause.
		if strings.Contains(err.Error(), "index_not_found") {
			return nil, e.signalCause(kind, json.RawMessage(err.Error()))
		}
		return nil, err.Error()
	}
	return out, ""
}

// bucketKey renders one aggregation bucket key as a string. Keys come back
// typed, so a boolean or numeric enum is rendered verbatim rather than
// dropped: a value the check cannot see is a violation it cannot raise.
func bucketKey(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return strings.Trim(string(raw), `"`)
}

// capped sorts a value set, clips it to the cap, and folds the clipping
// into the truncation the reading already carries. Truncation is always
// reported (ADR-0034 §4), so the clip can never be silent.
func capped(set map[string]bool, limit int, truncated bool) ([]string, bool) {
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	if len(out) > limit {
		out = out[:limit]
		truncated = true
	}
	return out, truncated
}
