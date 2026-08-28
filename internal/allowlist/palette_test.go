package allowlist

import (
	"testing"

	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/pkg/ownership"
)

// keys reduces a palette to its component keys, in palette order.
func keys(p Palette) []string {
	var out []string
	for _, e := range p.Entries {
		out = append(out, string(e.Component.Class)+"/"+e.Component.Type)
	}
	return out
}

func palette(t *testing.T, p *Policy, team ownership.TeamID) Palette {
	t.Helper()
	pal, err := p.EffectivePalette(team)
	if err != nil {
		t.Fatal(err)
	}
	return pal
}

func wantKeys(t *testing.T, got Palette, want ...string) {
	t.Helper()
	g := keys(got)
	if len(g) != len(want) {
		t.Fatalf("palette for %s is %v, want %v", got.Team, g, want)
	}
	for i := range want {
		if g[i] != want[i] {
			t.Fatalf("palette for %s is %v, want %v", got.Team, g, want)
		}
	}
}

// AC: a child team's effective palette derives per ADR-0021's inheritance
// rules. Narrowing-only: each declared list intersects the parent's
// effective list, so a child listing what its parent excludes gains nothing,
// and a team with no list of its own inherits unchanged.
func TestEffectivePaletteNarrowsDownTheTree(t *testing.T) {
	p := mustLoad(t, fixtureLists, "")

	// org's own list, intersected with the default-allow above it.
	// Palette order is the Catalogue's (class, type) order.
	org := palette(t, p, "org")
	wantKeys(t, org,
		"exporter/otlphttp", "processor/batch",
		"receiver/kafka", "receiver/otlp", "receiver/prometheus")
	for _, e := range org.Entries {
		if e.Origin != OriginAllowList {
			t.Errorf("%s/%s has origin %q, want %q", e.Component.Class, e.Component.Type, e.Origin, OriginAllowList)
		}
	}

	// platform declares nothing: it inherits org's effective list unchanged.
	wantKeys(t, palette(t, p, "platform"),
		"exporter/otlphttp", "processor/batch",
		"receiver/kafka", "receiver/otlp", "receiver/prometheus")

	// payments intersects: prometheus subtracted (not in its list),
	// exporter/otlphttp subtracted, and its exporter/kafka and processor/*
	// entries gain nothing beyond what org already allows.
	wantKeys(t, palette(t, p, "payments"),
		"processor/batch", "receiver/kafka", "receiver/otlp")

	// checkout declares nothing: it inherits payments' effective list.
	wantKeys(t, palette(t, p, "checkout"),
		"processor/batch", "receiver/kafka", "receiver/otlp")

	// data sits on the org branch only, so payments' narrowing never reaches it.
	wantKeys(t, palette(t, p, "data"),
		"exporter/otlphttp", "processor/batch",
		"receiver/kafka", "receiver/otlp", "receiver/prometheus")
}

// AC: a Grant widens a specific team's palette and appears with provenance
// in effective-palette output. It applies to the target's subtree and to
// nobody else: not the granting team, not the target's ancestors.
func TestGrantWidensTheTargetSubtreeWithProvenance(t *testing.T) {
	p := mustLoad(t, fixtureLists, fixtureGrants)

	pay := palette(t, p, "payments")
	wantKeys(t, pay,
		"exporter/kafka", "processor/batch", "receiver/kafka", "receiver/otlp")
	granted := pay.Entries[0]
	if granted.Origin != OriginGrant {
		t.Fatalf("exporter/kafka has origin %q, want %q", granted.Origin, OriginGrant)
	}
	if granted.Grant != "exporter-kafka-for-payments" || granted.GrantedBy != "platform" || granted.GrantedTo != "payments" {
		t.Errorf("grant provenance = %q by %q to %q", granted.Grant, granted.GrantedBy, granted.GrantedTo)
	}

	// The subtree inherits the widened list.
	wantKeys(t, palette(t, p, "checkout"),
		"exporter/kafka", "processor/batch", "receiver/kafka", "receiver/otlp")

	// The granting team and the target's ancestors are untouched.
	wantKeys(t, palette(t, p, "platform"),
		"exporter/otlphttp", "processor/batch",
		"receiver/kafka", "receiver/otlp", "receiver/prometheus")
	wantKeys(t, palette(t, p, "org"),
		"exporter/otlphttp", "processor/batch",
		"receiver/kafka", "receiver/otlp", "receiver/prometheus")
}

