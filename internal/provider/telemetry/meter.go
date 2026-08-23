package telemetry

// The metering half of the seam (REQ-050, ADR-0040): one Tier's
// pipeline-grain flow counters, read out of the same self-telemetry the
// rest of ADR-0039 reads, on demand and stored nowhere. No metering
// pipeline exists here: the reading is built, returned, and forgotten.
//
// The counters are cumulative monotonic sums, so a window's throughput is
// a delta, never a sum of datapoints. Each collector incarnation is its
// own series (`service.instance.id` is regenerated on every process
// start, R-4 §2c), which makes last-minus-first per incarnation exact
// across restarts, with no reset heuristic anywhere. The deltas are then
// summed backend-side by a `sum_bucket` pipeline aggregation, which is
// what ADR-0040 §5 means by collectors collapsing into server-side sums:
// the platform receives one number per (signal, exporter), not one per
// collector, and keeps none of it.
//
// The cost is stated rather than hidden. The incarnation buckets the
// delta needs are computed backend-side and travel back in the response
// even though only the collapsed totals are read; both the incarnation
// and exporter fan-outs are capped, and a cap reached is reported
// Truncated rather than quietly rounding a Tier's throughput down.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/selftelemetry"
	seam "github.com/telecraft-dev/telecraft/internal/telemetry"
)

// deltaAggs builds the per-incarnation counter-delta aggregation for one
// counter field: bucket by incarnation, take last minus first inside each,
// and collapse the buckets to one total.
func (e *Elasticsearch) deltaAggs(field string) map[string]any {
	return map[string]any{
		"instances": map[string]any{
			"terms": map[string]any{"field": e.instanceIDField, "size": e.instanceLimit},
			"aggs": map[string]any{
				"first": map[string]any{"min": map[string]any{"field": field}},
				"last":  map[string]any{"max": map[string]any{"field": field}},
				"delta": map[string]any{"bucket_script": map[string]any{
					"buckets_path": map[string]any{"first": "first", "last": "last"},
					"script":       "params.last - params.first",
				}},
			},
		},
		"total": map[string]any{"sum_bucket": map[string]any{"buckets_path": "instances>delta"}},
	}
}

// counterAgg wraps a delta aggregation in the filter that keeps
// incarnations which never reported the counter out of the buckets.
func (e *Elasticsearch) counterAgg(name string) map[string]any {
	field := e.metricValuePrefix + name
	return map[string]any{
		"filter": map[string]any{"exists": map[string]any{"field": field}},
		"aggs":   e.deltaAggs(field),
	}
}

// meterBody builds one signal's metering query: the window and Tier-stamp
// filter, the four whole-Tier counters, the per-exporter out split, the
// freshness reading, and the incarnation count.
func (e *Elasticsearch) meterBody(tier string, window time.Duration, counters selftelemetry.FlowCounters) map[string]any {
	sentField := e.metricValuePrefix + counters.Sent
	out := map[string]any{
		"exporters": map[string]any{
			"terms": map[string]any{"field": e.exporterField, "size": e.exporterLimit},
			"aggs":  e.deltaAggs(sentField),
		},
		// The out-rate is the sum of the exporters' own out-rates: one
		// number per Tier and one per exporter, from the same buckets.
		"total": map[string]any{"sum_bucket": map[string]any{"buckets_path": "exporters>total"}},
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
					map[string]any{"term": map[string]any{e.tierField: tier}},
				},
			},
		},
		"aggs": map[string]any{
			"in":             e.counterAgg(counters.Accepted),
			"refused":        e.counterAgg(counters.Refused),
			"send_failed":    e.counterAgg(counters.SendFailed),
			"enqueue_failed": e.counterAgg(counters.EnqueueFailed),
			"out": map[string]any{
				"filter": map[string]any{"exists": map[string]any{"field": sentField}},
				"aggs":   out,
			},
			"newest":       map[string]any{"max": map[string]any{"field": "@timestamp"}},
			"incarnations": map[string]any{"cardinality": map[string]any{"field": e.instanceIDField}},
		},
	}
}

type bucketTotal struct {
	Value *float64 `json:"value"`
}

type instanceBuckets struct {
	SumOther int64 `json:"sum_other_doc_count"`
}

type counterResult struct {
	Total     bucketTotal     `json:"total"`
	Instances instanceBuckets `json:"instances"`
}

type exporterResult struct {
	Total     bucketTotal `json:"total"`
	Exporters struct {
		SumOther int64 `json:"sum_other_doc_count"`
		Buckets  []struct {
			Key       string          `json:"key"`
			Total     bucketTotal     `json:"total"`
			Instances instanceBuckets `json:"instances"`
		} `json:"buckets"`
	} `json:"exporters"`
}

