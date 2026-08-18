package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeExemptionsDir drops the given files into a fresh directory.
func writeExemptionsDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadExemptions(t *testing.T) {
	dir := writeExemptionsDir(t, map[string]string{
		"payments.yaml": `
- id: payments-onboarding
  requirement: logs-delivered
  owner: payments-lead
  expires: 2026-09-01
  team: payments
  reason: onboarding until September
- id: checkout-migration
  requirement: logs-delivered
  owner: platform-observability
  expires: 2026-10-01
  service: checkout
`,
	})

	got, err := LoadExemptions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("loaded %d exemptions, want 2", len(got))
	}
	if got[0].ID != "checkout-migration" || got[1].ID != "payments-onboarding" {
		t.Errorf("order %q, %q — exemptions load in stable ID order", got[0].ID, got[1].ID)
	}
	if got[0].Service != "checkout" || got[1].Team != "payments" {
		t.Errorf("subjects not parsed: %+v", got)
	}
	if want := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC); !got[0].Expires.Std().Equal(want) {
		t.Errorf("expiry = %v, want %v", got[0].Expires.Std(), want)
	}
}

// A directory with no exemption files is none, not an error — unlike the
// requirements library, where empty would pass everything vacuously, zero
// exemptions is the strictest state there is.
func TestLoadExemptionsEmptyDir(t *testing.T) {
	got, err := LoadExemptions(t.TempDir())
	if err != nil || len(got) != 0 {
		t.Fatalf("empty dir: got %v, %v — want none and no error", got, err)
	}
	if _, err := LoadExemptions(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Error("a missing directory must fail — it is almost always the wrong path")
	}
}

// Criterion: an Exemption without owner or expiry fails load — along with
// every other half-authored shape a waiver must not take (REQ-014, ADR-0037).
func TestExemptionLoadFailsClosed(t *testing.T) {
	valid := func(mutilate func(map[string]string)) string {
		f := map[string]string{
			"id":          "id: e1",
			"requirement": "requirement: logs-delivered",
			"owner":       "owner: someone",
			"expires":     "expires: 2026-09-01",
			"subject":     "service: checkout",
		}
		mutilate(f)
		var b strings.Builder
		for _, k := range []string{"id", "requirement", "owner", "expires", "subject"} {
			if f[k] != "" {
				b.WriteString(f[k] + "\n")
			}
		}
		return b.String()
	}

	cases := []struct {
		name, body, want string
	}{
		{
			name: "no owner",
			body: valid(func(f map[string]string) { f["owner"] = "" }),
			want: "has no owner",
		},
		{
			name: "no expiry",
			body: valid(func(f map[string]string) { f["expires"] = "" }),
			want: "has no expiry",
		},
		{
			name: "no id",
			body: valid(func(f map[string]string) { f["id"] = "" }),
			want: "no id",
		},
		{
			name: "no requirement",
			body: valid(func(f map[string]string) { f["requirement"] = "" }),
			want: "names no requirement",
		},
		{
			name: "no subject",
			body: valid(func(f map[string]string) { f["subject"] = "" }),
			want: "has no subject",
		},
		{
			name: "two subjects",
			body: valid(func(f map[string]string) { f["subject"] = "service: checkout\nteam: payments" }),
			want: "exactly one subject",
		},
		{
			name: "unknown field",
			body: valid(func(f map[string]string) { f["subject"] = "service: checkout\nexpries: 2026-09-01" }),
			want: "expries",
		},
		{
			name: "malformed date",
			body: valid(func(f map[string]string) { f["expires"] = "expires: next spring" }),
			want: "2006-01-02",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeExemptionsDir(t, map[string]string{"e.yaml": tc.body})
			got, err := LoadExemptions(dir)
			if err == nil {
				t.Fatal("load succeeded, want a failure that names the problem")
			}
			if len(got) != 0 {
				t.Error("a failed load must fail closed — no exemptions returned")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestExemptionDuplicateIDAcrossFiles(t *testing.T) {
	body := "id: e1\nrequirement: logs-delivered\nowner: someone\nexpires: 2026-09-01\nservice: checkout\n"
	dir := writeExemptionsDir(t, map[string]string{"a.yaml": body, "b.yaml": body})
	_, err := LoadExemptions(dir)
	if err == nil || !strings.Contains(err.Error(), "defined in both") {
		t.Fatalf("err = %v, want both files named for the duplicate id", err)
	}
}

// ADR-0037 §3: an expired Exemption left in the tree is an authoring
// finding — dead config — and one waiving a requirement the library does
// not hold can never take effect, which is almost always a typo whose
// author believes a waiver is in force.
func TestExemptionFindings(t *testing.T) {
	expired := Exemption{
		ID:          "old",
		Requirement: "logs-delivered",
		Owner:       "someone",
		Expires:     Date(time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)),
		Service:     "checkout",
	}
	typo := Exemption{
		ID:          "typo",
		Requirement: "logs-deliverd",
		Owner:       "someone",
		Expires:     Date(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)),
		Service:     "checkout",
	}
	live := Exemption{
		ID:          "live",
		Requirement: "logs-delivered",
		Owner:       "someone",
		Expires:     Date(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)),
		Service:     "checkout",
	}

	got := ExemptionFindings([]Exemption{expired, typo, live}, lib(req()), evalAt)
	if len(got) != 2 {
		t.Fatalf("findings = %+v, want exactly the expired and the typo surfaced", got)
	}
	if got[0].ExemptionID != "old" || !strings.Contains(got[0].Message, "expired 2026-08-01") {
		t.Errorf("expired finding = %+v", got[0])
	}
	if got[1].ExemptionID != "typo" || !strings.Contains(got[1].Message, "waives nothing") {
		t.Errorf("typo finding = %+v", got[1])
	}
}
