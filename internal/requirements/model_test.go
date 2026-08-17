package requirements

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// REQ-023 as a structural guarantee: no field anywhere in the Requirement
// model may carry a backend query. The strict loader already rejects a
// `query:` key at load; this test keeps the model itself from ever growing
// one, whatever it gets called.
func TestNoQueryLanguageFieldIsRepresentable(t *testing.T) {
	forbidden := []string{"query", "expression", "dsl", "filter"}

	var walk func(typ reflect.Type, path string, seen map[reflect.Type]bool)
	walk = func(typ reflect.Type, path string, seen map[reflect.Type]bool) {
		for typ.Kind() == reflect.Ptr || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || seen[typ] {
			return
		}
		seen[typ] = true
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			name := strings.ToLower(f.Name + " " + f.Tag.Get("yaml"))
			for _, bad := range forbidden {
				if strings.Contains(name, bad) {
					t.Errorf("%s.%s: the Requirement model must not carry a %s field (REQ-023)", path, f.Name, bad)
				}
			}
			walk(f.Type, path+"."+f.Name, seen)
		}
	}
	walk(reflect.TypeOf(Requirement{}), "Requirement", map[reflect.Type]bool{})
}

func TestKindIsDerivedFromTheAssertionsPresent(t *testing.T) {
	cfg := &ConfigAssertion{HasReceiver: []string{"otlp"}}
	sig := &SignalAssertion{Kind: Logs}

	cases := []struct {
		req  Requirement
		want Kind
	}{
		{Requirement{Config: cfg}, KindConfig},
		{Requirement{Signal: sig}, KindSignal},
		{Requirement{Config: cfg, Signal: sig}, KindConfigAndSignal},
	}
	for _, tc := range cases {
		if got := tc.req.Kind(); got != tc.want {
			t.Errorf("Kind() = %q, want %q", got, tc.want)
		}
	}
}

func TestAppliesTo(t *testing.T) {
	everywhere := Requirement{}
	if !everywhere.AppliesTo("production") || !everywhere.AppliesTo("anything") {
		t.Error("a requirement with no environments list must apply everywhere")
	}
	scoped := Requirement{Environments: []string{"production", "staging"}}
	if !scoped.AppliesTo("staging") || scoped.AppliesTo("dev") {
		t.Error("a scoped requirement must apply exactly to its listed environments")
	}
}

func TestLongestWindowReturnsTheLongestRequested(t *testing.T) {
	lib := Library{Requirements: map[string]Requirement{
		"a": {Signal: &SignalAssertion{Window: Duration(time.Hour)}},
		"b": {Signal: &SignalAssertion{Window: Duration(72 * time.Hour)}},
		"c": {},
	}}
	if got := lib.LongestWindow(); got != 72*time.Hour {
		t.Fatalf("LongestWindow = %v, want 72h", got)
	}
}

func TestCoverageDefaultsToTotal(t *testing.T) {
	if got := (SignalAssertion{}).Coverage(); got != 1.0 {
		t.Fatalf("Coverage() with no explicit value = %v, want 1.0", got)
	}
	relaxed := 0.95
	if got := (SignalAssertion{AttributeCoverage: &relaxed}).Coverage(); got != 0.95 {
		t.Fatalf("Coverage() = %v, want 0.95", got)
	}
}
