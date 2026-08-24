package requirements

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/telecraft-dev/telecraft/internal/schemaregistry"
)

// snapshotRef is the ref the fixture Schema Registry version is imported at,
// and the version the fixture library pins.
const snapshotRef = "v1.4.0"

// installedRegistries imports the Schema Registry fixture that lives beside
// the schemaregistry package and writes it out as an installed artefact, the
// way an import run would. The fixture is reused rather than copied for the
// reason a requirement references a registry rather than copying one: a
// second registry in this package's testdata would drift from the first.
func installedRegistries(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := schemaregistry.Source{
		Repository: "git.example.test/estate/registry",
		Ref:        snapshotRef,
		Commit:     "3f2a1c8d5b7e9046a1c2d3e4f5061728394a5b6c",
	}
	reg, _, err := schemaregistry.Import(filepath.Join("..", "schemaregistry", "testdata", "registry-"+snapshotRef), src)
	if err != nil {
		t.Fatalf("importing the fixture Schema Registry: %v", err)
	}
	if _, _, err := reg.Write(dir); err != nil {
		t.Fatalf("installing the fixture Schema Registry: %v", err)
	}
	return dir
}

// rejected copies one fixture out of testdata/rejected into a library of its
// own, so each refusal is asserted against one file saying one thing.
func rejected(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "rejected", name))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write(t, dir, name, string(body))
	return dir
}

// The fixture schema requirements are references: a pinned version and a
// scope, with no attribute list anywhere in them.
func TestSchemaRequirementsLoadAsReferences(t *testing.T) {
	lib, err := Load(filepath.Join("testdata", "library"), WithSchemaRegistries(installedRegistries(t)))
	if err != nil {
		t.Fatalf("the fixture library does not load: %v", err)
	}

	pinned := lib.Requirements["db-spans-conform"]
	if pinned.Kind() != KindSchemaConformance {
		t.Errorf("db-spans-conform kind = %q, want %q", pinned.Kind(), KindSchemaConformance)
	}
	if pinned.Schema == nil {
		t.Fatal("db-spans-conform loaded without a schema assertion")
	}
	if pinned.Schema.RegistryVersion != snapshotRef || pinned.Schema.Tracking() {
		t.Errorf("db-spans-conform reference = %+v, want a pin on %s", pinned.Schema, snapshotRef)
	}
	if got := pinned.Schema.Scope.Groups; len(got) != 1 || got[0] != "span.db.client" {
		t.Errorf("db-spans-conform scope groups = %v, want [span.db.client]", got)
	}
	if !pinned.Schema.Covers(Traces) || pinned.Schema.Covers(Logs) {
		t.Errorf("db-spans-conform signals = %v, want traces only", pinned.Schema.Signals)
	}
	if pinned.Schema.Window.Std() != 24*time.Hour {
		t.Errorf("db-spans-conform window = %v, want 24h", pinned.Schema.Window.Std())
	}
	if len(pinned.Schema.Attributes) != 0 || len(pinned.Schema.RequiredAttributes) != 0 {
		t.Error("db-spans-conform carries an inline attribute list, which no loaded requirement may hold")
	}
	// Absent placement is landed: the reading the product exists for.
	if pinned.Placement != Landed {
		t.Errorf("db-spans-conform placement = %q, want landed by default", pinned.Placement)
	}
	if !pinned.AppliesTo("production") || pinned.AppliesTo("staging") {
		t.Error("db-spans-conform must apply to production and only production")
	}

	tracking := lib.Requirements["enterprise-attributes-tracked"]
	if tracking.Schema == nil || !tracking.Schema.Tracking() {
		t.Fatalf("enterprise-attributes-tracked did not load as a tracking reference: %+v", tracking.Schema)
	}
	if tracking.Schema.RegistryVersion != "" {
		t.Errorf("a tracking reference pinned %q as well", tracking.Schema.RegistryVersion)
	}
	if got := tracking.Schema.Scope.Namespaces; len(got) != 1 || got[0] != "enterprise" {
		t.Errorf("enterprise-attributes-tracked namespaces = %v, want [enterprise]", got)
	}
	if tracking.Placement != Landed {
		t.Errorf("enterprise-attributes-tracked placement = %q, want landed", tracking.Placement)
	}
}

