package activation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/ownership"
)

func write(t *testing.T, dir, content string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, File), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

const twoActivations = `
catalogue:
  active: v0.159.0
  activations:
    - version: v0.155.0
      at: 2026-08-01T09:00:00Z
      by: platform-lead
      impact:
        summary: Catalogue v0.155.0 holds nothing this estate uses.
    - version: v0.159.0
      previous: v0.155.0
      at: 2026-08-20T11:30:00Z
      by: platform-lead
      impact:
        summary: "Catalogue v0.155.0 to v0.159.0: 1 component in use is removed."
        lines:
          - "receiver/kafka is removed. 1 Blueprint uses it: platform/gateway (Platform Engineering)."
`

func TestAbsentFileDesignatesNothing(t *testing.T) {
	rec, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("an estate that has activated nothing should load: %v", err)
	}
	if _, ok := rec.Active(Catalogue); ok {
		t.Error("an estate with no record designated a version")
	}
}

func TestActiveVersionIsTheOneTheLastActivationDesignated(t *testing.T) {
	rec, err := Load(write(t, t.TempDir(), twoActivations))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := rec.Active(Catalogue)
	if !ok || got != "v0.159.0" {
		t.Errorf("active catalogue is %q (found %v), want v0.159.0", got, ok)
	}
	if _, ok := rec.Active(SchemaRegistry); ok {
		t.Error("a record naming no Schema Registry designated one")
	}
	latest, ok := rec.Catalogue.Latest()
	if !ok || latest.Previous != "v0.155.0" || latest.By != "platform-lead" {
		t.Errorf("latest activation is %+v", latest)
	}
}

// The file's whole promise: a version is active because somebody activated
// it, on a report. Each way of breaking that promise is a load error.
func TestALoadRefusesADesignationNobodyActivated(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		wantHas string
	}{
		{
			name: "active version with no activation behind it",
			file: `
catalogue:
  active: v0.159.0
  activations: []
`,
			wantHas: "records no activation",
		},
		{
			name: "an activation carrying no impact report",
			file: `
catalogue:
  active: v0.159.0
  activations:
    - version: v0.159.0
      at: 2026-08-20T11:30:00Z
      by: platform-lead
      impact: {}
`,
			wantHas: "carries no impact report",
		},
		{
			name: "an activation naming nobody",
			file: `
catalogue:
  active: v0.159.0
  activations:
    - version: v0.159.0
      at: 2026-08-20T11:30:00Z
      impact:
        summary: something changed
`,
			wantHas: "records no owner",
		},
		{
			name: "an active version the last activation did not designate",
			file: `
catalogue:
  active: v0.160.0
  activations:
    - version: v0.159.0
      at: 2026-08-20T11:30:00Z
      by: platform-lead
      impact:
        summary: something changed
`,
			wantHas: "the last activation designated",
		},
		{
			name: "a history that does not join up",
			file: `
catalogue:
  active: v0.159.0
  activations:
    - version: v0.155.0
      at: 2026-08-01T09:00:00Z
      by: platform-lead
      impact:
        summary: first
    - version: v0.159.0
      previous: v0.100.0
      at: 2026-08-20T11:30:00Z
      by: platform-lead
      impact:
        summary: second
`,
			wantHas: "but v0.155.0 was active",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(write(t, t.TempDir(), tc.file))
			if err == nil {
				t.Fatal("the record loaded")
			}
			if !strings.Contains(err.Error(), tc.wantHas) {
				t.Errorf("error is %q, want it to mention %q", err, tc.wantHas)
			}
		})
	}
}

func TestAnUnknownFieldFailsTheLoad(t *testing.T) {
	_, err := Load(write(t, t.TempDir(), "catalogue:\n  active: v1\n  actiations: []\n"))
	if err == nil {
		t.Fatal("a misspelt field loaded")
	}
}

func TestApplyRecordsTheReportItWasDecidedOn(t *testing.T) {
	rep := Report{
		Kind: Catalogue,
		From: "",
		To:   "v0.155.0",
		Changes: []Change{{
			Kind:    Deprecated,
			Subject: "processor/transform",
			Detail:  "is deprecated for logs in this version",
			Uses:    []Use{{Blueprint: "pipelines/flow", Team: "Pipelines"}},
		}},
	}
	at := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	rec, err := Apply(Record{}, Catalogue, rep, "platform-lead", at)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := rec.Active(Catalogue)
	if !ok || got != "v0.155.0" {
		t.Fatalf("active is %q (found %v)", got, ok)
	}
	entry, _ := rec.Catalogue.Latest()
	if entry.Impact.Summary != rep.Summary() {
		t.Errorf("recorded summary %q, want %q", entry.Impact.Summary, rep.Summary())
	}
	if len(entry.Impact.Lines) != 1 || entry.Impact.Lines[0] != rep.Lines()[0] {
		t.Errorf("recorded lines %q, want %q", entry.Impact.Lines, rep.Lines())
	}
	if entry.At != at {
		t.Errorf("recorded time %v, want %v", entry.At, at)
	}
}

func TestApplyRefusesAReportComputedSomewhereElse(t *testing.T) {
	base, err := Apply(Record{}, Catalogue, Report{Kind: Catalogue, To: "v0.155.0"}, "platform-lead", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		rep     Report
		by      ownership.OwnerID
		wantHas string
	}{
		{
			name:    "a report against a version that is not active",
			rep:     Report{Kind: Catalogue, From: "v0.100.0", To: "v0.159.0"},
			by:      "platform-lead",
			wantHas: "Recompute the report against the active version",
		},
		{
			name:    "a first-activation report when a version is already active",
			rep:     Report{Kind: Catalogue, To: "v0.159.0"},
			by:      "platform-lead",
			wantHas: "is already active",
		},
		{
			name:    "a report for the other substrate",
			rep:     Report{Kind: SchemaRegistry, From: "v0.155.0", To: "v0.159.0"},
			by:      "platform-lead",
			wantHas: "cannot activate",
		},
		{
			name:    "an activation nobody decided",
			rep:     Report{Kind: Catalogue, From: "v0.155.0", To: "v0.159.0"},
			by:      "",
			wantHas: "needs the owner deciding it",
		},
		{
			name:    "activating the version that is already active",
			rep:     Report{Kind: Catalogue, From: "v0.155.0", To: "v0.155.0"},
			by:      "platform-lead",
			wantHas: "already active",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Apply(base, Catalogue, tc.rep, tc.by, time.Now()); err == nil {
				t.Fatal("the activation was recorded")
			} else if !strings.Contains(err.Error(), tc.wantHas) {
				t.Errorf("error is %q, want it to mention %q", err, tc.wantHas)
			}
		})
	}
}

func TestSaveWritesWhatLoadReadsBack(t *testing.T) {
	dir := t.TempDir()
	rec, err := Apply(Record{}, SchemaRegistry, Report{Kind: SchemaRegistry, To: "v1.4.0"}, "platform-lead", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, rec); err != nil {
		t.Fatal(err)
	}
	back, err := Load(dir)
	if err != nil {
		t.Fatalf("a record this package wrote did not load: %v", err)
	}
	if got, _ := back.Active(SchemaRegistry); got != "v1.4.0" {
		t.Errorf("read back %q, want v1.4.0", got)
	}
}

func TestInstalledListsEveryRetainedVersion(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"catalogue-v0.155.0.json", "catalogue-v0.159.0.json", "notes.txt", "schema-registry-v1.4.0.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Installed(dir, "catalogue-")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"v0.155.0", "v0.159.0"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("installed %q, want %q", got, want)
	}
}