// meterResponse is one signal's metering response. Only the collapsed
// totals are read; the incarnation buckets the backend computed the
// deltas from are deliberately left unparsed.
type meterResponse struct {
	Status       int `json:"status"`
	Aggregations struct {
		In            counterResult  `json:"in"`
		Refused       counterResult  `json:"refused"`
		SendFailed    counterResult  `json:"send_failed"`
		EnqueueFailed counterResult  `json:"enqueue_failed"`
		Out           exporterResult `json:"out"`
		Newest        struct {
			Value *float64 `json:"value"`
		} `json:"newest"`
		Incarnations struct {
			Value int `json:"value"`
		} `json:"incarnations"`
	} `json:"aggregations"`
	Error json.RawMessage `json:"error"`
}

// items rounds a collapsed counter delta to whole items. The counters are
// integers; the aggregation returns them as floats, and a negative delta
// is impossible on a monotonic counter within one incarnation, so it
// reads as zero rather than as a negative throughput.
func items(t bucketTotal) int64 {
	if t.Value == nil || *t.Value <= 0 {
		return 0
	}
	return int64(*t.Value + 0.5)
}

// Meter issues one _msearch covering the three data signals for one Tier.
// The counters live in the self-telemetry metrics index whichever signal
// they count, so every sub-query hits the same index and differs only in
// which counter fields it reads.
//
// Index resolution is strict, like Observe and ObserveSelf: a pattern
// matching nothing is Known false with a cause, never a metered zero. A
// Tier whose collectors are silent and a provider pointed at the wrong
// index look identical from here, and reporting the second as "nothing
// flowed" is exactly the fabricated value ADR-0040 §6 refuses.
func (e *Elasticsearch) Meter(ctx context.Context, tier string, window time.Duration) seam.Metered {
	asOf := e.now()
	order := seam.Signals()
	index := e.indices[requirements.Metrics]

	var body bytes.Buffer
	for _, kind := range order {
		counters, ok := selftelemetry.FlowCountersFor(kind)
		if !ok {
			continue
		}
		header, _ := json.Marshal(map[string]any{
			"index":              index,
			"allow_no_indices":   false,
			"ignore_unavailable": false,
		})
		query, _ := json.Marshal(e.meterBody(tier, window, counters))
		body.Write(header)
		body.WriteByte('\n')
		body.Write(query)
		body.WriteByte('\n')
	}

	raw, err := e.send(ctx, http.MethodPost, "/_msearch", body.Bytes(), "application/x-ndjson")
	if err != nil {
		return seam.MeterUnknown(asOf, window, err.Error())
	}

	var resp struct {
		Responses []meterResponse `json:"responses"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return seam.MeterUnknown(asOf, window, fmt.Sprintf("undecodable search response: %v", err))
	}
	if len(resp.Responses) != len(order) {
		return seam.MeterUnknown(asOf, window, fmt.Sprintf("search returned %d responses, expected %d", len(resp.Responses), len(order)))
	}

	m := seam.Metered{AsOf: asOf, Window: window, Signals: map[requirements.SignalKind]seam.MeteredSignal{}}
	incarnations := 0
	var incarnationCause string
	incarnationsTruncated := false

	for i, kind := range order {
		r := resp.Responses[i]
		if len(r.Error) > 0 {
			cause := e.signalCause(requirements.Metrics, r.Error)
			m.Signals[kind] = seam.MeteredSignal{Known: false, Cause: cause}
			if incarnationCause == "" {
				incarnationCause = cause
			}
			continue
		}

		agg := r.Aggregations
		sig := seam.MeteredSignal{
			Known:         true,
			In:            items(agg.In.Total),
			Out:           items(agg.Out.Total),
			Refused:       items(agg.Refused.Total),
			SendFailed:    items(agg.SendFailed.Total),
			EnqueueFailed: items(agg.EnqueueFailed.Total),
		}
		if agg.Newest.Value != nil {
			sig.Newest = epochMillis(*agg.Newest.Value)
		}
		for _, b := range agg.Out.Exporters.Buckets {
			if sig.Exporters == nil {
				sig.Exporters = map[string]int64{}
			}
			sig.Exporters[b.Key] = items(b.Total)
			if b.Instances.SumOther > 0 {
				sig.Truncated = true
			}
		}
		if agg.Out.Exporters.SumOther > 0 || agg.In.Instances.SumOther > 0 ||
			agg.Refused.Instances.SumOther > 0 || agg.SendFailed.Instances.SumOther > 0 ||
			agg.EnqueueFailed.Instances.SumOther > 0 {
			sig.Truncated = true
		}
		m.Signals[kind] = sig

		// The incarnation count is Tier-wide: a restart takes the whole
		// process with it, so the highest per-signal count is the Tier's.
		if agg.Incarnations.Value > incarnations {
			incarnations = agg.Incarnations.Value
		}
		if agg.Incarnations.Value >= e.instanceLimit {
			incarnationsTruncated = true
		}
	}

	if incarnationCause != "" && incarnations == 0 {
		m.Incarnations = seam.Incarnations{Known: false, Cause: incarnationCause}
		return m
	}
	m.Incarnations = seam.Incarnations{Known: true, Count: incarnations, Truncated: incarnationsTruncated}
	return m
}
