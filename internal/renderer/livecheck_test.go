package renderer

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/telecraft-dev/telecraft/internal/livecheck"
)

// estateLiveCheck is the declaration the render tests opt Tiers in
// against: an estate endpoint plus a production override, so the per-Tier
// resolution is exercised.
func estateLiveCheck() *LiveCheck {
	return &LiveCheck{
		Endpoint: "livecheck.observability.internal:4317",
		Environments: map[string]string{
			"production": "livecheck-prod.observability.internal:4317",
		},
	}
}

// liveCheckInputs is the fixture estate with the live-check declaration
// set and the named Tiers opted in.
func liveCheckInputs(t *testing.T, lc *LiveCheck, optIn map[string]*LiveCheckOptIn) Inputs {
	t.Helper()
	in := fixtureInputs(t)
	in.LiveCheck = lc
	for id, opt := range optIn {
		tier, ok := in.Topology.Tiers[id]
		if !ok {
			t.Fatalf("fixture has no tier %q", id)
		}
		tier.LiveCheck = opt
		in.Topology.Tiers[id] = tier
	}
	return in
}

// The file alone changes nothing: a declaration with no Tier opting in
// renders every artefact byte for byte as an estate without the file.
func TestLiveCheckDeclarationAloneChangesNothing(t *testing.T) {
	base, err := Render(fixtureInputs(t))
	if err != nil {
		t.Fatal(err)
	}
	declared, err := Render(liveCheckInputs(t, estateLiveCheck(), nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(base.Artefacts) != len(declared.Artefacts) {
		t.Fatalf("the declaration alone changed the artefact count: %d and %d", len(base.Artefacts), len(declared.Artefacts))
	}
	for rel, content := range base.Artefacts {
		if !bytes.Equal(content, declared.Artefacts[rel]) {
			t.Errorf("artefact %s differs once live-check.yaml exists, but with no Tier opted in nothing may change", rel)
		}
	}
}

// ADR-0034 §5: an opted-in Tier tees one pipeline per wired lane, sharing
// the lane's receivers, sampling at the resolved rate, and exporting only
// to the resolved live-check endpoint under the id the liveness reading
// queries (livecheck.ExporterID), so the renderer and the reading agree
// by construction.
func TestLiveCheckBranchRendersBesideTheLanes(t *testing.T) {
	res, err := Render(liveCheckInputs(t, estateLiveCheck(), map[string]*LiveCheckOptIn{"data-flow/gateway": {}}))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(res.Artefacts["rendered/data-flow/gateway.yaml"], &doc); err != nil {
		t.Fatal(err)
	}
	service := doc["service"].(map[string]any)
	pipelines := service["pipelines"].(map[string]any)

	for _, signal := range []string{"traces", "metrics", "logs"} {
		main, _ := pipelines[signal].(map[string]any)
		teed, ok := pipelines[liveCheckPipelineName(signal)].(map[string]any)
		if !ok {
			t.Errorf("no %s pipeline: every wired lane of an opted-in Tier tees", liveCheckPipelineName(signal))
			continue
		}
		if !reflect.DeepEqual(teed["receivers"], main["receivers"]) {
			t.Errorf("%s receivers = %v, want the lane's own %v: the branch is a tee off the same intake", signal, teed["receivers"], main["receivers"])
		}
		// The gateway has an untrusted Hop, so the strip leads the teed
		// pipeline too: the live-check exporter must not observe the
		// platform namespace on inbound data either (ADR-0007).
		wantProcessors := []any{StripProcessorID, LiveCheckSamplerID}
		if !reflect.DeepEqual(teed["processors"], wantProcessors) {
			t.Errorf("%s processors = %v, want %v", signal, teed["processors"], wantProcessors)
		}
		if want := []any{livecheck.ExporterID}; !reflect.DeepEqual(teed["exporters"], want) {
			t.Errorf("%s exporters = %v, want %v and nothing else", signal, teed["exporters"], want)
		}
	}

	exporters, _ := doc["exporters"].(map[string]any)
	def, ok := exporters[livecheck.ExporterID].(map[string]any)
	if !ok {
		t.Fatalf("no %s exporter definition", livecheck.ExporterID)
	}
	if got := def["endpoint"]; got != "livecheck-prod.observability.internal:4317" {
		t.Errorf("live-check exporter endpoint = %v, want the production override resolved for the Tier's Environment", got)
	}
	processors, _ := doc["processors"].(map[string]any)
	sampler, ok := processors[LiveCheckSamplerID].(map[string]any)
	if !ok {
		t.Fatalf("no %s processor definition", LiveCheckSamplerID)
	}
	if got := sampler["sampling_percentage"]; got != DefaultLiveCheckSamplePercent {
		t.Errorf("sampling_percentage = %v, want the default %v", got, DefaultLiveCheckSamplePercent)
	}

	// The record describes every pipeline the artefact wires (ADR-0040
	// §1), the teed branch included: its out-rate is the liveness
	// reading's evidence that the tap is being fed.
	recorded := res.Exporters["data-flow/gateway"]
	for _, signal := range []string{"traces", "metrics", "logs"} {
		if got := recorded[liveCheckPipelineName(signal)]; !reflect.DeepEqual(got, []string{livecheck.ExporterID}) {
			t.Errorf("recorded %s lane = %v, want %v", liveCheckPipelineName(signal), got, []string{livecheck.ExporterID})
		}
	}

	// Tiers that did not opt in render no branch, whatever the estate
	// declares.
	for _, rel := range []string{"rendered/data-flow/edge.yaml", "rendered/data-flow/gateway-staging.yaml", UnmatchedArtefactPath} {
		if strings.Contains(string(res.Artefacts[rel]), "telecraft.live-check") {
			t.Errorf("%s mentions the live-check branch, but its Tier never opted in", rel)
		}
	}
}

// A Tier with only trusted arrivals tees without the strip: the teed
// pipeline mirrors the intake discipline of the lanes it branches off.
func TestLiveCheckBranchOnTrustedTierCarriesNoStrip(t *testing.T) {
	res, err := Render(liveCheckInputs(t, estateLiveCheck(), map[string]*LiveCheckOptIn{"data-flow/edge": {}}))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(res.Artefacts["rendered/data-flow/edge.yaml"], &doc); err != nil {
		t.Fatal(err)
	}
	pipelines := doc["service"].(map[string]any)["pipelines"].(map[string]any)
	teed, ok := pipelines[liveCheckPipelineName("traces")].(map[string]any)
	if !ok {
		t.Fatal("the opted-in edge Tier tees no traces pipeline")
	}
	if want := []any{LiveCheckSamplerID}; !reflect.DeepEqual(teed["processors"], want) {
		t.Errorf("teed processors = %v, want only the sampler on an all-trusted Tier", teed["processors"])
	}
}

// The branch sits beside the lanes, never inside them: the main pipelines
// are byte-identical with and without the tap, so the tap dying costs
// findings, never data (REQ-002). The assertion is that every line of the
// base artefact survives, in order, in the opted-in artefact: the tap only
// ever adds.
func TestLiveCheckLeavesTheMainPipelinesByteIdentical(t *testing.T) {
	base, err := Render(fixtureInputs(t))
	if err != nil {
		t.Fatal(err)
	}
	teed, err := Render(liveCheckInputs(t, estateLiveCheck(), map[string]*LiveCheckOptIn{"data-flow/gateway": {}}))
	if err != nil {
		t.Fatal(err)
	}

	baseLines := strings.Split(string(base.Artefacts["rendered/data-flow/gateway.yaml"]), "\n")
	teedLines := strings.Split(string(teed.Artefacts["rendered/data-flow/gateway.yaml"]), "\n")
	i := 0
	for _, line := range teedLines {
		if i < len(baseLines) && line == baseLines[i] {
			i++
		}
	}
	if i != len(baseLines) {
		t.Errorf("the opted-in artefact rewrote base content near %q: the branch may only add, never touch the lanes", baseLines[i])
	}

	// The named half of the same claim: each main pipeline parses to the
	// identical wiring.
	wiring := func(artefact []byte) map[string]any {
		var doc map[string]any
		if err := yaml.Unmarshal(artefact, &doc); err != nil {
			t.Fatal(err)
		}
		return doc["service"].(map[string]any)["pipelines"].(map[string]any)
	}
	basePipelines := wiring(base.Artefacts["rendered/data-flow/gateway.yaml"])
	teedPipelines := wiring(teed.Artefacts["rendered/data-flow/gateway.yaml"])
	for _, signal := range []string{"traces", "metrics", "logs"} {
		if !reflect.DeepEqual(basePipelines[signal], teedPipelines[signal]) {
			t.Errorf("the %s pipeline changed when the Tier opted in:\nwithout: %v\nwith:    %v", signal, basePipelines[signal], teedPipelines[signal])
		}
	}
}

// Opting in without an estate declaration refuses the render, naming both
// files: the opt-in and the destination live in two places, and only the
// render sees both.
func TestLiveCheckOptInWithoutDeclarationRefuses(t *testing.T) {
	root := scratchEstate(t, `
name: flow
version: 1
owner: pipelines-lead
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
  - name: to-out
    class: exporter
    type: otlphttp
    version: 1
pipelines:
  traces:
    - component: otlp-in
    - component: to-out
`, `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@1
live_check: {}
`, nil)

	_, err := Render(estateInputs(t, root))
	if err == nil {
		t.Fatal("a Tier opted in with no live-check.yaml and the render did not refuse")
	}
	for _, want := range []string{"teams/pipelines/tiers/gateway.yaml", LiveCheckFile} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not name %q:\n%v", want, err)
		}
	}
}

