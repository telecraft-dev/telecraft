package requirements

import (
	"strings"
	"testing"
)

func libWithEnvironments(t *testing.T, environments string) Library {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "r.yaml", `
- id: scoped
  title: Scoped requirement
  version: 1
  owner: someone
  environments: `+environments+`
  signal: {kind: logs, present: true, window: 24h}
  remediation: n/a
`)
	lib, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return lib
}

// ADR-0033 §3: an environments list matching no known environment must
// surface, not silently never apply. It is a finding rather than a load
// error because the environment vocabulary is open and adopter-defined —
// the loader has no authority to reject a name the estate might grow.
func TestNeverMatchingEnvironmentsListIsAnAuthoringFinding(t *testing.T) {
	lib := libWithEnvironments(t, "[prod]") // typo: the estate says "production"

	findings := lib.EnvironmentFindings([]string{"production", "staging"})
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(findings), findings)
	}
	f := findings[0]
	if f.RequirementID != "scoped" {
		t.Errorf("finding names requirement %q, want scoped", f.RequirementID)
	}
	if !strings.Contains(f.Message, "never apply") || !strings.Contains(f.Message, `"prod"`) {
		t.Errorf("finding does not say the requirement never applies, naming the environment: %s", f.Message)
	}
}

// One unknown entry among known ones is still a finding — that entry never
// matches — but the message must not claim the whole requirement is dead.
func TestUnknownEntryAmongKnownOnesIsAnEntryFinding(t *testing.T) {
	lib := libWithEnvironments(t, "[production, prdouction]")

	findings := lib.EnvironmentFindings([]string{"production"})
	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(findings), findings)
	}
	f := findings[0]
	if !strings.Contains(f.Message, `"prdouction"`) {
		t.Errorf("finding does not name the unknown entry: %s", f.Message)
	}
	if strings.Contains(f.Message, "never apply") {
		t.Errorf("finding wrongly claims the requirement never applies: %s", f.Message)
	}
}

func TestKnownEnvironmentsRaiseNoFinding(t *testing.T) {
	lib := libWithEnvironments(t, "[production]")
	if fs := lib.EnvironmentFindings([]string{"production", "staging"}); len(fs) != 0 {
		t.Fatalf("unexpected findings: %v", fs)
	}
}

func TestAbsentEnvironmentsListRaisesNoFinding(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "r.yaml", goodReq)
	lib, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Absent means "applies everywhere" — nothing to check against the
	// estate, even an estate with no known environments at all.
	if fs := lib.EnvironmentFindings(nil); len(fs) != 0 {
		t.Fatalf("unexpected findings for an env-neutral requirement: %v", fs)
	}
}
