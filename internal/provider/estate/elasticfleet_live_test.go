package estate

// Live verification against a real Elastic Fleet API (issue #23): the
// ADR-0008/ADR-0036 discipline that an integration against an API that can
// change unannounced needs a contract test, not just a client. ADR-0046 §3
// adds the redaction pin: the rules in elasticfleetredaction.go are held
// against observed behaviour, so an Elastic Fleet release changing them
// surfaces here as a contract failure, not estate-wide false drift.
//
// The tests are gated on TELECRAFT_ELASTICFLEET_LIVE_ENDPOINT (the Kibana
// base URL) and TELECRAFT_ELASTICFLEET_LIVE_APIKEY, and skip loudly when
// either is unset — the same pattern as the TelemetryProvider live suite —
// so a plain `go test ./...` stays green with no live stack. Reading needs
// only the fleet-agents-read privilege:
//
//	TELECRAFT_ELASTICFLEET_LIVE_ENDPOINT=https://<kibana> \
//	TELECRAFT_ELASTICFLEET_LIVE_APIKEY=<key> \
//	go test ./internal/provider/estate/ -run Live -v
//
// Record-content checks need at least one collector enrolled (an otelcol
// with the opamp extension pointed at the project's OpAMP endpoint with a
// valid enrolment key); with none enrolled the suite verifies the API
// contract it can — routes registered, kuery accepted — and skips the
// rest, saying so.

import (
	"context"
	"os"
	"reflect"
	"testing"

	seam "github.com/telecraft-dev/telecraft/internal/estate"
)

const (
	liveEndpointGate = "TELECRAFT_ELASTICFLEET_LIVE_ENDPOINT"
	liveAPIKeyGate   = "TELECRAFT_ELASTICFLEET_LIVE_APIKEY"
)

func liveElasticFleet(t *testing.T) *ElasticFleet {
	t.Helper()
	endpoint := os.Getenv(liveEndpointGate)
	if endpoint == "" {
		t.Skipf("set %s (and %s) to run against a live Elastic Fleet API", liveEndpointGate, liveAPIKeyGate)
	}
	p, err := NewElasticFleet(ElasticFleetConfig{
		Endpoint: endpoint,
		APIKey:   os.Getenv(liveAPIKeyGate),
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// The read contract: the agent list route is registered, accepts the
// type:OPAMP filter, and the estate reading built from it conforms —
// identity on every collector, timestamps on every reading carried,
// delivery status untouched (Estate itself can never say "the console was
// unreachable", so the raw probe here is what keeps this test from
// passing vacuously against a dead endpoint).
func TestElasticFleetLiveEstate(t *testing.T) {
	p := liveElasticFleet(t)
	ctx := context.Background()

	var probe struct {
		Total int `json:"total"`
	}
	if err := p.getJSON(ctx, "/api/fleet/agents?kuery=type%3AOPAMP&perPage=1&page=1", &probe); err != nil {
		t.Fatalf("the agent list route did not answer: %v — the read contract is broken, not merely empty", err)
	}

	est := p.Estate(ctx)
	if est.AsOf.IsZero() {
		t.Error("the live estate reading carries no as_of")
	}
	for _, c := range est.Collectors {
		name := seam.Fingerprint(c.Identity)
		if len(c.Identity) == 0 {
			t.Error("a live collector was returned with no identity attributes")
		}
		if c.Effective.AsOf.IsZero() || c.Health.AsOf.IsZero() {
			t.Errorf("collector %s carries a reading without as_of", name)
		}
		if !c.Effective.Known && c.Effective.Cause == "" {
			t.Errorf("collector %s: effective is a silent gap", name)
		}
		if !c.Health.Known && c.Health.Cause == "" {
			t.Errorf("collector %s: health is a silent gap", name)
		}
		if !reflect.DeepEqual(c.DeliveryStatus, seam.DeliveryStatus{}) {
			t.Errorf("collector %s carries a delivery-status reading — Elastic Fleet can never supply one", name)
		}
	}
	if len(est.Collectors) == 0 {
		t.Skipf("the live project has no collectors enrolled (total %d for type:OPAMP) — the read contract held, record content could not be exercised; enrol a collector to complete the check", probe.Total)
	}
}

// The redaction pin (ADR-0046 §3), held against the live API: every
// string scalar whose key the pinned rules say is redacted must come back
// as the placeholder, and every placeholder must sit under a key the
// pinned rules predict — either direction failing means an Elastic Fleet
// release changed the list and the pin must be re-observed and re-versioned.
func TestElasticFleetLiveRedactionContract(t *testing.T) {
	p := liveElasticFleet(t)
	ctx := context.Background()

	records, err := p.agents(ctx)
	if err != nil {
		t.Fatalf("the agent list route did not answer: %v", err)
	}

	checked := 0
	for _, r := range records {
		var body struct {
			EffectiveConfig map[string]any `json:"effective_config"`
		}
		if err := p.getJSON(ctx, "/api/fleet/agents/"+r.ID+"/effective_config", &body); err != nil {
			t.Errorf("effective config for %s did not answer: %v", r.ID, err)
			continue
		}
		if body.EffectiveConfig == nil {
			continue
		}
		checked++
		walkScalars(body.EffectiveConfig, func(key, value string) {
			if ElasticFleetRedacts(key) && value != ElasticFleetRedactedValue {
				t.Errorf("collector %s: key %q holds %q in the clear — the pinned rules say Elastic Fleet redacts it, so the upstream list has narrowed; re-observe and re-version the pin (ADR-0046 §3)", r.ID, key, value)
			}
			if value == ElasticFleetRedactedValue && !ElasticFleetRedacts(key) {
				t.Errorf("collector %s: key %q was redacted but the pinned rules do not predict it — the upstream list has widened; re-observe and re-version the pin (ADR-0046 §3)", r.ID, key)
			}
		})
	}
	if checked == 0 {
		t.Skip("no live collector has reported an effective config — the redaction pin could not be exercised; enrol a collector whose config carries a secret-named key to complete the check")
	}
}

// walkScalars visits every string leaf of a decoded JSON tree with the key
// it hangs under, recursing through maps and lists exactly as the upstream
// redaction traversal does. A list element inherits the key of the list.
func walkScalars(node any, visit func(key, value string)) {
	var walk func(key string, v any)
	walk = func(key string, v any) {
		switch v := v.(type) {
		case map[string]any:
			for k, child := range v {
				walk(k, child)
			}
		case []any:
			for _, child := range v {
				walk(key, child)
			}
		case string:
			visit(key, v)
		}
	}
	walk("", node)
}