// The generated ids are reserved on every Tier (ADR-0024 §5), exactly as
// the strip processor's is: an authored instance landing on one would be
// silently overwritten on the Tiers that tee.
func TestLiveCheckGeneratedIDsAreReserved(t *testing.T) {
	cases := map[string]string{
		"sampler": `
name: flow
version: 1
owner: pipelines-lead
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
  - name: telecraft.live-check
    class: processor
    type: probabilistic_sampler
    version: 1
  - name: to-out
    class: exporter
    type: otlphttp
    version: 1
pipelines:
  traces:
    - component: otlp-in
    - component: telecraft.live-check
    - component: to-out
`,
		"exporter": `
name: flow
version: 1
owner: pipelines-lead
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
  - name: telecraft.live-check
    class: exporter
    type: otlp
    version: 1
pipelines:
  traces:
    - component: otlp-in
    - component: telecraft.live-check
`,
	}
	for name, blueprintYAML := range cases {
		t.Run(name, func(t *testing.T) {
			root := scratchEstate(t, blueprintYAML, scratchTier, nil)
			_, err := Render(estateInputs(t, root))
			if err == nil || !strings.Contains(err.Error(), "reserved") {
				t.Fatalf("an authored instance claimed a generated live-check id without refusal: %v", err)
			}
		})
	}
}

