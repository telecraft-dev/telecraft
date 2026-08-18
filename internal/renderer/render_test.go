package renderer

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestRenderMatchesGoldenArtefacts is the determinism acceptance in its
// literal shape: the golden tree is committed, so any behavioural change
// shows up as a reviewable git diff (ADR-0028 §2). Regenerate with
// TELECRAFT_UPDATE_GOLDEN=1 go test ./internal/renderer/.
func TestRenderMatchesGoldenArtefacts(t *testing.T) {
	res, err := Render(fixtureInputs(t))
	if err != nil {
		t.Fatal(err)
	}

	goldenRoot := filepath.Join("testdata", "golden")
	if os.Getenv("TELECRAFT_UPDATE_GOLDEN") != "" {
		if err := os.RemoveAll(goldenRoot); err != nil {
			t.Fatal(err)
		}
		for rel, content := range res.Artefacts {
			path := filepath.Join(goldenRoot, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, content, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	want := map[string][]byte{}
	err = filepath.Walk(goldenRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, err := filepath.Rel(goldenRoot, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		want[filepath.ToSlash(rel)] = content
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for rel := range want {
		if _, ok := res.Artefacts[rel]; !ok {
			t.Errorf("golden artefact %s was not rendered", rel)
		}
	}
	for rel, got := range res.Artefacts {
		if !bytes.Equal(got, want[rel]) {
			t.Errorf("artefact %s differs from golden — identical inputs must produce byte-identical artefacts", rel)
		}
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	first, err := Render(fixtureInputs(t))
	if err != nil {
		t.Fatal(err)
	}
	second, err := Render(fixtureInputs(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Artefacts) != len(second.Artefacts) {
		t.Fatalf("two renders produced %d and %d artefacts", len(first.Artefacts), len(second.Artefacts))
	}
	for rel, content := range first.Artefacts {
		if !bytes.Equal(content, second.Artefacts[rel]) {
			t.Errorf("artefact %s differs between two renders of identical inputs", rel)
		}
	}
}

// gatewayArtefact renders the fixture and returns the production gateway's
// collector YAML, parsed and raw.
func gatewayArtefact(t *testing.T) (map[string]any, string) {
	t.Helper()
	res, err := Render(fixtureInputs(t))
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := res.Artefacts["rendered/data-flow/gateway.yaml"]
	if !ok {
		t.Fatal("no rendered/data-flow/gateway.yaml artefact")
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("gateway artefact is not valid YAML: %v", err)
	}
	return doc, string(raw)
}

// Hard rule 1 (REQ-034, ADR-0010): additional OpAMP extensions render as
// opamp/<x>. A bare `opamp` block would silently override the Supervisor's
// injected endpoint, and the type/name id scheme makes it unrepresentable.
func TestOpampExtensionRendersNamespacedNeverBare(t *testing.T) {
	doc, _ := gatewayArtefact(t)
	extensions, ok := doc["extensions"].(map[string]any)
	if !ok {
		t.Fatal("gateway artefact has no extensions section")
	}
	if _, bare := extensions["opamp"]; bare {
		t.Error("a bare opamp extension rendered — it silently overrides the Supervisor's injected endpoint (ADR-0010)")
	}
	if _, ok := extensions["opamp/reporting"]; !ok {
		t.Error("the authored opamp-type extension did not render as opamp/reporting")
	}
	service := doc["service"].(map[string]any)
	wired, _ := service["extensions"].([]any)
	var namespaced bool
	for _, id := range wired {
		if id == "opamp" {
			t.Errorf("service.extensions wires a bare opamp id")
		}
		if id == "opamp/reporting" {
			namespaced = true
		}
	}
	if !namespaced {
		t.Errorf("service.extensions = %v — the opamp extension must be wired under its namespaced id", wired)
	}
}

// Hard rule 2 (REQ-034, ADR-0010): the node-unique identifying attribute
// arrives via Downward API env indirection — one DaemonSet manifest, per-node
// identity — and the commit stamp makes the artefact carry its own identity
// (ADR-0013).
func TestIdentityStamps(t *testing.T) {
	doc, _ := gatewayArtefact(t)
	service := doc["service"].(map[string]any)
	telemetry, _ := service["telemetry"].(map[string]any)
	resource, _ := telemetry["resource"].(map[string]any)
	if got := resource[CommitAttribute]; got != fixtureCommit {
		t.Errorf("%s = %v, want the input SHA %s", CommitAttribute, got, fixtureCommit)
	}
	if got := resource["k8s.node.name"]; got != "${env:"+NodeEnvVar+"}" {
		t.Errorf("k8s.node.name = %v, want the %s env indirection", got, NodeEnvVar)
	}
}

// Hard rule 3 (REQ-034, ADR-0007): data crossing an untrusted Hop sheds the
// platform's attribute namespace, and the generated strip runs before
// anything else can observe inbound data. A Tier with only trusted arrivals
// gets no strip.
func TestUntrustedHopStripGeneratedAutomatically(t *testing.T) {
	res, err := Render(fixtureInputs(t))
	if err != nil {
		t.Fatal(err)
	}

	doc, _ := gatewayArtefact(t)
	processors, _ := doc["processors"].(map[string]any)
	if _, ok := processors[StripProcessorID]; !ok {
		t.Fatalf("gateway has an untrusted Hop but no %s processor", StripProcessorID)
	}
	service := doc["service"].(map[string]any)
	pipelines := service["pipelines"].(map[string]any)
	for signal, p := range pipelines {
		chain := p.(map[string]any)["processors"].([]any)
		if len(chain) == 0 || chain[0] != StripProcessorID {
			t.Errorf("%s pipeline processors = %v — the strip runs first, before anything observes inbound data", signal, chain)
		}
	}

	var edge map[string]any
	if err := yaml.Unmarshal(res.Artefacts["rendered/data-flow/edge.yaml"], &edge); err != nil {
		t.Fatal(err)
	}
	edgeProcessors, _ := edge["processors"].(map[string]any)
	if _, ok := edgeProcessors[StripProcessorID]; ok {
		t.Error("edge has only trusted Hops but renders the strip processor anyway")
	}
}

// The one policy hard block (ADR-0022 §3): a Blueprint using a catalogue
// type outside the owning team's effective palette refuses the render — the
// artefact tree cannot be produced, which is what makes the PR unmergeable
// (ADR-0028 §3).
func TestPaletteViolationRefusesRender(t *testing.T) {
	root := scratchEstate(t, `
name: flow
version: 1
owner: pipelines-lead
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
  - name: to-kafka
    class: exporter
    type: kafka
    version: 1
pipelines:
  traces:
    - component: otlp-in
    - component: to-kafka
`, scratchTier, map[string]string{
		"allow-lists.yaml": `
allow_lists:
  - team: org
    owner: org-lead
    allow:
      - receiver/otlp
      - processor/*
      - exporter/otlphttp
`,
	})

	_, err := Render(estateInputs(t, root))
	if err == nil {
		t.Fatal("a palette violation rendered — the allow-list check is the one rule that hard-blocks at render (ADR-0022 §3)")
	}
	for _, want := range []string{"exporter/kafka", "effective palette", "Grant"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal does not mention %q:\n%v", want, err)
		}
	}
}

// A floor breach is a violation-grade finding routed onward, never a block
// (ADR-0022 §4, ADR-0023 §5): the artefact still renders, and the sibling
// staging Tier — same Blueprint, non-production Environment — is judged
// with no floor at all.
func TestFloorBreachIsAFindingNeverABlock(t *testing.T) {
	res, err := Render(fixtureInputs(t))
	if err != nil {
		t.Fatalf("a floor breach must never refuse the render (ADR-0023 §5): %v", err)
	}
	if _, ok := res.Artefacts["rendered/data-flow/gateway.yaml"]; !ok {
		t.Fatal("the breaching Tier's artefact was withheld — a breach is a finding, not a block")
	}

	var gateway, staging []Finding
	for _, f := range res.Findings {
		if f.Kind != KindFloor {
			continue
		}
		switch f.Tier {
		case "data-flow/gateway":
			gateway = append(gateway, f)
		case "data-flow/gateway-staging":
			staging = append(staging, f)
		}
	}
	if len(gateway) != 1 {
		t.Fatalf("production gateway floor findings = %v, want exactly the alpha-for-logs transform breach", gateway)
	}
	f := gateway[0]
	if f.Lane != "logs" {
		t.Errorf("the breach is on the logs lane (alpha for logs only), got %q", f.Lane)
	}
	for _, want := range []string{"infosec/pii-redaction", "alpha", "beta", "C1", "production", "product/checkout"} {
		if !strings.Contains(f.Message, want) {
			t.Errorf("floor finding does not name %q: %s", want, f.Message)
		}
	}
	if len(staging) != 0 {
		t.Errorf("staging floor findings = %v — non-production carries no floor (ADR-0023 §3)", staging)
	}
}

// Per-Environment binding is realised through sibling Tiers (ADR-0025 §3):
// each renders its own artefact at its own stable path.
func TestSiblingTiersRenderDistinctArtefacts(t *testing.T) {
	res, err := Render(fixtureInputs(t))
	if err != nil {
		t.Fatal(err)
	}
	prod, ok := res.Artefacts["rendered/data-flow/gateway.yaml"]
	if !ok {
		t.Fatal("no production gateway artefact")
	}
	staging, ok := res.Artefacts["rendered/data-flow/gateway-staging.yaml"]
	if !ok {
		t.Fatal("no staging gateway artefact")
	}
	if bytes.Equal(prod, staging) {
		t.Error("sibling Tiers rendered byte-identical artefacts — each carries its own Tier identity")
	}
}

// REQ-032: exactly one collector artefact per Tier, plus the supervisor
// config where the Tier is served — and only there.
func TestSupervisorRendersOnlyWhereServed(t *testing.T) {
	res, err := Render(fixtureInputs(t))
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := res.Artefacts["rendered/data-flow/gateway.supervisor.yaml"]
	if !ok {
		t.Fatal("the served gateway Tier renders no supervisor.yaml (REQ-032)")
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	server, _ := doc["server"].(map[string]any)
	if server["endpoint"] != "wss://opamp.telecraft.internal/v1/opamp" {
		t.Errorf("supervisor server.endpoint = %v", server["endpoint"])
	}
	capabilities, _ := doc["capabilities"].(map[string]any)
	if capabilities["accepts_remote_config"] != true {
		t.Error("accepts_remote_config is not enabled — it is off upstream by default (ADR-0010)")
	}
	agent, _ := doc["agent"].(map[string]any)
	if agent["automatic_config_rollback"] != true {
		t.Error("automatic_config_rollback is not enabled — revert-on-failure is off upstream (ADR-0010)")
	}
	storage, _ := doc["storage"].(map[string]any)
	if storage["directory"] != SupervisorStorageDir {
		t.Errorf("supervisor storage.directory = %v, want the durable path (ADR-0010)", storage["directory"])
	}

	for rel := range res.Artefacts {
		if strings.HasSuffix(rel, ".supervisor.yaml") && rel != "rendered/data-flow/gateway.supervisor.yaml" {
			t.Errorf("unexpected supervisor artefact %s — only served Tiers get one", rel)
		}
	}
}

// A rendered-id collision is a mechanical render error (ADR-0024 §5): a
// local Component whose name lands on a shared Component's `team.name` id
// would leave one definition silently shadowing the other.
func TestRenderedIDCollisionRefuses(t *testing.T) {
	// The shared Component pipelines/scrub renders as
	// attributes/pipelines.scrub; a local Component named `pipelines.scrub`
	// of the same type lands on the identical rendered id.
	root := scratchEstate(t, `
name: flow
version: 1
owner: pipelines-lead
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
  - name: pipelines.scrub
    class: processor
    type: attributes
    version: 1
  - name: to-out
    class: exporter
    type: otlphttp
    version: 1
pipelines:
  traces:
    - component: otlp-in
    - component: pipelines.scrub
    - component: pipelines/scrub@1
    - component: to-out
`, scratchTier, map[string]string{
		"teams/pipelines/components/scrub.yaml": `
name: scrub
class: processor
type: attributes
version: 1
owner: pipelines-lead
`,
	})

	_, err := Render(estateInputs(t, root))
	if err == nil || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("a rendered-id collision did not refuse the render (ADR-0024 §5): %v", err)
	}
}

// The strip processor's id is reserved on every Tier: an authored instance
// landing on it would be silently overwritten exactly where the strip
// matters most.
func TestStripProcessorIDIsReserved(t *testing.T) {
	root := scratchEstate(t, `
name: flow
version: 1
owner: pipelines-lead
components:
  - name: otlp-in
    class: receiver
    type: otlp
    version: 1
  - name: telecraft.untrusted-hop
    class: processor
    type: attributes
    version: 1
  - name: to-out
    class: exporter
    type: otlphttp
    version: 1
pipelines:
  traces:
    - component: otlp-in
    - component: telecraft.untrusted-hop
    - component: to-out
`, scratchTier, nil)

	_, err := Render(estateInputs(t, root))
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("an authored instance claimed %s without refusal: %v", StripProcessorID, err)
	}
}

// A Tier binding pinned off the Blueprint's head renders head — the only
// content this tree holds — and surfaces the stale pin as a finding, never
// a silent substitution and never a block (ADR-0026).
func TestBindingOffHeadIsAFinding(t *testing.T) {
	root := scratchEstate(t, `
name: flow
version: 2
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
`, nil)

	res, err := Render(estateInputs(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.Artefacts["rendered/pipelines/gateway.yaml"]; !ok {
		t.Fatal("the off-head binding withheld the artefact")
	}
	var found bool
	for _, f := range res.Findings {
		if f.Kind == KindBinding && f.Tier == "pipelines/gateway" {
			found = true
			if !strings.Contains(f.Message, "version 2") {
				t.Errorf("binding finding does not name head: %s", f.Message)
			}
		}
	}
	if !found {
		t.Error("no binding finding for a Tier pinned off head (ADR-0026)")
	}
}

// A dangling binding refuses: a Tier whose artefact cannot exist would
// leave rendered/ inconsistent with the authored trees (ADR-0028 §2).
func TestDanglingBindingRefuses(t *testing.T) {
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
blueprint: pipelines/retired@1
`, nil)

	_, err := Render(estateInputs(t, root))
	if err == nil || !strings.Contains(err.Error(), "pipelines/retired@1") {
		t.Fatalf("a dangling binding did not refuse the render: %v", err)
	}
}

// A lane that compiles to a pipeline no collector accepts refuses —
// mechanical validity, not policy (ADR-0024 §7).
func TestReceiverlessLaneRefuses(t *testing.T) {
	root := scratchEstate(t, `
name: flow
version: 1
owner: pipelines-lead
components:
  - name: batcher
    class: processor
    type: batch
    version: 1
  - name: to-out
    class: exporter
    type: otlphttp
    version: 1
pipelines:
  traces:
    - component: batcher
    - component: to-out
`, scratchTier, nil)

	_, err := Render(estateInputs(t, root))
	if err == nil || !strings.Contains(err.Error(), "no receiver side") {
		t.Fatalf("a receiverless lane did not refuse the render: %v", err)
	}
}

// The Unmatched artefact (ADR-0030): rendered unconditionally at its
// distinguished path, commit-stamped, labelled governed-by-nobody,
// self-telemetry only — no data pipelines, no receivers, no exporters —
// and non-empty by construction (ADR-0010 rule 6).
func TestUnmatchedArtefactRendersUnconditionally(t *testing.T) {
	res, err := Render(fixtureInputs(t))
	if err != nil {
		t.Fatal(err)
	}
	raw, ok := res.Artefacts[UnmatchedArtefactPath]
	if !ok {
		t.Fatalf("no %s artefact — the renderer emits it unconditionally (ADR-0030)", UnmatchedArtefactPath)
	}
	if len(raw) == 0 {
		t.Fatal("the Unmatched artefact is empty — it exists to be the non-empty thing the server serves (ADR-0010 rule 6)")
	}

	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("the Unmatched artefact is not valid YAML: %v", err)
	}
	for _, section := range []string{"receivers", "processors", "exporters", "connectors"} {
		if _, has := doc[section]; has {
			t.Errorf("the Unmatched artefact has a %s section — self-telemetry only, no data pipelines (ADR-0030)", section)
		}
	}
	service, ok := doc["service"].(map[string]any)
	if !ok {
		t.Fatal("the Unmatched artefact has no service section")
	}
	if _, has := service["pipelines"]; has {
		t.Error("the Unmatched artefact wires pipelines — no data pipelines (ADR-0030)")
	}
	telemetry, _ := service["telemetry"].(map[string]any)
	resource, _ := telemetry["resource"].(map[string]any)
	if resource[CommitAttribute] != fixtureCommit {
		t.Errorf("commit stamp = %v, want %v — the artefact carries its own identity (ADR-0013)", resource[CommitAttribute], fixtureCommit)
	}
	if resource[UnmatchedAttribute] != true {
		t.Errorf("%s = %v — the unmatched collector is labelled governed-by-nobody (ADR-0030)", UnmatchedAttribute, resource[UnmatchedAttribute])
	}
}
