package normalise

import (
	"strings"
	"testing"
)

// judged normalises both sides under one profile and returns the layer-3
// findings of the cross as ADR-0054 §1 defines it: the Intended tree
// against the projection of the report onto the keys it asserts.
func judged(t *testing.T, intendedRaw, reportedRaw []byte, p Profile) []Change {
	t.Helper()
	in, err := Normalised(intendedRaw, p)
	if err != nil {
		t.Fatalf("normalise the Intended artefact: %v", err)
	}
	rep, err := Normalised(reportedRaw, p)
	if err != nil {
		t.Fatalf("normalise the reported config: %v", err)
	}
	return Layer3(in, Asserted(in, rep))
}

func judgedYAML(t *testing.T, intended, reported string) []Change {
	t.Helper()
	return judged(t, []byte(intended), []byte(reported), Exact())
}

// The bug on issue #110: a collector running exactly the artefact it was
// served read as drifted with 77 changes, 71 of them the collector
// expanding its own defaults against a sparse authored artefact. The
// served fixture pair carries every class the issue counted.
func TestACollectorRunningWhatItWasSentReadsClean(t *testing.T) {
	changes := judged(t,
		load(t, "testdata/served/intended.yaml"),
		load(t, "testdata/served/reported.yaml"),
		Supervisor())
	if len(changes) != 0 {
		t.Errorf("a collector running what it was sent reads as drifted, in %d places:\n%v", len(changes), changes)
	}
}

// The other half of the same fixture: nothing about that collector is
// undescribed either, so the compensating structural check stays silent on
// a clean estate rather than replacing one permanently red band with
// another (ADR-0054 §2).
func TestACollectorRunningWhatItWasSentDescribesNothingExtra(t *testing.T) {
	in, err := Normalised(load(t, "testdata/served/intended.yaml"), Supervisor())
	if err != nil {
		t.Fatalf("normalise the Intended artefact: %v", err)
	}
	rep, err := Normalised(load(t, "testdata/served/reported.yaml"), Supervisor())
	if err != nil {
		t.Fatalf("normalise the reported config: %v", err)
	}
	if found := Undescribed(in, rep); len(found) != 0 {
		t.Errorf("a collector running what it was sent carries undescribed structure: %v", found)
	}
}

// A setting the collector defaulted is not something the estate asked for,
// so it is not something the estate can have drifted from (ADR-0054 §1).
func TestASettingTheArtefactNeverMentionsIsNotDrift(t *testing.T) {
	changes := judgedYAML(t,
		"processors:\n  batch/batcher:\n    timeout: 2s\n",
		"processors:\n  batch/batcher:\n    timeout: 2s\n    send_batch_max_size: 0\n    metadata_cardinality_limit: 1000\n")
	if len(changes) != 0 {
		t.Errorf("the collector's own defaults read as drift: %v", changes)
	}
}

// An empty component body asserts that the component is there and nothing
// about how it is tuned, so its defaulted settings are not drift either.
func TestAnEmptyComponentBodyAssertsPresenceOnly(t *testing.T) {
	for name, intended := range map[string]string{
		"spelled null":  "processors:\n  batch:\n",
		"spelled empty": "processors:\n  batch: {}\n",
	} {
		changes := judgedYAML(t, intended, "processors:\n  batch:\n    timeout: 200ms\n    send_batch_size: 8192\n")
		if len(changes) != 0 {
			t.Errorf("%s: an unconfigured component reads as drifted once the collector tunes it: %v", name, changes)
		}
	}
}

// A value the artefact does assert, running differently, is the drift the
// band exists for.
func TestAnAssertedValueRunningDifferentlyIsDrift(t *testing.T) {
	changes := judgedYAML(t,
		"processors:\n  batch/batcher:\n    timeout: 2s\n",
		"processors:\n  batch/batcher:\n    timeout: 30s\n    send_batch_max_size: 0\n")
	if len(changes) != 1 || changes[0].Path != "processors.batch/batcher.timeout" {
		t.Errorf("an edited timeout does not read as drift, or reads as more than itself: %v", changes)
	}
}

// The cost of the trade is bounded by keeping this: a key the artefact
// asserts that the collector is not carrying at all still reads as drift.
// This is the class issue #110 saw as `sending_queue.enabled: removed
// true`, and it is deliberately still reported.
func TestAnAssertedKeyMissingFromTheReportIsDrift(t *testing.T) {
	changes := judgedYAML(t,
		"exporters:\n  otlp_http/out:\n    sending_queue:\n      enabled: true\n      queue_size: 1000\n",
		"exporters:\n  otlp_http/out:\n    sending_queue:\n      queue_size: 1000\n      num_consumers: 10\n")
	if len(changes) != 1 || changes[0].Kind != "removed" {
		t.Errorf("a key the artefact asserts went missing without reading as drift: %v", changes)
	}
}