// The rate resolves Tier override first, then the estate default, then
// the named constant, and the rendered artefact states the applied rate
// in its head comment and sampler config.
func TestLiveCheckSampleRateResolution(t *testing.T) {
	five, twenty := 5.0, 20.0
	lc := LiveCheck{Endpoint: "x:4317"}
	tier := Tier{LiveCheck: &LiveCheckOptIn{}}
	if got := lc.SampleRate(tier); got != DefaultLiveCheckSamplePercent {
		t.Errorf("rate with nothing set = %v, want the default %v", got, DefaultLiveCheckSamplePercent)
	}
	lc.SamplePercent = &five
	if got := lc.SampleRate(tier); got != 5 {
		t.Errorf("rate with an estate default = %v, want 5", got)
	}
	tier.LiveCheck.SamplePercent = &twenty
	if got := lc.SampleRate(tier); got != 20 {
		t.Errorf("rate with a Tier override = %v, want the override 20", got)
	}
}

func TestLiveCheckTierOverrideRendersItsRate(t *testing.T) {
	rate := 2.5
	res, err := Render(liveCheckInputs(t, estateLiveCheck(), map[string]*LiveCheckOptIn{"data-flow/gateway": {SamplePercent: &rate}}))
	if err != nil {
		t.Fatal(err)
	}
	artefact := string(res.Artefacts["rendered/data-flow/gateway.yaml"])
	if !strings.Contains(artefact, "sampling_percentage: 2.5") {
		t.Error("the Tier's override did not reach the sampler config")
	}
	if !strings.Contains(artefact, "samples 2.5 per cent") {
		t.Error("the artefact does not state the applied rate in its comment")
	}
}

