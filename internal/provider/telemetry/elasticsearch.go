// Package telemetry holds provider-side implementations of the
// TelemetryProvider seam (internal/telemetry). Everything vendor-shaped is
// confined to this tree (ADR-0001): index patterns, field layouts, the query
// DSL. The core sees only seam readings, so a second backend sits beside the
// first without the evaluator changing at all.
package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/telecraft-dev/telecraft/internal/requirements"
	seam "github.com/telecraft-dev/telecraft/internal/telemetry"
)

// Elasticsearch observes telemetry that landed in Elasticsearch. First-party,
// never privileged (ADR-0001).
//
// It speaks the two HTTP endpoints it needs (_msearch, _search) directly
// rather than through the official go-elasticsearch client: the client's
// mandatory product check would force every test double to emit a response
// header whose name is itself a banned bare-company word under the ADR-0001
// provider lint, and two endpoints do not justify a dependency. go.mod stays
// untouched.
//
// Field naming is configurable rather than hardcoded because Elasticsearch's
// OTLP ingest lands resource attributes differently depending on mapping
// mode: OTel-native mode (the default here) puts them under
// resource.attributes.*, while ECS mode promotes service.name to the top
// level. Guessing wrong yields an estate where every Service looks silent —
// the most alarming and least useful failure this provider could have.
type Elasticsearch struct {
	endpoint string
	apiKey   string
	http     *http.Client

	indices          map[requirements.SignalKind]string
	serviceNameField string
	attributePaths   []string
	sampleSize       int

	// now stamps AsOf onto every reading; tests pin it.
	now func() time.Time
}

// ElasticsearchConfig configures the Elasticsearch implementation. Only
// Endpoint is mandatory; every default matches Elasticsearch's OTLP ingest
// in OTel-native mapping mode, which is what the managed OTLP endpoint
// produces.
type ElasticsearchConfig struct {
	Endpoint string

	// APIKey is optional: a local or test cluster with security disabled
	// needs none. When set it is sent as an ApiKey authorization header.
	APIKey string

	// Indices maps each signal to the index (or index pattern) queried for
	// it. Defaults: logs-*, metrics-*, traces-*. Patterns are resolved
	// strictly — a pattern matching no index yields Known false, never a
	// silent empty result (see Observe).
	Indices map[requirements.SignalKind]string

	// ServiceNameField is the document field holding service.name.
	// Default: resource.attributes.service.name (OTel-native mode).
	ServiceNameField string

	// AttributePaths are the document field prefixes under which OTel
	// attributes land; AttributeNames unions field names beneath them and
	// strips the prefix to recover the attribute name. Defaults:
	// attributes., resource.attributes., scope.attributes.
	AttributePaths []string

	// SampleSize caps how many records AttributeNames inspects; a window
	// holding more is reported Truncated, never silently approximated.
	// Default 200.
	SampleSize int

	Timeout time.Duration
}

// NewElasticsearch validates the config and returns the provider. This is
// the only constructor error the implementation ever produces: after boot,
// degradation is data in the readings, never an error (ADR-0008).
func NewElasticsearch(cfg ElasticsearchConfig) (*Elasticsearch, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("elasticsearch endpoint is required")
	}
	if cfg.ServiceNameField == "" {
		cfg.ServiceNameField = "resource.attributes.service.name"
	}
	if len(cfg.AttributePaths) == 0 {
		cfg.AttributePaths = []string{"attributes.", "resource.attributes.", "scope.attributes."}
	}
	if cfg.SampleSize <= 0 {
		cfg.SampleSize = 200
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	idx := map[requirements.SignalKind]string{
		requirements.Logs:    "logs-*",
		requirements.Metrics: "metrics-*",
		requirements.Traces:  "traces-*",
	}
	for k, v := range cfg.Indices {
		idx[k] = v
	}
	return &Elasticsearch{
		endpoint:         strings.TrimRight(cfg.Endpoint, "/"),
		apiKey:           cfg.APIKey,
		http:             &http.Client{Timeout: cfg.Timeout},
		indices:          idx,
		serviceNameField: cfg.ServiceNameField,
		attributePaths:   cfg.AttributePaths,
		sampleSize:       cfg.SampleSize,
		now:              time.Now,
	}, nil
}

func (e *Elasticsearch) Name() string { return "elasticsearch" }