// A component the artefact describes that the collector is not running is
// the same case one level up, and needs no structural check of its own.
func TestAComponentTheArtefactDescribesGoingMissingIsDrift(t *testing.T) {
	changes := judgedYAML(t,
		"processors:\n  memory_limiter/guard:\n    check_interval: 1s\n  batch/batcher:\n    timeout: 2s\n",
		"processors:\n  batch/batcher:\n    timeout: 2s\n")
	if len(changes) != 1 || changes[0].Path != "processors.memory_limiter/guard" || changes[0].Kind != "removed" {
		t.Errorf("a described component going missing does not read as drift: %v", changes)
	}
}

// Pipeline order is semantic (ADR-0004), so an asserted list is judged
// whole: a pipeline that grew a processor the artefact does not list is
// drift, never a defaulted extra.
func TestAPipelineThatGrewAProcessorIsDrift(t *testing.T) {
	const intended = "service:\n  pipelines:\n    traces:\n      processors:\n        - memory_limiter\n"
	const reported = "service:\n  pipelines:\n    traces:\n      processors:\n        - memory_limiter\n        - attributes/exfiltrate\n"
	changes := judgedYAML(t, intended, reported)
	if len(changes) != 1 || changes[0].Kind != "added" {
		t.Errorf("a processor appended to an asserted pipeline does not read as drift: %v", changes)
	}
}

// The collector expands `${env:…}` at load, so the node's own name arriving
// where the artefact wrote the reference is expansion, not drift
// (ADR-0054 §3).
func TestAnExpandedEnvironmentReferenceIsNotDrift(t *testing.T) {
	changes := judgedYAML(t,
		"service:\n  telemetry:\n    resource:\n      k8s.node.name: ${env:TELECRAFT_NODE_NAME}\n",
		"service:\n  telemetry:\n    resource:\n      k8s.node.name: devenv-gateway-2\n")
	if len(changes) != 0 {
		t.Errorf("environment expansion reads as drift: %v", changes)
	}
}

// An artefact that pins part of a value keeps that part judged: the
// reference stands for what the node supplied and the literal text around
// it still has to match.
func TestTheLiteralPartOfAnEnvironmentReferenceIsStillJudged(t *testing.T) {
	const intended = "exporters:\n  otlp/out:\n    endpoint: https://${env:GATEWAY_HOST}:4317\n"
	if changes := judgedYAML(t, intended,
		"exporters:\n  otlp/out:\n    endpoint: https://gateway.internal:4317\n"); len(changes) != 0 {
		t.Errorf("the expanded host reads as drift: %v", changes)
	}
	if changes := judgedYAML(t, intended,
		"exporters:\n  otlp/out:\n    endpoint: http://somewhere-else:9999\n"); len(changes) != 1 {
		t.Errorf("an endpoint edited around the reference does not read as drift: %v", changes)
	}
}

// A stamp genuinely going missing is the one thing on that line worth an
// alarm (ADR-0013), and reading the re-encoded resource back into a map
// must not swallow it.
func TestAStampMissingFromTheReportedResourceIsDrift(t *testing.T) {
	changes := judged(t,
		[]byte("service:\n  telemetry:\n    resource:\n      telecraft.commit: d0d0d0d0\n"),
		[]byte("service:\n  telemetry:\n    resource:\n      - name: k8s.node.name\n        value: gateway-2\n"),
		Supervisor())
	if len(changes) != 1 || changes[0].Path != "service.telemetry.resource.telecraft.commit" || changes[0].Kind != "removed" {
		t.Errorf("a stamp missing from the reported resource does not read as drift: %v", changes)
	}
}

// The projection is the cross's operation, never a Mutation: the layer-2
// digest of one document is unchanged by it, so nothing about ADR-0046 §1's
// digest identity moves. Two authored configs that differ still differ.
func TestTheProjectionDoesNotChangeAdocumentsOwnDigest(t *testing.T) {
	base := load(t, "testdata/corpus/edge-k8s/base.yaml")
	def := load(t, "testdata/corpus/edge-k8s/ambiguous-explicit-default.yaml")
	if layer2(t, base, Exact()) == layer2(t, def, Exact()) {
		t.Fatal("two authored configs digest equal — the projection has leaked into layer 2 (ADR-0046 §2)")
	}
}

// The refusals stay refusals: a report the parser will not read fails
// closed before anything is projected (ADR-0046).
func TestARefusedReportIsNeverProjected(t *testing.T) {
	if _, err := Normalised([]byte("receivers: {}\nreceivers: {}\n"), Exact()); err == nil ||
		!strings.Contains(err.Error(), "duplicate map key") {
		t.Errorf("a duplicate map key did not fail closed (err=%v)", err)
	}
}
