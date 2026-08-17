package telemetry

// The first end-to-end conformance tracer (issue #11, criterion 4): fixture
// Effective + real Observed → verdict. A fixture estate and a requirements
// library load from files, the Observed readings come from a live
// Elasticsearch through the seam — narrowed per Environment — and the
// verdict cross judges each row independently.
//
// Gated on TELECRAFT_TELEMETRY_LIVE_ENDPOINT like the rest of the live
// suite; see elasticsearch_live_test.go.

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/conformance"
	"github.com/telecraft-dev/telecraft/internal/requirements"
	seam "github.com/telecraft-dev/telecraft/internal/telemetry"
)

const liveLibrary = `
- id: logs-delivered
  title: Logs are collected and delivered
  version: 1
  requirement_level: required
  owner: platform-observability
  config:
    has_receiver: [filelog, otlp]
  signal:
    kind: logs
    present: true
    window: 15m
    min_volume: 1
    required_attributes:
      - resource.attributes.service.name
  remediation: wire a filelog or otlp receiver into a logs pipeline
`

// The same Service, the same Effective config, two environments — so any
// difference between the two verdicts can only come from the per-environment
// Observed reading.
const liveEstate = `
services:
  - name: checkout
    environments:
      - name: production
        pipelines:
          - name: logs
            receivers: [filelog]
            processors: [batch]
            exporters: [otlphttp]
      - name: staging
        pipelines:
          - name: logs
            receivers: [filelog]
            processors: [batch]
            exporters: [otlphttp]
`

// seedLiveEnvironments recreates the logs index with production-tagged
// records for the fixture Service — and none for staging. Without the
// environment narrowing, a staging reading would see the production records
// and pass: exactly the blending ADR-0033 forbids.
func seedLiveEnvironments(t *testing.T, endpoint string) {
	t.Helper()
	logs := liveIndices[requirements.Logs]
	liveDo(t, http.MethodDelete, endpoint+"/"+logs+"?ignore_unavailable=true", "")
	liveDo(t, http.MethodPut, endpoint+"/"+logs, `{
		"mappings": {
			"dynamic_templates": [
				{"strings_as_keyword": {"match_mapping_type": "string", "mapping": {"type": "keyword"}}}
			],
			"properties": {"@timestamp": {"type": "date"}}
		}
	}`)

	now := time.Now().UTC().Format(time.RFC3339)
	doc := func(service, env string) string {
		return fmt.Sprintf(`{"@timestamp": %q, "resource": {"attributes": {"service.name": %q, "deployment.environment.name": %q}}}`,
			now, service, env)
	}
	var bulk bytes.Buffer
	for _, d := range []string{
		doc("checkout", "production"),
		doc("checkout", "production"),
		doc("checkout", "production"),
		// Another Service's staging records prove the reading filters on the
		// (Service, Environment) conjunction, not on either term alone.
		doc("somebody-else", "staging"),
	} {
		bulk.WriteString(`{"index":{}}` + "\n" + d + "\n")
	}
	liveDo(t, http.MethodPost, endpoint+"/"+logs+"/_bulk?refresh=true", bulk.String())
}

func TestLiveVerdictForFixtureEstate(t *testing.T) {
	endpoint := envEndpoint(t)
	seedLiveEnvironments(t, endpoint)
	es := liveProvider(t)

	dir := t.TempDir()
	libDir := filepath.Join(dir, "library")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(libDir, "delivery.yaml"), []byte(liveLibrary), 0o644); err != nil {
		t.Fatal(err)
	}
	estatePath := filepath.Join(dir, "estate.yaml")
	if err := os.WriteFile(estatePath, []byte(liveEstate), 0o644); err != nil {
		t.Fatal(err)
	}

	lib, err := requirements.Load(libDir)
	if err != nil {
		t.Fatal(err)
	}
	estate, err := conformance.LoadEstate(estatePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(estate.Rows) != 2 {
		t.Fatalf("fixture estate has %d rows, want 2", len(estate.Rows))
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	verdicts := map[string]conformance.Verdict{}
	for _, row := range estate.Rows {
		ev := conformance.Evidence{
			Effective: row.Effective,
			Observed:  map[time.Duration]seam.Observed{},
		}
		for _, r := range lib.Sorted() {
			if r.Signal == nil || !r.AppliesTo(row.Environment) {
				continue
			}
			w := r.Signal.Window.Std()
			if _, done := ev.Observed[w]; done {
				continue
			}
			ev.Observed[w] = es.Observe(ctx,
				seam.Service{Name: row.Service, Environment: row.Environment},
				w, r.Signal.RequiredAttributes)
		}
		verdicts[row.Environment] = conformance.Evaluate(row.Row, lib, ev, time.Now())
	}

	prod := verdicts["production"]
	if got := prod.Worst(); got != conformance.Compliant {
		t.Errorf("production verdict = %s, want compliant (findings: %+v)", got, prod.Findings)
	}
	if s := prod.Score(); s.Failing != 0 || s.Passing != 1 {
		t.Errorf("production score = %+v, want 1 passing and nothing failing", s)
	}

	// Same Service, same Effective config, no staging records: the verdict
	// is independent — and it is broken_pipeline, the finding no config-only
	// or unscoped reading could produce.
	staging := verdicts["staging"]
	if got := staging.Worst(); got != conformance.BrokenPipeline {
		t.Errorf("staging verdict = %s, want broken_pipeline (findings: %+v)", got, staging.Findings)
	}
	if s := staging.Score(); s.Failing != 1 {
		t.Errorf("staging score = %+v, want exactly one counting failure — the CI gate's non-zero exit", s)
	}
}
