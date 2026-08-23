package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeEstate(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "estate.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFixtureEstateLoads(t *testing.T) {
	estate, err := LoadEstate(filepath.Join("testdata", "estate", "estate.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := len(estate.Rows); got != 3 {
		t.Fatalf("estate has %d rows, want 3, one per (Service, Environment)", got)
	}

	byRow := map[Row]EstateRow{}
	for _, r := range estate.Rows {
		byRow[r.Row] = r
	}
	prod, ok := byRow[Row{Service: "checkout", Environment: "production"}]
	if !ok {
		t.Fatal("no row for checkout in production")
	}
	if !prod.Effective.Known || len(prod.Effective.Pipelines) != 2 {
		t.Fatalf("checkout production effective = %+v, want a known reading of 2 pipelines", prod.Effective)
	}
	if got := prod.Effective.Pipelines[0].Receivers; len(got) != 2 || got[0] != "filelog" {
		t.Errorf("component order not preserved: %v", got)
	}

	empty := byRow[Row{Service: "billing", Environment: "production"}]
	if !empty.Effective.Known || len(empty.Effective.Pipelines) != 0 {
		t.Errorf("an empty config must load as a known reading of nothing: %+v", empty.Effective)
	}

	if got := estate.Environments(); len(got) != 2 || got[0] != "production" || got[1] != "staging" {
		t.Errorf("Environments() = %v, want [production staging]", got)
	}
}

func TestEstateLoadFailsClosed(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{
			name: "unknown field",
			body: "services:\n  - name: checkout\n    environments:\n      - name: production\n        pipeliness: []\n",
			want: "pipeliness",
		},
		{
			name: "duplicate row",
			body: "services:\n  - name: checkout\n    environments:\n      - name: production\n      - name: production\n",
			want: "appears twice in environment",
		},
		{
			name: "nameless service",
			body: "services:\n  - environments:\n      - name: production\n",
			want: "no name",
		},
		{
			name: "service with no environment",
			body: "services:\n  - name: checkout\n    environments: []\n",
			want: "no environment",
		},
		{
			name: "nameless pipeline",
			body: "services:\n  - name: checkout\n    environments:\n      - name: production\n        pipelines:\n          - receivers: [otlp]\n",
			want: "pipeline with no name",
		},
		{
			name: "empty estate",
			body: "services: []\n",
			want: "declares no services",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			estate, err := LoadEstate(writeEstate(t, tc.body))
			if err == nil {
				t.Fatal("load succeeded, want a failure that names the problem")
			}
			if len(estate.Rows) != 0 {
				t.Error("a failed load must fail closed: no rows returned")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestEstateGraceTableLoads(t *testing.T) {
	estate, err := LoadEstate(writeEstate(t, `
grace:
  - class: C1
    window: 24h
  - class: C2
    window: 24h
  - class: C3
    window: 720h
services:
  - name: checkout
    class: C1
    onboarded: 2026-08-10
    environments:
      - name: production
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(estate.Grace) != 3 {
		t.Fatalf("grace table = %+v, want 3 entries", estate.Grace)
	}
	if w, ok := estate.Grace.WindowFor("C3"); !ok || w != 720*time.Hour {
		t.Errorf("WindowFor(C3) = %v, %v", w, ok)
	}
	if _, ok := estate.Grace.WindowFor("C9"); ok {
		t.Error("WindowFor must report a class the table does not define")
	}
	row := estate.Rows[0]
	if row.Class != "C1" || !row.Onboarded.Equal(time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("row = %+v, want class C1 onboarded 2026-08-10", row)
	}
}

// The grace table and the class fields fail closed like the rest of the
// estate file: a table that grew with class, a class the table cannot
// place, or an onboarding date with no class to scale it is a load error.
func TestEstateGraceFailsClosed(t *testing.T) {
	deployed := "    environments:\n      - name: production\n"
	cases := []struct {
		name, body, want string
	}{
		{
			name: "grace grows as class rises",
			body: "grace:\n  - class: C1\n    window: 720h\n  - class: C2\n    window: 24h\nservices:\n  - name: checkout\n" + deployed,
			want: "is shorter than class",
		},
		{
			name: "duplicate class",
			body: "grace:\n  - class: C1\n    window: 24h\n  - class: C1\n    window: 24h\nservices:\n  - name: checkout\n" + deployed,
			want: "twice",
		},
		{
			name: "nameless class",
			body: "grace:\n  - window: 24h\nservices:\n  - name: checkout\n" + deployed,
			want: "no class",
		},
		{
			name: "non-positive window",
			body: "grace:\n  - class: C1\n    window: 0h\nservices:\n  - name: checkout\n" + deployed,
			want: "positive",
		},
		{
			name: "class the table does not define",
			body: "grace:\n  - class: C1\n    window: 24h\nservices:\n  - name: checkout\n    class: C9\n" + deployed,
			want: "does not define",
		},
		{
			name: "class with no grace table at all",
			body: "services:\n  - name: checkout\n    class: C1\n" + deployed,
			want: "does not define",
		},
		{
			name: "onboarded without class",
			body: "services:\n  - name: checkout\n    onboarded: 2026-08-10\n" + deployed,
			want: "onboarded date but no class",
		},
		{
			name: "malformed onboarded date",
			body: "services:\n  - name: checkout\n    class: C1\n    onboarded: last tuesday\n" + deployed,
			want: "2006-01-02",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			estate, err := LoadEstate(writeEstate(t, tc.body))
			if err == nil {
				t.Fatal("load succeeded, want a failure that names the problem")
			}
			if len(estate.Rows) != 0 || len(estate.Grace) != 0 {
				t.Error("a failed load must fail closed: nothing returned")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestEstateLoadMissingFile(t *testing.T) {
	if _, err := LoadEstate(filepath.Join(t.TempDir(), "absent.yaml")); err == nil {
		t.Fatal("loading a missing estate file must fail")
	}
}