func TestLoadLiveCheck(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, LiveCheckFile, `
live_check:
  endpoint: livecheck.observability.internal:4317
  protocol: grpc
  environments:
    production: livecheck-prod.observability.internal:4317
  sample_percent: 25
`)
	lc, err := LoadLiveCheck(root)
	if err != nil {
		t.Fatal(err)
	}
	if lc == nil {
		t.Fatal("a declared live-check.yaml loaded as absent")
	}
	if lc.Endpoint != "livecheck.observability.internal:4317" {
		t.Errorf("endpoint = %q", lc.Endpoint)
	}
	if got := lc.Resolve("production"); got != "livecheck-prod.observability.internal:4317" {
		t.Errorf("Resolve(production) = %q, want the override", got)
	}
	if got := lc.Resolve("staging"); got != lc.Endpoint {
		t.Errorf("Resolve(staging) = %q, want the estate endpoint", got)
	}
	if got := lc.SampleRate(Tier{}); got != 25 {
		t.Errorf("SampleRate = %v, want the declared 25", got)
	}
}

// The file is optional: absence is nil with no error, the deliberate
// opt-out state, unlike telemetry.yaml.
func TestLoadLiveCheckAbsentIsNil(t *testing.T) {
	lc, err := LoadLiveCheck(t.TempDir())
	if err != nil {
		t.Fatalf("a missing live-check.yaml errored: %v", err)
	}
	if lc != nil {
		t.Fatalf("a missing live-check.yaml loaded as %+v, want nil", lc)
	}
}

func TestLoadLiveCheckFailsClosed(t *testing.T) {
	cases := map[string]string{
		"no endpoint":         "live_check:\n  protocol: grpc\n",
		"unaccepted protocol": "live_check:\n  endpoint: x:4317\n  protocol: http/protobuf\n",
		"empty override":      "live_check:\n  endpoint: x:4317\n  environments:\n    production: \"\"\n",
		"unknown field":       "live_check:\n  endpoint: x:4317\n  headers:\n    a: b\n",
		"misspelled section":  "livecheck:\n  endpoint: x:4317\n",
		// The sample is sent over grpc, which carries no request path, so
		// an endpoint ending in a signal path is authored wrong whichever
		// way it is read (the ADR-0053 §2 refusal reused).
		"endpoint carries a signal path": "live_check:\n  endpoint: https://x:4318/v1/logs\n",
		"override carries a signal path": "live_check:\n  endpoint: x:4317\n  environments:\n    production: https://y:4318/v1/traces\n",
		"sample rate of zero":            "live_check:\n  endpoint: x:4317\n  sample_percent: 0\n",
		"sample rate above 100":          "live_check:\n  endpoint: x:4317\n  sample_percent: 101\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, LiveCheckFile, body)
			if _, err := LoadLiveCheck(root); err == nil {
				t.Fatalf("loaded %q without error, but the declaration fails closed", name)
			}
		})
	}
}

// A Tier's own sample_percent is range-checked at load, like the estate's.
func TestTierLiveCheckSamplePercentFailsClosed(t *testing.T) {
	root := scratchEstate(t, `
name: flow
version: 1
owner: pipelines-lead
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
  - name: to-out
    class: exporter
    type: otlphttp
    version: 1
pipelines:
  traces:
    - component: otlp-in
    - component: to-out
`, `
owner: pipelines-lead
environment: production
blueprint: pipelines/flow@1
live_check:
  sample_percent: 0
`, nil)

	if _, err := LoadTopology(root); err == nil || !strings.Contains(err.Error(), "sample_percent") {
		t.Fatalf("a zero sample_percent loaded: %v", err)
	}
}
