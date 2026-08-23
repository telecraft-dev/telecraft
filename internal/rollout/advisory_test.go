package rollout

import (
	"testing"

	"github.com/telecraft-dev/telecraft/internal/estate"
)

// fromYAML and toYAML are two rendered artefacts with distinct pipeline
// wiring: the to adds a batch processor.
const fromYAML = `
receivers:
  otlp/in: {}
exporters:
  otlphttp/out: {}
service:
  pipelines:
    traces:
      receivers: [otlp/in]
      exporters: [otlphttp/out]
  telemetry:
    resource:
      telecraft.commit: 8b7df143d91c716ecfa5fc1730022f6b421b05cd
`

const toYAML = `
receivers:
  otlp/in: {}
processors:
  batch/batch: {}
exporters:
  otlphttp/out: {}
service:
  pipelines:
    traces:
      receivers: [otlp/in]
      processors: [batch/batch]
      exporters: [otlphttp/out]
  telemetry:
    resource:
      telecraft.commit: 8b7df143d91c716ecfa5fc1730022f6b421b05cd
`

func artefacts(t *testing.T) (from, to Artefact) {
	t.Helper()
	from, err := ParseArtefact([]byte(fromYAML))
	if err != nil {
		t.Fatal(err)
	}
	to, err = ParseArtefact([]byte(toYAML))
	if err != nil {
		t.Fatal(err)
	}
	return from, to
}

func TestParseArtefactReadsPipelines(t *testing.T) {
	_, to := artefacts(t)
	if len(to.Pipelines) != 1 || to.Pipelines[0].Name != "traces" {
		t.Fatalf("pipelines = %+v, want the one traces pipeline", to.Pipelines)
	}
	p := to.Pipelines[0]
	if len(p.Receivers) != 1 || len(p.Processors) != 1 || len(p.Exporters) != 1 {
		t.Errorf("pipeline sides = %+v, want 1/1/1", p)
	}
	if len(to.Hash) == 0 {
		t.Error("no artefact hash")
	}
}

// The served path answers by acknowledged config hash; the Foreign path by
// reported pipeline wiring; and a collector whose readings cannot tell
// reads unknown, never guessed (ADR-0029 §7, ADR-0008).
func TestRunningArtefact(t *testing.T) {
	from, to := artefacts(t)

	cases := []struct {
		name string
		c    estate.Collector
		want Running
	}{
		{
			name: "served, acknowledged the to hash",
			c: estate.Collector{DeliveryStatus: estate.DeliveryStatus{
				Known: true, State: estate.DeliveryApplied, ConfigHash: to.Hash,
			}},
			want: RunningTo,
		},
		{
			name: "served, still acknowledging the from hash",
			c: estate.Collector{DeliveryStatus: estate.DeliveryStatus{
				Known: true, State: estate.DeliveryApplied, ConfigHash: from.Hash,
			}},
			want: RunningFrom,
		},
		{
			name: "foreign, wiring matches the to artefact",
			c: estate.Collector{Effective: estate.Effective{
				Known: true, Pipelines: to.Pipelines,
			}},
			want: RunningTo,
		},
		{
			name: "foreign, wiring matches the from artefact",
			c: estate.Collector{Effective: estate.Effective{
				Known: true, Pipelines: from.Pipelines,
			}},
			want: RunningFrom,
		},
		{
			name: "foreign, wiring matches neither",
			c: estate.Collector{Effective: estate.Effective{
				Known: true, Pipelines: []estate.Pipeline{{Name: "logs", Receivers: []string{"filelog/x"}, Exporters: []string{"otlphttp/out"}}},
			}},
			want: RunningOther,
		},
		{
			name: "no readings",
			c:    estate.Collector{},
			want: RunningUnknown,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RunningArtefact(tc.c, from, to); got != tc.want {
				t.Errorf("running = %s, want %s", got, tc.want)
			}
		})
	}
}

// Wiring both artefacts share distinguishes nothing: the reading is
// honestly unknown rather than guessed.
func TestSharedWiringReadsUnknown(t *testing.T) {
	from, _ := artefacts(t)
	c := estate.Collector{Effective: estate.Effective{Known: true, Pipelines: from.Pipelines}}
	if got := RunningArtefact(c, from, from); got != RunningUnknown {
		t.Errorf("running = %s, want unknown when both artefacts share the wiring", got)
	}
}