// A Grant is narrowed back out below like anything else (ADR-0021 §3): a
// descendant's declared list that omits the granted component removes it.
func TestGrantIsNarrowedBackOutBelow(t *testing.T) {
	lists := fixtureLists + `
  - team: checkout
    owner: checkout-lead
    allow:
      - receiver/otlp
`
	p := mustLoad(t, lists, fixtureGrants)

	// payments still holds the granted exporter…
	wantKeys(t, palette(t, p, "payments"),
		"exporter/kafka", "processor/batch", "receiver/kafka", "receiver/otlp")
	// …and checkout's own list narrowed it back out.
	wantKeys(t, palette(t, p, "checkout"), "receiver/otlp")
}

// The union applies after the intersection at each step: a Grant widens its
// target's effective list even past the target's own declared list. The
// escape hatch would be useless if the list it escapes could veto it.
func TestGrantOverridesTheTargetsOwnList(t *testing.T) {
	lists := `
allow_lists:
  - team: payments
    owner: payments-lead
    allow:
      - receiver/otlp
`
	p := mustLoad(t, lists, fixtureGrants)
	pal := palette(t, p, "payments")
	wantKeys(t, pal, "exporter/kafka", "receiver/otlp")
	if pal.Entries[0].Origin != OriginGrant || pal.Entries[1].Origin != OriginAllowList {
		t.Errorf("origins = %q, %q", pal.Entries[0].Origin, pal.Entries[1].Origin)
	}
}

// Default posture (ADR-0021 §4): absent any authored list on the chain, the
// effective list is the whole active Catalogue.
func TestDefaultPostureAllowsTheWholeCatalogue(t *testing.T) {
	p := mustLoad(t, "", "")
	pal := palette(t, p, "checkout")
	if len(pal.Entries) != fixtureCatalogue(t).Len() {
		t.Fatalf("default palette holds %d of %d components", len(pal.Entries), fixtureCatalogue(t).Len())
	}
	for _, e := range pal.Entries {
		if e.Origin != OriginDefault {
			t.Errorf("%s has origin %q, want %q", e.Component.Type, e.Origin, OriginDefault)
		}
	}
	if pal.Catalogue != "v0.158.0" {
		t.Errorf("palette names catalogue %q", pal.Catalogue)
	}
}

// Entries are shapes: class-exact, type-patterned. exporter/kafka* selects
// the kafka exporter and never the kafka receiver.
func TestPatternEntriesAreClassScoped(t *testing.T) {
	lists := `
allow_lists:
  - team: org
    owner: org-lead
    allow: ['exporter/kafka*']
`
	p := mustLoad(t, lists, "")
	wantKeys(t, palette(t, p, "org"), "exporter/kafka")
}

func TestEffectivePaletteForUnknownTeamIsAnError(t *testing.T) {
	p := mustLoad(t, "", "")
	if _, err := p.EffectivePalette("nosuch"); err == nil {
		t.Fatal("unknown team produced a palette")
	}
}

// Allows is the render gate's question (ADR-0021, ADR-0022): membership in
// the effective palette, with deprecated_type aliases resolved like every
// Catalogue lookup.
func TestAllows(t *testing.T) {
	p := mustLoad(t, fixtureLists, fixtureGrants)

	cases := []struct {
		team  ownership.TeamID
		class catalogue.Class
		typ   string
		want  bool
	}{
		{"payments", catalogue.Receiver, "otlp", true},
		{"payments", catalogue.Exporter, "kafka", true},  // via the grant
		{"platform", catalogue.Exporter, "kafka", false}, // grant targets payments
		{"payments", catalogue.Exporter, "otlphttp", false},
		{"payments", catalogue.Receiver, "prometheus", false},
		{"org", catalogue.Receiver, "prometheus", true},
		{"payments", catalogue.Receiver, "nosuch", false}, // not in the Catalogue at all
	}
	for _, c := range cases {
		got, err := p.Allows(c.team, c.class, c.typ)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("Allows(%s, %s/%s) = %v, want %v", c.team, c.class, c.typ, got, c.want)
		}
	}

	if _, err := p.Allows("nosuch", catalogue.Receiver, "otlp"); err == nil {
		t.Error("Allows for an unknown team did not error")
	}
}

func TestAllowsResolvesDeprecatedTypeAliases(t *testing.T) {
	lists := `
allow_lists:
  - team: org
    owner: org-lead
    allow: [connector/span_metrics]
`
	p := mustLoad(t, lists, "")
	got, err := p.Allows("org", catalogue.Connector, "spanmetrics")
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Error("the alias did not resolve to the allowed canonical component")
	}
}
