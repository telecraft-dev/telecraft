package blueprint

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseReference(t *testing.T) {
	good := map[string]Reference{
		"otlp-in":                 {Name: "otlp-in"},
		"infosec/pii-redaction":   {Team: "infosec", Name: "pii-redaction"},
		"infosec/pii-redaction@3": {Team: "infosec", Name: "pii-redaction", Pin: 3},
	}
	for in, want := range good {
		got, err := parseReference(in)
		if err != nil {
			t.Errorf("parseReference(%q) failed: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("parseReference(%q) = %+v, want %+v", in, got, want)
		}
		if got.String() != in {
			t.Errorf("Reference(%q).String() = %q, want it rendered as authored", in, got.String())
		}
	}

	bad := []string{
		"",
		"infosec/",
		"/pii-redaction",
		"a/b/c",
		"infosec/pii-redaction@0",
		"infosec/pii-redaction@head",
		"@3",
	}
	for _, in := range bad {
		if _, err := parseReference(in); err == nil {
			t.Errorf("parseReference(%q) did not fail", in)
		}
	}
}

func TestReferenceIdentity(t *testing.T) {
	local := Reference{Name: "guard"}
	if !local.Local() || local.ID() != "guard" {
		t.Errorf("local reference misreports itself: %+v", local)
	}
	shared := Reference{Team: "infosec", Name: "pii-redaction", Pin: 3}
	if shared.Local() || shared.ID() != "infosec/pii-redaction" || shared.String() != "infosec/pii-redaction@3" {
		t.Errorf("shared reference misreports itself: %+v", shared)
	}
}

func TestClaimUnmarshal(t *testing.T) {
	var c Claim
	if err := yaml.Unmarshal([]byte(`req-payment-completeness@3`), &c); err != nil {
		t.Fatal(err)
	}
	if c.Requirement != "req-payment-completeness" || c.Version != 3 {
		t.Errorf("claim parsed as %+v", c)
	}
	if c.String() != "req-payment-completeness@3" {
		t.Errorf("claim renders as %q", c.String())
	}

	for _, in := range []string{"req-unstamped", "req@0", "req@", "@3", "req@two"} {
		var bad Claim
		if err := yaml.Unmarshal([]byte(in), &bad); err == nil {
			t.Errorf("claim %q did not fail to parse", in)
		}
	}
}

func TestComponentID(t *testing.T) {
	shared := Component{Name: "pii-redaction", Team: "infosec"}
	if shared.ID() != "infosec/pii-redaction" {
		t.Errorf("shared Component id = %q", shared.ID())
	}
	local := Component{Name: "guard"}
	if local.ID() != "guard" {
		t.Errorf("local Component id = %q", local.ID())
	}
}

func TestSortedAccessorsAreStable(t *testing.T) {
	est := Estate{
		Components: map[string]Component{
			"b/x": {Name: "x", Team: "b"},
			"a/y": {Name: "y", Team: "a"},
		},
		Blueprints: map[string]Blueprint{
			"b/gw": {Name: "gw", Team: "b"},
			"a/gw": {Name: "gw", Team: "a"},
		},
	}
	comps := est.SortedComponents()
	if comps[0].ID() != "a/y" || comps[1].ID() != "b/x" {
		t.Errorf("components not in id order: %v", comps)
	}
	bps := est.SortedBlueprints()
	if bps[0].ID() != "a/gw" || bps[1].ID() != "b/gw" {
		t.Errorf("blueprints not in id order: %v", bps)
	}
}

func TestClaimErrorMentionsVersionStamp(t *testing.T) {
	var c Claim
	err := yaml.Unmarshal([]byte(`req-unstamped`), &c)
	if err == nil || !strings.Contains(err.Error(), "version-stamped") {
		t.Fatalf("unstamped claim error = %v", err)
	}
}