// Every refusal the new kind carries, one fixture file each. The message
// matters as much as the refusal: an author who wrote the wrong thing has to
// be told which file, and what to write instead.
func TestSchemaConformanceRefusals(t *testing.T) {
	regs := WithSchemaRegistries(installedRegistries(t))
	cases := map[string]string{
		"inline-attributes.yaml":          "never a copy",
		"inline-required-attributes.yaml": "never a copy",
		"unresolvable-version.yaml":       "is not installed",
		"unknown-group.yaml":              "does not declare",
		"unknown-namespace.yaml":          "carries no attribute in",
		"no-version.yaml":                 "names no Schema Registry version",
		"pinned-and-tracking.yaml":        "both pins a Schema Registry version and tracks head",
		"unknown-track-mode.yaml":         "is not a tracking mode",
		"live-placement.yaml":             "not implemented yet",
		"unknown-placement.yaml":          "unknown placement",
		"placement-without-schema.yaml":   "only a schema_conformance requirement has a placement",
		"empty-scope.yaml":                "empty schema_conformance scope",
		"no-signals.yaml":                 "covers no signals",
		"no-window.yaml":                  "positive schema_conformance window",
		"schema-and-signal.yaml":          "judged against the Schema Registry alone",
		"asserts-nothing.yaml":            "asserts nothing",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			err := loadErr(t, rejected(t, name), regs)
			if err == nil {
				t.Fatalf("%s loaded, and it must not", name)
			}
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error does not say why:\n got  %v\n want it to mention %q", err, want)
			}
			if !strings.Contains(err.Error(), name) {
				t.Errorf("error does not name the file: %v", err)
			}
		})
	}
}

// A reference nobody can resolve is the failure this loader exists to
// refuse. A load given no Schema Registry directory can resolve nothing, so
// it fails rather than passing on a reference it never checked.
func TestSchemaReferenceWithoutARegistryDirectoryIsRejected(t *testing.T) {
	err := loadErr(t, filepath.Join("testdata", "library"))
	if err == nil {
		t.Fatal("a schema reference loaded with no Schema Registry to resolve it against")
	}
	if !strings.Contains(err.Error(), "no Schema Registry directory") {
		t.Errorf("error does not say what is missing: %v", err)
	}
	if !strings.Contains(err.Error(), "schema.yaml") {
		t.Errorf("error does not name the file: %v", err)
	}
}

// A tracking reference names no version, so there is nothing to resolve
// against; what must exist is a registry for there to be a head at all.
func TestTrackingReferenceNeedsAnInstalledRegistry(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "r.yaml", `
id: tracks-head
title: Tracks head
version: 1
owner: platform-observability
schema_conformance:
  track: head
  scope:
    namespaces: [enterprise]
  signals: [traces]
  window: 24h
remediation: import a registry version
`)
	err := loadErr(t, dir, WithSchemaRegistries(t.TempDir()))
	if err == nil || !strings.Contains(err.Error(), "no Schema Registry version is installed") {
		t.Fatalf("expected an uninstalled-registry error, got %v", err)
	}
}

// The fetch planner reads LongestWindow to decide how much history to ask a
// TelemetryProvider for. A schema window left out of it would have the
// evaluator asking for a reading nobody fetched.
func TestLongestWindowAccountsForSchemaWindows(t *testing.T) {
	lib := Library{Requirements: map[string]Requirement{
		"signal": {ID: "signal", Signal: &SignalAssertion{Kind: Logs, Window: Duration(time.Hour)}},
		"schema": {ID: "schema", Schema: &SchemaAssertion{Window: Duration(72 * time.Hour)}},
	}}
	if got := lib.LongestWindow(); got != 72*time.Hour {
		t.Fatalf("LongestWindow = %v, want the 72h schema window", got)
	}
}
