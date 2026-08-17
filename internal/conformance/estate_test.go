package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		t.Fatalf("estate has %d rows, want 3 — one per (Service, Environment)", got)
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
			want: "one row per (Service, Environment)",
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
			want: "vacuously",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			estate, err := LoadEstate(writeEstate(t, tc.body))
			if err == nil {
				t.Fatal("load succeeded, want a failure that names the problem")
			}
			if len(estate.Rows) != 0 {
				t.Error("a failed load must fail closed — no rows returned")
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
