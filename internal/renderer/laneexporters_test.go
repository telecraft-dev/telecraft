package renderer

// The render records the exporter side of every lane it wires, which is
// the Hop-to-exporter join ADR-0040 §1 needs and the authored model does
// not carry: a Hop is authored on the receiving Tier and names no
// component (ADR-0007).
//
// The recorded mapping and the artefact are two statements about the same
// wiring, so the test reads the exporters back out of the emitted YAML and
// requires them to agree. Asserting the map against a hand-written literal
// would let the two drift apart in exactly the way that matters: a
// recorded mapping that no longer describes what the collector runs
// attributes a Hop's rate to an exporter the artefact never wired.

import (
	"bytes"
	"testing"

	"gopkg.in/yaml.v3"
)

// wiredExporters reads `service::pipelines::<signal>::exporters` out of a
// rendered artefact: what the collector will actually run.
func wiredExporters(t *testing.T, artefact []byte) map[string][]string {
	t.Helper()
	var doc struct {
		Service struct {
			Pipelines map[string]struct {
				Exporters []string `yaml:"exporters"`
			} `yaml:"pipelines"`
		} `yaml:"service"`
	}
	if err := yaml.Unmarshal(artefact, &doc); err != nil {
		t.Fatalf("the rendered artefact does not parse: %v", err)
	}
	out := map[string][]string{}
	for lane, p := range doc.Service.Pipelines {
		out[lane] = p.Exporters
	}
	return out
}

func TestRenderRecordsTheExporterSideOfEveryLaneItWires(t *testing.T) {
	res, err := Render(fixtureInputs(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Exporters) == 0 {
		t.Fatal("the render recorded no lane wiring at all")
	}

	for _, tier := range fixtureInputs(t).Topology.SortedTiers() {
		id := tier.ID()
		recorded, ok := res.Exporters[id]
		if !ok {
			t.Errorf("tier %q rendered an artefact but recorded no lane wiring", id)
			continue
		}
		artefact, ok := res.Artefacts[ArtefactPath(tier)]
		if !ok {
			t.Errorf("tier %q recorded lane wiring but rendered no artefact", id)
			continue
		}
		wired := wiredExporters(t, artefact)

		if len(recorded) != len(wired) {
			t.Errorf("tier %q records %d lanes and wires %d", id, len(recorded), len(wired))
		}
		for lane, want := range wired {
			got, present := recorded[lane]
			if !present {
				t.Errorf("tier %q wires a %s lane the record does not mention", id, lane)
				continue
			}
			if len(got) != len(want) {
				t.Errorf("tier %q %s lane: recorded %v, artefact wires %v", id, lane, got, want)
				continue
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("tier %q %s lane: recorded %v, artefact wires %v", id, lane, got, want)
					break
				}
			}
		}
		// A lane the Blueprint does not wire is absent, never an empty
		// list: "this Tier exports no logs" and "this lane fans out to
		// nothing" are different answers (ADR-0008).
		for lane, ids := range recorded {
			if len(ids) == 0 {
				t.Errorf("tier %q records an empty %s lane, which should be absent instead", id, lane)
			}
		}
	}
}

// The `@next` artefact of a dual-bound Tier is not running anywhere yet
// (ADR-0029 §3), so the recorded mapping must describe the base render.
// Recording the candidate's wiring would attribute a Hop's throughput to
// an exporter no collector has ever exported through.
func TestRecordedWiringDescribesTheRunningArtefactNotTheCandidate(t *testing.T) {
	res, err := Render(estateInputs(t, rolloutEstate(t)))
	if err != nil {
		t.Fatal(err)
	}

	base, ok := res.Artefacts["rendered/pipelines/gateway.yaml"]
	if !ok {
		t.Fatal("no base artefact rendered")
	}
	next, ok := res.Artefacts["rendered/pipelines/gateway@next.yaml"]
	if !ok {
		t.Fatal("no @next artefact rendered, so this test would prove nothing")
	}
	if bytes.Equal(base, next) {
		t.Fatal("the two renders are identical, so this test would prove nothing")
	}

	recorded, ok := res.Exporters["pipelines/gateway"]
	if !ok {
		t.Fatal("the dual-bound Tier recorded no lane wiring")
	}
	baseWiring := wiredExporters(t, base)
	for lane, ids := range recorded {
		want, inBase := baseWiring[lane]
		if !inBase {
			t.Errorf("recorded a %s lane the base artefact does not wire", lane)
			continue
		}
		if len(ids) != len(want) {
			t.Errorf("%s lane: recorded %v, base artefact wires %v", lane, ids, want)
			continue
		}
		for i := range want {
			if ids[i] != want[i] {
				t.Errorf("%s lane: recorded %v, base artefact wires %v", lane, ids, want)
				break
			}
		}
	}
}
