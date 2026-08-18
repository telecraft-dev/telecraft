package telemetry

// The self-telemetry half of the seam (REQ-053, ADR-0039): the platform
// reads collector health from the adopter's Elasticsearch exactly the way
// it reads any other telemetry — same indices, same round-trip discipline,
// no privileged side channel. The provider reports the component-identity
// attribute combinations verbatim; interpreting them is the one
// platform-owned normaliser's job (internal/selftelemetry), never this
// package's — a second backend must not become a second opinion on join
// keys.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	"github.com/telecraft-dev/telecraft/internal/selftelemetry"
	seam "github.com/telecraft-dev/telecraft/internal/telemetry"
)

// identityFields returns the document fields holding one signal's
// component-identity attributes. In OTel-native mapping mode the legacy
// join keys are datapoint attributes (attributes.*) and the
// otelcol.component.* keys are instrumentation scope attributes
// (scope.attributes.*) — the spellings themselves are platform knowledge,
// owned by the normaliser package (ADR-0039 §3).
func (e *Elasticsearch) identityFields(kind requirements.SignalKind) map[string]string {
	fields := map[string]string{}
	if kind == requirements.Metrics {
		for _, name := range selftelemetry.MetricIdentityAttributes {
			fields[name] = "attributes." + name
		}
	}
	// Scope attributes ride every signal: primary on logs, and present on
	// metrics too once the mirrored newPipelineTelemetry flag flips
	// (ADR-0039 §4) — reading them costs nothing while the gate is off.
	for _, name := range selftelemetry.ScopeIdentityAttributes {
		fields[name] = "scope.attributes." + name
	}
	return fields
}

// observeSelfBody builds one signal's count-commits-and-identities query:
// zero hits fetched, exact totals, the window and Tier-stamp filter, a
// terms aggregation over the commit stamps and a composite aggregation
// over the identity attributes — missing buckets included, because
// identity-dropping components and collector-level telemetry are readings
// too (R-4 §5.2).
func (e *Elasticsearch) observeSelfBody(tier string, window time.Duration, kind requirements.SignalKind) map[string]any {
	sources := []any{}
	fields := e.identityFields(kind)
	for _, name := range sortedFieldNames(fields) {
		sources = append(sources, map[string]any{
			name: map[string]any{"terms": map[string]any{"field": fields[name], "missing_bucket": true}},
		})
	}
	return map[string]any{
		"size":             0,
		"track_total_hits": true,
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
			"commits": map[string]any{
				"terms": map[string]any{"field": e.commitField, "size": e.commitLimit},
			},
			"components": map[string]any{
				"composite": map[string]any{"size": e.identityLimit, "sources": sources},
			},
		},
	}
}

func sortedFieldNames(fields map[string]string) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ObserveSelf issues one _msearch covering the internal signals (logs and
// metrics — traces stay off in v1, ADR-0039 §1) for one Tier, matched on
// the telecraft.tier resource stamp.
//
// Index resolution is strict, like Observe: a pattern matching nothing is
// Known false with a cause, never an observed silence — the provider
// cannot tell "no self-telemetry has ever landed" from "pointed at the
// wrong index", and transport loss must read as unknown, never as failure
// (ADR-0039 §2). Identity combinations and commit stamps beyond the
// per-signal caps are reported Truncated, never silently dropped.
func (e *Elasticsearch) ObserveSelf(ctx context.Context, tier string, window time.Duration) seam.SelfObserved {
	asOf := e.now()
	order := seam.SelfSignals()

	var body bytes.Buffer
	for _, kind := range order {
		header, _ := json.Marshal(map[string]any{
			"index":              e.indices[kind],
			"allow_no_indices":   false,
			"ignore_unavailable": false,
		})
		query, _ := json.Marshal(e.observeSelfBody(tier, window, kind))
		body.Write(header)
		body.WriteByte('\n')
		body.Write(query)
		body.WriteByte('\n')
	}

	raw, err := e.send(ctx, http.MethodPost, "/_msearch", body.Bytes(), "application/x-ndjson")
	if err != nil {
		return seam.SelfUnknown(asOf, window, err.Error())
	}

	var resp struct {
		Responses []struct {
			Status int `json:"status"`
			Hits   struct {
				Total struct {
					Value int64 `json:"value"`
				} `json:"total"`
			} `json:"hits"`
			Aggregations struct {
				Commits struct {
					SumOther int64 `json:"sum_other_doc_count"`
					Buckets  []struct {
						Key      any   `json:"key"`
						DocCount int64 `json:"doc_count"`
					} `json:"buckets"`
				} `json:"commits"`
				Components struct {
					Buckets []struct {
						Key      map[string]any `json:"key"`
						DocCount int64          `json:"doc_count"`
					} `json:"buckets"`
				} `json:"components"`
			} `json:"aggregations"`
			Error json.RawMessage `json:"error"`
		} `json:"responses"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return seam.SelfUnknown(asOf, window, fmt.Sprintf("undecodable search response: %v", err))
	}
	if len(resp.Responses) != len(order) {
		return seam.SelfUnknown(asOf, window, fmt.Sprintf("search returned %d responses, expected %d", len(resp.Responses), len(order)))
	}

	obs := seam.SelfObserved{AsOf: asOf, Window: window, Signals: map[requirements.SignalKind]seam.SelfSignal{}}
	for i, kind := range order {
		r := resp.Responses[i]
		if len(r.Error) > 0 {
			obs.Signals[kind] = seam.SelfSignal{Known: false, Cause: e.signalCause(kind, r.Error)}
			continue
		}

		total := r.Hits.Total.Value
		sig := seam.SelfSignal{Known: true, Present: total > 0, Volume: total}

		for _, b := range r.Aggregations.Commits.Buckets {
			if sha, ok := b.Key.(string); ok {
				if sig.Commits == nil {
					sig.Commits = map[string]int64{}
				}
				sig.Commits[sha] = b.DocCount
			}
		}
		if r.Aggregations.Commits.SumOther > 0 {
			sig.Truncated = true
		}

		for _, b := range r.Aggregations.Components.Buckets {
			attrs := map[string]string{}
			for name, v := range b.Key {
				// missing_bucket renders an absent attribute as null;
				// absence is shape, not data (R-4 §5.2).
				if s, ok := v.(string); ok {
					attrs[name] = s
				}
			}
			sig.Components = append(sig.Components, seam.ComponentTelemetry{Attributes: attrs, Records: b.DocCount})
		}
		if len(r.Aggregations.Components.Buckets) >= e.identityLimit {
			sig.Truncated = true
		}
		obs.Signals[kind] = sig
	}
	return obs
}
