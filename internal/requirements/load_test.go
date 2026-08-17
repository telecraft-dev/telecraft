package requirements

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// write drops one library file into dir, creating parents as needed.
func write(t *testing.T, dir, name, body string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// loadErr is Load reduced to its error, for tests that only assert on failure.
func loadErr(t *testing.T, dir string) error {
	t.Helper()
	lib, err := Load(dir)
	if err != nil && len(lib.Requirements) != 0 {
		t.Fatalf("Load failed but returned a non-empty library — a failed load must fail closed")
	}
	return err
}

const goodReq = `
- id: logs-delivered
  title: Logs delivered
  version: 1
  requirement_level: required
  owner: platform-observability
  signal:
    kind: logs
    present: true
    window: 24h
  remediation: add a filelog receiver
`

// The fixture library is the acceptance surface: it loads, and every
// requirement exposes kind, requirement_level, owner and (optionally)
// environments.
func TestFixtureLibraryLoads(t *testing.T) {
	lib, err := Load(filepath.Join("testdata", "library"))
	if err != nil {
		t.Fatalf("the fixture library does not load: %v", err)
	}
	if len(lib.Requirements) != 7 {
		t.Fatalf("fixture library holds %d requirements, want 7", len(lib.Requirements))
	}
	for _, r := range lib.Sorted() {
		if r.Owner == "" {
			t.Errorf("requirement %q loaded without an owner", r.ID)
		}
		if !r.Level.Valid() {
			t.Errorf("requirement %q loaded with invalid level %q", r.ID, r.Level)
		}
		if k := r.Kind(); k != KindConfig && k != KindSignal && k != KindConfigAndSignal {
			t.Errorf("requirement %q reports unknown kind %q", r.ID, k)
		}
	}

	// Spot-check that the fields carry what was authored, not defaults.
	logs := lib.Requirements["logs-delivered"]
	if logs.Kind() != KindConfigAndSignal {
		t.Errorf("logs-delivered kind = %q, want %q", logs.Kind(), KindConfigAndSignal)
	}
	if logs.Level != Required {
		t.Errorf("logs-delivered requirement_level = %q, want required", logs.Level)
	}
	if len(logs.Environments) != 0 || !logs.AppliesTo("staging") {
		t.Error("logs-delivered has no environments list and must apply everywhere")
	}

	recent := lib.Requirements["traces-recent"]
	if len(recent.Environments) != 1 || recent.Environments[0] != "production" {
		t.Errorf("traces-recent environments = %v, want [production]", recent.Environments)
	}
	if !recent.AppliesTo("production") || recent.AppliesTo("staging") {
		t.Error("traces-recent must apply to production and only production")
	}
	if recent.Signal == nil || recent.Signal.Window.Std() != time.Hour {
		t.Errorf("traces-recent window not parsed as 1h: %+v", recent.Signal)
	}

	metrics := lib.Requirements["metrics-delivered"]
	if metrics.Level != ConditionallyRequired {
		t.Errorf("metrics-delivered requirement_level = %q, want conditionally_required", metrics.Level)
	}

	identity := lib.Requirements["service-name-on-logs"]
	if identity.Kind() != KindSignal {
		t.Errorf("service-name-on-logs kind = %q, want %q", identity.Kind(), KindSignal)
	}
	if got := identity.Signal.Coverage(); got != 0.99 {
		t.Errorf("service-name-on-logs coverage = %v, want 0.99", got)
	}
}

// An unknown field must fail the load naming the file and the field. Dropping
// it instead would let a misspelled key silently weaken a requirement.
func TestUnknownFieldFailsClosedWithFileAndField(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "r.yaml", `
- id: logs-delivered
  title: Logs delivered
  version: 1
  owner: platform-observability
  severty: high
  signal:
    kind: logs
    present: true
    window: 24h
  remediation: add a filelog receiver
`)
	err := loadErr(t, dir)
	if err == nil {
		t.Fatal("expected a load error for an unknown field")
	}
	if !strings.Contains(err.Error(), "r.yaml") {
		t.Errorf("error does not name the file: %v", err)
	}
	if !strings.Contains(err.Error(), "severty") {
		t.Errorf("error does not name the unknown field: %v", err)
	}
}

func TestMalformedYAMLFailsClosedNamingTheFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "broken.yaml", "{ this is: [not yaml")
	err := loadErr(t, dir)
	if err == nil {
		t.Fatal("expected a load error for malformed YAML")
	}
	if !strings.Contains(err.Error(), "broken.yaml") {
		t.Errorf("error does not name the file: %v", err)
	}
}

// REQ-023: a query string is not representable, so an authored attempt to
// smuggle one in dies at load as an unknown field.
func TestQueryFieldIsRejectedAtLoad(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "r.yaml", `
- id: sneaky
  title: Embeds a backend query
  version: 1
  owner: someone
  signal:
    kind: logs
    present: true
    window: 24h
    query: 'status:error AND service.name:checkout'
  remediation: n/a
`)
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "query") {
		t.Fatalf("expected a load error naming the query field, got %v", err)
	}
}

func TestDuplicateIDNamesBothFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.yaml", goodReq)
	write(t, dir, "b.yaml", goodReq)
	err := loadErr(t, dir)
	if err == nil {
		t.Fatal("expected a load error for a duplicate requirement id")
	}
	if !strings.Contains(err.Error(), "a.yaml") || !strings.Contains(err.Error(), "b.yaml") {
		t.Errorf("duplicate error does not name both files: %v", err)
	}
}

func TestOwnerlessRequirementIsRejected(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "r.yaml", `
- id: logs-delivered
  title: Logs delivered
  version: 1
  signal: {kind: logs, present: true, window: 24h}
  remediation: add a filelog receiver
`)
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "owner") {
		t.Fatalf("expected an owner error, got %v", err)
	}
}

// A finding with no suggested fix is a complaint. Every requirement must
// carry the change that closes it.
func TestRequirementWithoutRemediationIsRejected(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "r.yaml", `
- id: logs-delivered
  title: Logs delivered
  version: 1
  owner: platform-observability
  signal: {kind: logs, present: true, window: 24h}
`)
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "remediation") {
		t.Fatalf("expected a remediation error, got %v", err)
	}
}

func TestRequirementAssertingNothingIsRejected(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "r.yaml", `
- id: empty
  title: Asserts nothing
  version: 1
  owner: someone
  remediation: n/a
`)
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "asserts nothing") {
		t.Fatalf("expected an asserts-nothing error, got %v", err)
	}
}

func TestUnknownRequirementLevelIsRejected(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "r.yaml", `
- id: logs-delivered
  title: Logs delivered
  version: 1
  requirement_level: mandatory
  owner: platform-observability
  signal: {kind: logs, present: true, window: 24h}
  remediation: add a filelog receiver
`)
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "requirement_level") {
		t.Fatalf("expected a requirement_level error, got %v", err)
	}
}

// Absent requirement_level defaults to recommended, matching the upstream
// semantic-conventions default (ADR-0009) rather than inventing a dialect.
func TestAbsentLevelDefaultsToRecommended(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "r.yaml", `
- id: logs-delivered
  title: Logs delivered
  version: 1
  owner: platform-observability
  signal: {kind: logs, present: true, window: 24h}
  remediation: add a filelog receiver
`)
	lib, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := lib.Requirements["logs-delivered"].Level; got != Recommended {
		t.Fatalf("absent requirement_level loaded as %q, want recommended", got)
	}
}

func TestSignalAssertionValidation(t *testing.T) {
	cases := map[string]struct {
		body string
		want string
	}{
		"unknown signal kind": {
			body: `{kind: profiles, present: true, window: 24h}`,
			want: "signal kind",
		},
		"missing window": {
			body: `{kind: logs, present: true}`,
			want: "window",
		},
		"coverage above one": {
			body: `{kind: logs, present: true, window: 24h, attribute_coverage: 1.5}`,
			want: "attribute_coverage",
		},
		"coverage of zero asserts nothing": {
			body: `{kind: logs, present: true, window: 24h, attribute_coverage: 0}`,
			want: "attribute_coverage",
		},
		"negative volume": {
			body: `{kind: logs, present: true, window: 24h, min_volume: -1}`,
			want: "min_volume",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "r.yaml", `
- id: r
  title: T
  version: 1
  owner: someone
  signal: `+tc.body+`
  remediation: n/a
`)
			err := loadErr(t, dir)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected an error mentioning %q, got %v", tc.want, err)
			}
		})
	}
}

func TestWindowMustBeADurationString(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "r.yaml", `
- id: r
  title: T
  version: 1
  owner: someone
  signal: {kind: logs, present: true, window: yesterday}
  remediation: n/a
`)
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "r.yaml") {
		t.Fatalf("expected a window parse error naming the file, got %v", err)
	}
}

func TestEnvironmentsListValidation(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "r.yaml", `
- id: r
  title: T
  version: 1
  owner: someone
  environments: [production, production]
  signal: {kind: logs, present: true, window: 24h}
  remediation: n/a
`)
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "twice") {
		t.Fatalf("expected a duplicate-environment error, got %v", err)
	}
}

// A single mapping per file is as natural as a list; both load.
func TestSingleRequirementPerFileLoads(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "one.yaml", `
id: logs-delivered
title: Logs delivered
version: 1
owner: platform-observability
signal: {kind: logs, present: true, window: 24h}
remediation: add a filelog receiver
`)
	lib, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lib.Requirements["logs-delivered"]; !ok {
		t.Fatal("single-mapping file did not load")
	}
}

func TestMultipleYAMLDocumentsAreRejected(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "two.yaml", goodReq+"\n---\n- id: other\n")
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "one concern per file") {
		t.Fatalf("expected a multi-document error, got %v", err)
	}
}

func TestEmptyFileIsRejected(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "empty.yaml", "# nothing here\n")
	err := loadErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "empty.yaml") {
		t.Fatalf("expected an empty-file error, got %v", err)
	}
}

func TestMissingDirectoryIsAnError(t *testing.T) {
	if err := loadErr(t, filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected an error for a missing library directory")
	}
}

func TestEmptyLibraryIsAnError(t *testing.T) {
	if err := loadErr(t, t.TempDir()); err == nil {
		t.Fatal("an empty library must not load: it would judge everything compliant vacuously")
	}
}
