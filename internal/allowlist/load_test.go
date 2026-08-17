package allowlist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/catalogue"
	"github.com/telecraft-dev/telecraft/internal/ownership"
)

// The fixture tree: org > platform > payments > checkout, with data as a
// sibling branch under org — enough depth for narrowing across three levels
// and a branch a Grant must not leak into.
const fixtureTeams = `
teams:
  - id: org
    name: Org
    owners: [org-lead]
    teams:
      - id: platform
        name: Platform
        owners: [platform-lead]
        teams:
          - id: payments
            name: Payments
            owners: [payments-lead]
            teams:
              - id: checkout
                name: Checkout
                owners: [checkout-lead]
      - id: data
        name: Data
        owners: [data-lead]
`

func fixtureTree(t *testing.T) ownership.Tree {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, ownership.TeamsFile, fixtureTeams)
	tree, err := ownership.LoadTeams(filepath.Join(dir, ownership.TeamsFile))
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

// fixtureCatalogue builds a small Catalogue through the real artefact
// round-trip, so lookups and alias resolution behave exactly as they do
// against a loaded artefact. kafka exists as a receiver and an exporter —
// the class-collapse case — and span_metrics carries its historical alias.
func fixtureCatalogue(t *testing.T) *catalogue.Catalogue {
	t.Helper()
	comp := func(class catalogue.Class, typ, deprecated string) catalogue.Component {
		return catalogue.Component{
			Class:          class,
			Type:           typ,
			DeprecatedType: deprecated,
			Module:         "example.com/otelcol/" + string(class) + "/" + typ,
			Stability:      map[string]catalogue.Level{"traces": catalogue.Beta},
		}
	}
	cat := &catalogue.Catalogue{
		FormatVersion: catalogue.FormatVersion,
		Source:        catalogue.Source{Repository: "example.com/otelcol", Ref: "v0.158.0"},
		Components: []catalogue.Component{
			comp(catalogue.Receiver, "otlp", ""),
			comp(catalogue.Receiver, "prometheus", ""),
			comp(catalogue.Receiver, "kafka", ""),
			comp(catalogue.Processor, "batch", ""),
			comp(catalogue.Processor, "attributes", ""),
			comp(catalogue.Exporter, "kafka", ""),
			comp(catalogue.Exporter, "otlphttp", ""),
			comp(catalogue.Connector, "span_metrics", "spanmetrics"),
		},
	}
	path, _, err := cat.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := catalogue.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// loadPolicy writes the given policy files (empty string omits the file)
// into a fresh estate directory and loads them against the fixture tree and
// catalogue.
func loadPolicy(t *testing.T, lists, grants string) (*Policy, error) {
	t.Helper()
	dir := t.TempDir()
	if lists != "" {
		writeFile(t, dir, AllowListsFile, lists)
	}
	if grants != "" {
		writeFile(t, dir, GrantsFile, grants)
	}
	p, err := Load(dir, fixtureTree(t), fixtureCatalogue(t))
	if err != nil && p != nil {
		t.Fatal("Load failed but returned a policy — a failed load must fail closed")
	}
	return p, err
}

// mustLoad is loadPolicy for tests that need the policy.
func mustLoad(t *testing.T, lists, grants string) *Policy {
	t.Helper()
	p, err := loadPolicy(t, lists, grants)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// wantLoadError asserts the load fails and the error mentions want.
func wantLoadError(t *testing.T, lists, grants, want string) {
	t.Helper()
	_, err := loadPolicy(t, lists, grants)
	if err == nil {
		t.Fatalf("load succeeded, want an error mentioning %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error does not mention %q:\n%v", want, err)
	}
}

const fixtureLists = `
allow_lists:
  - team: org
    owner: org-lead
    allow:
      - receiver/*
      - processor/batch
      - exporter/otlphttp
  - team: payments
    owner: payments-lead
    allow:
      - receiver/otlp
      - receiver/kafka
      - processor/*
      - exporter/kafka
`

const fixtureGrants = `
grants:
  - id: exporter-kafka-for-payments
    owner: platform-lead
    team: payments
    adds:
      - exporter/kafka
`

func TestPolicyFixtureLoads(t *testing.T) {
	p := mustLoad(t, fixtureLists, fixtureGrants)

	if len(p.Lists) != 2 {
		t.Fatalf("got %d allow-lists, want 2", len(p.Lists))
	}
	org := p.Lists["org"]
	if org.Owner != "org-lead" || len(org.Allow) != 3 {
		t.Errorf("org list loaded as %+v", org)
	}
	if got := org.Allow[0].String(); got != "receiver/*" {
		t.Errorf("first org entry round-trips as %q", got)
	}

	g, ok := p.Grants["exporter-kafka-for-payments"]
	if !ok {
		t.Fatal("grant missing")
	}
	if g.Team != "payments" || g.Owner != "platform-lead" || len(g.Adds) != 1 {
		t.Errorf("grant loaded as %+v", g)
	}
	if p.Catalogue() != "v0.158.0" {
		t.Errorf("policy bound to catalogue %q", p.Catalogue())
	}
}

func TestAbsentPolicyFilesLoadAsDefaultPosture(t *testing.T) {
	p := mustLoad(t, "", "")
	if len(p.Lists) != 0 || len(p.Grants) != 0 {
		t.Fatalf("empty estate loaded lists=%d grants=%d", len(p.Lists), len(p.Grants))
	}
}

// AC: allow-list entries validate against a Catalogue version; unknown
// component types fail load.
func TestEntrySelectingNothingFailsLoad(t *testing.T) {
	lists := `
allow_lists:
  - team: org
    owner: org-lead
    allow: [receiver/nosuch]
`
	wantLoadError(t, lists, "", `entry "receiver/nosuch" selects nothing in catalogue v0.158.0`)

	grants := `
grants:
  - id: g
    owner: org-lead
    team: platform
    adds: [exporter/zzz*]
`
	wantLoadError(t, "", grants, `entry "exporter/zzz*" selects nothing`)
}

func TestMalformedEntriesFailLoad(t *testing.T) {
	cases := map[string]string{
		"otlp":            "is not class/type-pattern",
		"widget/otlp":     "is not a pipeline class",
		"receiver/ot[lp]": "type pattern contains",
		"receiver/a/b":    "type pattern contains",
	}
	for entry, want := range cases {
		lists := "allow_lists:\n  - team: org\n    owner: org-lead\n    allow: ['" + entry + "']\n"
		wantLoadError(t, lists, "", want)
	}
}

func TestUnknownTeamAndOwnerFailLoad(t *testing.T) {
	lists := `
allow_lists:
  - team: nosuch
    owner: ghost
    allow: [receiver/otlp]
`
	wantLoadError(t, lists, "", `names team "nosuch"`)
	wantLoadError(t, lists, "", `names owner "ghost"`)
}

func TestOwnerlessAllowListFailsLoad(t *testing.T) {
	lists := `
allow_lists:
  - team: org
    allow: [receiver/otlp]
`
	wantLoadError(t, lists, "", "has no owner")
}

func TestDuplicateListForOneTeamFailsLoad(t *testing.T) {
	lists := `
allow_lists:
  - team: org
    owner: org-lead
    allow: [receiver/otlp]
  - team: org
    owner: org-lead
    allow: [receiver/prometheus]
`
	wantLoadError(t, lists, "", "declares two allow-lists")
}

func TestEmptyAllowListFailsLoad(t *testing.T) {
	lists := `
allow_lists:
  - team: org
    owner: org-lead
    allow: []
`
	wantLoadError(t, lists, "", "declares no entries")
}

func TestDuplicateEntryFailsLoad(t *testing.T) {
	lists := `
allow_lists:
  - team: org
    owner: org-lead
    allow: [receiver/otlp, receiver/otlp]
`
	wantLoadError(t, lists, "", "appears twice")
}

// A Grant is parent-authored (ADR-0021 §3): the owner's team must be a
// proper ancestor of the target. Self-granting and sibling-granting are the
// two ways to fake a widening without the ancestor conversation.
func TestGrantAuthorityMustBeAProperAncestor(t *testing.T) {
	self := `
grants:
  - id: self-widening
    owner: payments-lead
    team: payments
    adds: [exporter/kafka]
`
	wantLoadError(t, "", self, "is not an ancestor of target team")

	sibling := `
grants:
  - id: sideways
    owner: data-lead
    team: checkout
    adds: [exporter/kafka]
`
	wantLoadError(t, "", sibling, "is not an ancestor of target team")

	descendant := `
grants:
  - id: upward
    owner: checkout-lead
    team: platform
    adds: [exporter/kafka]
`
	wantLoadError(t, "", descendant, "is not an ancestor of target team")

	ancestor := `
grants:
  - id: proper
    owner: org-lead
    team: checkout
    adds: [exporter/kafka]
`
	if _, err := loadPolicy(t, "", ancestor); err != nil {
		t.Fatalf("ancestor-authored grant rejected: %v", err)
	}
}

func TestGrantWithoutIDFailsLoad(t *testing.T) {
	grants := `
grants:
  - owner: org-lead
    team: payments
    adds: [exporter/kafka]
`
	wantLoadError(t, "", grants, "has no id")
}

func TestDuplicateGrantIDFailsLoad(t *testing.T) {
	grants := `
grants:
  - id: g
    owner: org-lead
    team: payments
    adds: [exporter/kafka]
  - id: g
    owner: org-lead
    team: checkout
    adds: [receiver/otlp]
`
	wantLoadError(t, "", grants, `grant "g" defined twice`)
}

func TestGrantAddingNothingFailsLoad(t *testing.T) {
	grants := `
grants:
  - id: g
    owner: org-lead
    team: payments
    adds: []
`
	wantLoadError(t, "", grants, "adds no entries")
}

func TestUnknownFieldFailsClosedNamingTheFile(t *testing.T) {
	lists := `
allow_lists:
  - teem: org
    owner: org-lead
    allow: [receiver/otlp]
`
	_, err := loadPolicy(t, lists, "")
	if err == nil {
		t.Fatal("unknown field decoded silently")
	}
	if !strings.Contains(err.Error(), "teem") || !strings.Contains(err.Error(), AllowListsFile) {
		t.Fatalf("error names neither the field nor the file:\n%v", err)
	}
}

func TestEmptyPolicyFileIsRejected(t *testing.T) {
	wantLoadError(t, "# nothing here\n", "", "empty file")
}

func TestFileWithNoListsIsRejected(t *testing.T) {
	wantLoadError(t, "allow_lists: []\n", "", "holds no allow_lists")
	wantLoadError(t, "", "grants: []\n", "holds no grants")
}

func TestMultipleYAMLDocumentsAreRejected(t *testing.T) {
	lists := "allow_lists:\n  - team: org\n    owner: org-lead\n    allow: [receiver/otlp]\n---\nallow_lists: []\n"
	wantLoadError(t, lists, "", "more than one YAML document")
}

func TestMissingEstateDirectoryIsAnError(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope"), fixtureTree(t), fixtureCatalogue(t))
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("got %v", err)
	}
}

func TestNilCatalogueIsAnError(t *testing.T) {
	_, err := Load(t.TempDir(), fixtureTree(t), nil)
	if err == nil || !strings.Contains(err.Error(), "no catalogue") {
		t.Fatalf("got %v", err)
	}
}

// An entry written against the historical alias keeps selecting the
// component it always did — aliases resolve on every lookup (ADR-0020 §3).
func TestAliasEntrySelectsTheCanonicalComponent(t *testing.T) {
	lists := `
allow_lists:
  - team: org
    owner: org-lead
    allow: [connector/spanmetrics]
`
	p := mustLoad(t, lists, "")
	pal, err := p.EffectivePalette("org")
	if err != nil {
		t.Fatal(err)
	}
	if len(pal.Entries) != 1 || pal.Entries[0].Component.Type != "span_metrics" {
		t.Fatalf("alias entry produced palette %+v", pal.Entries)
	}
}
