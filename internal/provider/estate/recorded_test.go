package estate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	seam "github.com/telecraft-dev/telecraft/internal/estate"
	"github.com/telecraft-dev/telecraft/internal/estate/estatetest"
)

// writeRecording drops one recorded reading into a temp dir and returns
// its path.
func writeRecording(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "collectors.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// recordedGateway is two gateway collectors, one of which the source could
// not read — the ordinary shape a recording takes.
const recordedGateway = `
as_of: 2026-08-18T12:00:00Z
refresh_cadence: 30s
collectors:
  - identity:
      telecraft.tier: gateway
      service.instance.id: gw-1
    effective:
      pipelines:
        - name: traces
          receivers: [otlp]
          processors: [batch]
          exporters: [otlphttp]
        - name: logs
          receivers: [filelog, otlp]
          exporters: [otlphttp]
  - identity:
      telecraft.tier: gateway
      service.instance.id: gw-2
    effective:
      known: false
      cause: the collector has never reported an effective config
`

func TestRecordedReadingConformsToTheSeam(t *testing.T) {
	p, err := NewRecorded(RecordedConfig{Path: writeRecording(t, recordedGateway)})
	if err != nil {
		t.Fatal(err)
	}
	estatetest.Run(t, estatetest.Kit{
		Provider: p,
		Seeded: []estatetest.Seed{
			{
				Identity: map[string]string{"telecraft.tier": "gateway", "service.instance.id": "gw-1"},
				Pipelines: []seam.Pipeline{
					{Name: "traces", Receivers: []string{"otlp"}, Processors: []string{"batch"}, Exporters: []string{"otlphttp"}},
					{Name: "logs", Receivers: []string{"filelog", "otlp"}, Exporters: []string{"otlphttp"}},
				},
			},
		},
		Absent: map[string]string{"telecraft.tier": "nowhere"},
	})
}

func TestRecordedReadingKeepsComponentOrderVerbatim(t *testing.T) {
	p, err := NewRecorded(RecordedConfig{Path: writeRecording(t, recordedGateway)})
	if err != nil {
		t.Fatal(err)
	}
	c := p.Estate(context.Background()).Lookup(map[string]string{"service.instance.id": "gw-1"})
	if !c.Effective.Known || len(c.Effective.Pipelines) != 2 {
		t.Fatalf("gw-1 effective = %+v, want a known reading of two pipelines", c.Effective)
	}
	if got := c.Effective.Pipelines[1].Receivers; len(got) != 2 || got[0] != "filelog" {
		t.Errorf("receivers = %v, want filelog before otlp — component order is part of the config (ADR-0004)", got)
	}
}

func TestRecordedUnreadableCollectorStaysUnknownWithItsCause(t *testing.T) {
	p, err := NewRecorded(RecordedConfig{Path: writeRecording(t, recordedGateway)})
	if err != nil {
		t.Fatal(err)
	}
	c := p.Estate(context.Background()).Lookup(map[string]string{"service.instance.id": "gw-2"})
	if c.Effective.Known {
		t.Fatalf("gw-2 effective = %+v, want an unknown reading — a collector the source could not read is not one reporting nothing", c.Effective)
	}
	if !strings.Contains(c.Effective.Cause, "never reported") {
		t.Errorf("cause = %q, want the recorded cause carried through", c.Effective.Cause)
	}
	if c.Effective.AsOf.IsZero() {
		t.Error("an unknown reading still carries as_of — we could not see, as of when (ADR-0036 §2)")
	}
}

func TestRecordedReadingIsStampedWhenItWasTakenNotWhenItWasRead(t *testing.T) {
	p, err := NewRecorded(RecordedConfig{Path: writeRecording(t, recordedGateway)})
	if err != nil {
		t.Fatal(err)
	}
	est := p.Estate(context.Background())
	if got := est.AsOf.Format("2006-01-02T15:04:05Z"); got != "2026-08-18T12:00:00Z" {
		t.Errorf("as_of = %s, want the instant recorded in the file — a recording does not get fresher by being opened", got)
	}
}

func TestRecordedDeclaresHealthAndDeliveryIncapable(t *testing.T) {
	p, err := NewRecorded(RecordedConfig{Path: writeRecording(t, recordedGateway)})
	if err != nil {
		t.Fatal(err)
	}
	decl := p.Declaration()
	if !decl.Capable(seam.EffectiveKind) {
		t.Error("the recording carries the Effective reading and must declare it capable")
	}
	for _, kind := range []seam.ReadingKind{seam.HealthKind, seam.DeliveryStatusKind} {
		if _, declared := decl.Readings[kind]; !declared {
			t.Errorf("reading %q is unmentioned — incapable is a declaration, never an omission (ADR-0036 §1)", kind)
		}
		if decl.Capable(kind) {
			t.Errorf("reading %q is declared capable but the format cannot carry it", kind)
		}
	}
}

func TestRecordedLoadFailsClosed(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{
			name: "unknown field",
			body: "as_of: 2026-08-18T12:00:00Z\nrefresh_cadence: 30s\ncollectorss: []\n",
			want: "collectorss",
		},
		{
			name: "no as_of",
			body: "refresh_cadence: 30s\ncollectors: []\n",
			want: "no as_of",
		},
		{
			name: "no refresh cadence",
			body: "as_of: 2026-08-18T12:00:00Z\ncollectors: []\n",
			want: "no refresh_cadence",
		},
		{
			name: "cadence not a duration",
			body: "as_of: 2026-08-18T12:00:00Z\nrefresh_cadence: soon\ncollectors: []\n",
			want: "not a duration",
		},
		{
			name: "collector with no identity",
			body: "as_of: 2026-08-18T12:00:00Z\nrefresh_cadence: 30s\ncollectors:\n  - effective: {pipelines: []}\n",
			want: "no identity attributes",
		},
		{
			name: "unreadable with no cause",
			body: "as_of: 2026-08-18T12:00:00Z\nrefresh_cadence: 30s\ncollectors:\n  - identity: {id: a}\n    effective: {known: false}\n",
			want: "no cause",
		},
		{
			name: "unreadable carrying pipelines",
			body: "as_of: 2026-08-18T12:00:00Z\nrefresh_cadence: 30s\ncollectors:\n  - identity: {id: a}\n    effective: {known: false, cause: quiet, pipelines: [{name: logs}]}\n",
			want: "payload means nothing",
		},
		{
			name: "same collector twice",
			body: "as_of: 2026-08-18T12:00:00Z\nrefresh_cadence: 30s\ncollectors:\n  - identity: {id: a}\n  - identity: {id: a}\n",
			want: "recorded twice",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewRecorded(RecordedConfig{Path: writeRecording(t, tc.body)})
			if err == nil {
				t.Fatalf("the reading loaded — a recording nobody can trust the shape of is worse than none")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to name %q", err, tc.want)
			}
		})
	}
}

func TestRecordedMissingFileIsALoadError(t *testing.T) {
	if _, err := NewRecorded(RecordedConfig{Path: filepath.Join(t.TempDir(), "absent.yaml")}); err == nil {
		t.Error("an absent recording loaded — the run must fail rather than judge an estate it never read")
	}
}
