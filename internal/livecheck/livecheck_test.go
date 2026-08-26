package livecheck

import (
	"reflect"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/requirements"
)

// The fixtures pin the emitted spellings against the upstream record
// schema (crates/weaver_live_check/docs/finding.md, read 2026-08-25): the
// finding id, level, sample and signal attributes, and the two template
// families, context and carried resource attributes.
func TestNormalise(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		attrs map[string]string
		want  Finding
	}{
		{
			name: "a violation with the full attribute set",
			body: "Attribute 'http.request.method' has type 'int', expected 'string'.",
			attrs: map[string]string{
				"weaver.finding.id":                                             "type_mismatch",
				"weaver.finding.level":                                          "violation",
				"weaver.finding.sample.type":                                    "attribute",
				"weaver.finding.signal.type":                                    "span",
				"weaver.finding.signal.name":                                    "GET /api/users",
				"weaver.finding.context.attribute_key":                          "http.request.method",
				"weaver.finding.context.expected":                               "string",
				"weaver.finding.resource.attribute.service.name":                "checkout",
				"weaver.finding.resource.attribute.deployment.environment.name": "production",
			},
			want: Finding{
				ID:          "type_mismatch",
				Level:       LevelViolation,
				Message:     "Attribute 'http.request.method' has type 'int', expected 'string'.",
				SampleType:  "attribute",
				SignalType:  "span",
				SignalName:  "GET /api/users",
				Service:     "checkout",
				Environment: "production",
				Context: map[string]string{
					"attribute_key": "http.request.method",
					"expected":      "string",
				},
				Resource: map[string]string{
					"service.name":                "checkout",
					"deployment.environment.name": "production",
				},
			},
		},
		{
			name: "an improvement on a metric",
			body: "Attribute 'server.port' is recommended and not present.",
			attrs: map[string]string{
				"weaver.finding.id":          "recommended_attribute_not_present",
				"weaver.finding.level":       "improvement",
				"weaver.finding.signal.type": "metric",
				"weaver.finding.signal.name": "http.server.request.duration",
			},
			want: Finding{
				ID:         "recommended_attribute_not_present",
				Level:      LevelImprovement,
				Message:    "Attribute 'server.port' is recommended and not present.",
				SignalType: "metric",
				SignalName: "http.server.request.duration",
			},
		},
		{
			// A record can say nothing at all: the normaliser fills no
			// silence in, and the evaluator fails an ungraded finding
			// closed rather than this package guessing at a level.
			name:  "an empty record stays empty",
			body:  "",
			attrs: map[string]string{},
			want:  Finding{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Normalise(tc.body, tc.attrs)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Normalise() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// The three severity tokens are the tap's own vocabulary, verbatim, and
// nothing else validates.
func TestLevelValid(t *testing.T) {
	for _, l := range []Level{LevelViolation, LevelImprovement, LevelInformation} {
		if !l.Valid() {
			t.Errorf("%q does not validate", l)
		}
	}
	for _, l := range []Level{"", "error", "warn"} {
		if l.Valid() {
			t.Errorf("%q validates and is not a tap severity", l)
		}
	}
}

// A finding's signal type lands on the platform's signal vocabulary where
// one exists, and nowhere otherwise: resource findings belong to every
// signal, and profiles are outside the vocabulary.
func TestSignalFor(t *testing.T) {
	cases := map[string]struct {
		kind requirements.SignalKind
		ok   bool
	}{
		"span":     {requirements.Traces, true},
		"metric":   {requirements.Metrics, true},
		"log":      {requirements.Logs, true},
		"resource": {"", false},
		"profile":  {"", false},
		"":         {"", false},
	}
	for signalType, want := range cases {
		kind, ok := SignalFor(signalType)
		if kind != want.kind || ok != want.ok {
			t.Errorf("SignalFor(%q) = (%q, %v), want (%q, %v)", signalType, kind, ok, want.kind, want.ok)
		}
	}
}