var _ seam.Provider = (*Elasticsearch)(nil)

// Observe issues one _msearch covering all three signals for one Service.
//
// Index resolution is strict (allow_no_indices=false): a concrete index that
// does not exist and a pattern matching nothing both come back as errors, and
// both surface as Known false. Elasticsearch would otherwise resolve an empty
// pattern to an empty-but-successful result, and this provider cannot tell
// "nothing has ever landed" from "pointed at the wrong index" — rendering
// that as an observed absence would be a fabricated value.
//
// Scaling note, stated rather than hidden: this is one round trip per
// Service per evaluation. That is fine into the low thousands at a sensible
// interval; beyond that the right change is a batched implementation
// aggregating by service in one query, which the seam already allows because
// Observed is returned per Service either way.
func (e *Elasticsearch) Observe(ctx context.Context, service seam.Service, window time.Duration, attributes []string) seam.Observed {
	asOf := e.now()
	order := seam.Signals()

	var body bytes.Buffer
	for _, kind := range order {
		header, _ := json.Marshal(map[string]any{
			"index":              e.indices[kind],
			"allow_no_indices":   false,
			"ignore_unavailable": false,
		})
		query, _ := json.Marshal(e.observeBody(service, window, attributes))
		body.Write(header)
		body.WriteByte('\n')
		body.Write(query)
		body.WriteByte('\n')
	}

	raw, err := e.send(ctx, http.MethodPost, "/_msearch", body.Bytes(), "application/x-ndjson")
	if err != nil {
		return seam.Unknown(asOf, window, err.Error())
	}

	var resp struct {
		Responses []struct {
			Status int `json:"status"`
			Hits   struct {
				Total struct {
					Value int64 `json:"value"`
				} `json:"total"`
			} `json:"hits"`
			Aggregations map[string]struct {
				DocCount int64 `json:"doc_count"`
			} `json:"aggregations"`
			Error json.RawMessage `json:"error"`
		} `json:"responses"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return seam.Unknown(asOf, window, fmt.Sprintf("undecodable search response: %v", err))
	}
	if len(resp.Responses) != len(order) {
		return seam.Unknown(asOf, window, fmt.Sprintf("search returned %d responses, expected %d", len(resp.Responses), len(order)))
	}

	obs := seam.Observed{AsOf: asOf, Window: window, Signals: map[requirements.SignalKind]seam.SignalObservation{}}
	for i, kind := range order {
		r := resp.Responses[i]
		if len(r.Error) > 0 {
			obs.Signals[kind] = seam.SignalObservation{Known: false, Cause: e.signalCause(kind, r.Error)}
			continue
		}

		total := r.Hits.Total.Value
		sig := seam.SignalObservation{Known: true, Present: total > 0, Volume: total}
		if total > 0 && len(attributes) > 0 {
			sig.AttributeCoverage = map[string]float64{}
			for _, attr := range attributes {
				if agg, ok := r.Aggregations["attr_"+sanitise(attr)]; ok {
					sig.AttributeCoverage[attr] = float64(agg.DocCount) / float64(total)
				}
			}
		}
		obs.Signals[kind] = sig
	}
	return obs
}

// AttributeNames unions the attribute field names carried by up to
// sampleSize records for one Service, signal and window, and reports the
// truncation whenever the window holds more (ADR-0034: never a silent
// approximation).
func (e *Elasticsearch) AttributeNames(ctx context.Context, service seam.Service, kind requirements.SignalKind, window time.Duration) seam.AttributeNames {
	asOf := e.now()
	reading := seam.AttributeNames{AsOf: asOf, Window: window}
	unknown := func(cause string) seam.AttributeNames {
		reading.Known = false
		reading.Cause = cause
		return reading
	}

	body := e.observeBody(service, window, nil)
	body["size"] = e.sampleSize
	body["_source"] = false
	fields := make([]string, 0, len(e.attributePaths))
	for _, p := range e.attributePaths {
		fields = append(fields, p+"*")
	}
	body["fields"] = fields

	raw, _ := json.Marshal(body)
	path := fmt.Sprintf("/%s/_search?allow_no_indices=false&ignore_unavailable=false", e.indices[kind])
	out, err := e.send(ctx, http.MethodPost, path, raw, "application/json")
	if err != nil {
		// Unlike _msearch, a missing index here surfaces as a whole-request
		// 404 rather than a per-response error; give it the same cause.
		if strings.Contains(err.Error(), "index_not_found") {
			return unknown(e.signalCause(kind, json.RawMessage(err.Error())))
		}
		return unknown(err.Error())
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
	if err := json.Unmarshal(out, &resp); err != nil {
		return unknown(fmt.Sprintf("undecodable search response: %v", err))
	}
	if len(resp.Error) > 0 {
		return unknown(e.signalCause(kind, resp.Error))
	}

	names := map[string]bool{}
	for _, hit := range resp.Hits.Hits {
		for field := range hit.Fields {
			for _, prefix := range e.attributePaths {
				if strings.HasPrefix(field, prefix) {
					// Dynamic string mappings expose a .keyword multi-field
					// beside the field itself; collapse it back onto the
					// attribute name rather than reporting a phantom.
					names[strings.TrimSuffix(strings.TrimPrefix(field, prefix), ".keyword")] = true
					break
				}
			}
		}
	}

	reading.Known = true
	reading.Names = make([]string, 0, len(names))
	for n := range names {
		reading.Names = append(reading.Names, n)
	}
	sort.Strings(reading.Names)
	reading.SampledRecords = int64(len(resp.Hits.Hits))
	reading.TotalRecords = resp.Hits.Total.Value
	reading.Truncated = reading.TotalRecords > reading.SampledRecords
	return reading
}

// observeBody builds the count-and-coverage query: zero hits fetched, exact
// totals, a filter on the window and the Service, and one exists-filter
// aggregation per requested attribute so coverage is measured in the same
// round trip as presence.
func (e *Elasticsearch) observeBody(service seam.Service, window time.Duration, attributes []string) map[string]any {
	body := map[string]any{
		"size":             0,
		"track_total_hits": true,
		"query": map[string]any{
			"bool": map[string]any{
				"filter": []any{
					map[string]any{"range": map[string]any{
						"@timestamp": map[string]any{"gte": "now-" + dateMath(window)},
					}},
					map[string]any{"term": map[string]any{e.serviceNameField: service.Name}},
				},
			},
		},
	}
	if len(attributes) > 0 {
		aggs := map[string]any{}
		for _, attr := range attributes {
			aggs["attr_"+sanitise(attr)] = map[string]any{
				"filter": map[string]any{"exists": map[string]any{"field": attr}},
			}
		}
		body["aggs"] = aggs
	}
	return body
}

// signalCause renders one per-signal search error as a reading cause. A
// missing index is called out by name: it usually means either nothing has
// ever landed for that signal or the provider is pointed at the wrong index,
// and the provider cannot tell which — which is exactly why the reading is
// Known false rather than an observed absence.
func (e *Elasticsearch) signalCause(kind requirements.SignalKind, esError json.RawMessage) string {
	if strings.Contains(string(esError), "index_not_found") {
		return fmt.Sprintf("no index matches %q — nothing has ever landed for %s, or the provider is pointed at the wrong index", e.indices[kind], kind)
	}
	return fmt.Sprintf("%s query failed: %s", kind, truncate(string(esError), 300))
}

func (e *Elasticsearch) send(ctx context.Context, method, path string, body []byte, contentType string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, e.endpoint+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if e.apiKey != "" {
		req.Header.Set("Authorization", "ApiKey "+e.apiKey)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := e.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("backend unreachable: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading backend response: %v", err)
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("backend returned %s: %s", resp.Status, truncate(strings.TrimSpace(string(raw)), 300))
	}
	return raw, nil
}

// dateMath renders a Go duration in Elasticsearch date-math units. Go prints
// sub-units ("24h0m0s") which the range query rejects, so the value is
// reduced to the largest whole unit it divides into.
func dateMath(d time.Duration) string {
	switch {
	case d%(24*time.Hour) == 0:
		return fmt.Sprintf("%dd", int64(d/(24*time.Hour)))
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", int64(d/time.Hour))
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", int64(d/time.Minute))
	default:
		return fmt.Sprintf("%ds", int64(d/time.Second))
	}
}

// sanitise makes an attribute name safe as an aggregation key.
func sanitise(s string) string {
	return strings.NewReplacer(".", "_", "-", "_", "@", "_", " ", "_").Replace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
