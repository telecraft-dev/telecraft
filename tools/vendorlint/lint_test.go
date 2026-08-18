package main

import (
	"regexp"
	"testing"
)

// TestSeededViolations runs the linter over the fixture tree and asserts the
// exact finding set: the ADR-0001 acceptance case — a vendor word seeded in a
// core path fails, and only the seeded lines fail.
func TestSeededViolations(t *testing.T) {
	result, err := Run("testdata/repo", "vendorlint.yaml")
	if err != nil {
		t.Fatal(err)
	}

	want := []string{
		`docs/example.md:5: [docs] "Fleet": a bare "Fleet" appears nowhere (ADR-0001)`,
		`internal/core/estate.go:3: [core] "Fleet": vendor word in the neutral core (ADR-0001)`,
		`internal/core/estate.go:5: [core] "elastic": vendor word in the neutral core (ADR-0001)`,
		`internal/core/estate.go:9: [core] "GitHub": forge vendor word in the neutral core (ADR-0028 §4)`,
		`internal/provider/telemetry/telemetry.go:3: [provider] "Fleet": qualify the product (ADR-0001)`,
	}
	got := make([]string, len(result.Findings))
	for i, f := range result.Findings {
		got[i] = f.String()
	}
	if len(got) != len(want) {
		t.Fatalf("got %d findings, want %d:\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("finding %d:\ngot:  %s\nwant: %s", i, got[i], want[i])
		}
	}
}

// TestGlobs pins the glob dialect: ** crosses separators, * does not.
func TestGlobs(t *testing.T) {
	cases := []struct {
		glob, path string
		want       bool
	}{
		{"internal/**", "internal/core/estate.go", true},
		{"internal/**", "internal/x.go", true},
		{"internal/**", "cmd/x.go", false},
		{"internal/provider/**", "internal/provider/telemetry/t.go", true},
		{"internal/provider/**", "internal/core/estate.go", false},
		{"README.md", "README.md", true},
		{"README.md", "docs/README.md", false},
		{"docs/*.md", "docs/glossary.md", true},
		{"docs/*.md", "docs/adr/0001.md", false},
	}
	for _, c := range cases {
		re, err := globToRegexp(c.glob)
		if err != nil {
			t.Fatal(err)
		}
		if got := re.MatchString(c.path); got != c.want {
			t.Errorf("glob %q vs %q: got %v, want %v", c.glob, c.path, got, c.want)
		}
	}
}

// TestAllowSpans pins the allow semantics: a match survives only when fully
// inside an allow-pattern match on the same line.
func TestAllowSpans(t *testing.T) {
	r := compiledRule{
		pattern: mustCompile(t, `\bFleet\b`),
		allow:   []*regexp.Regexp{mustCompile(t, `Elastic Fleet`)},
	}
	if got := violations(r, "Elastic Fleet is monitoring-only"); len(got) != 0 {
		t.Errorf("qualified use flagged: %v", got)
	}
	if got := violations(r, "Fleet redacts on key names"); len(got) != 1 {
		t.Errorf("bare use not flagged exactly once: %v", got)
	}
	if got := violations(r, "Elastic Fleet plus a bare Fleet"); len(got) != 1 {
		t.Errorf("mixed line should flag only the bare use: %v", got)
	}
}

func mustCompile(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()
	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatal(err)
	}
	return re
}
