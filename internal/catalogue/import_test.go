package catalogue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// snapshotDir is the testdata snapshot of representative upstream files from
// opentelemetry-collector-contrib at v0.158.0 — verbatim metadata.yaml, with
// go.mod trimmed to the module line the walker reads. A snapshot of upstream
// *inputs* is not hand-curation of the component list: the list is whatever
// the walker finds, and these files are upstream's, not ours.
var snapshotDir = filepath.Join("testdata", "contrib-v0.158.0")

var snapshotSource = Source{
	Repository: "github.com/open-telemetry/opentelemetry-collector-contrib",
	Ref:        "v0.158.0",
	Commit:     "821a9d9c2c1623c4a0ceba5d47b57c48879c3f84",
}

// write drops one file into dir, creating parents as needed.
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

// writeComponent lays out one component directory — a metadata.yaml with a
// sibling go.mod, which is what makes the walker see it.
func writeComponent(t *testing.T, root, dir, metadata string) {
	t.Helper()
	write(t, root, filepath.Join(dir, "go.mod"), "module example.com/collector/"+dir+"\n\ngo 1.26\n")
	write(t, root, filepath.Join(dir, "metadata.yaml"), metadata)
}

// importErr is Import reduced to its error, asserting that a failed import
// fails closed and returns no Catalogue at all.
func importErr(t *testing.T, root string) error {
	t.Helper()
	cat, _, err := Import(root, snapshotSource)
	if err != nil && cat != nil {
		t.Fatal("Import failed but returned a catalogue — a failed import must fail closed")
	}
	return err
}

// The snapshot import is the acceptance surface: every representative
// upstream shape — nested extensions, per-signal stability divergence,
// deprecated_type aliases, a deprecation notice, a non-pipeline class, a
// module with no metadata.yaml — lands where it should.
func TestSnapshotImportsCompletely(t *testing.T) {
	cat, cov, err := Import(snapshotDir, snapshotSource)
	if err != nil {
		t.Fatalf("the snapshot tree does not import: %v", err)
	}

	if cat.Len() != 9 {
		t.Fatalf("imported %d components, want 9", cat.Len())
	}
	wantCounts := map[Class]int{Receiver: 2, Processor: 1, Exporter: 2, Connector: 1, Extension: 3}
	for class, want := range wantCounts {
		if got := cov.Found[class]; got != want {
			t.Errorf("coverage found %d %s components, want %d", got, class, want)
		}
		if got := len(cat.ByClass(class)); got != want {
			t.Errorf("catalogue holds %d %s components, want %d", got, class, want)
		}
	}

	// Nested extensions are the depth-1 trap (R-1 §2a): both must be found.
	for _, typ := range []string{"file_storage", "k8s_observer", "json_log_encoding"} {
		if _, ok := cat.Lookup(Extension, typ); !ok {
			t.Errorf("nested extension %q was not discovered", typ)
		}
	}

	// Per-signal stability divergence must survive the import intact.
	kafka, ok := cat.Lookup(Receiver, "kafka")
	if !ok {
		t.Fatal("receiver kafka not imported")
	}
	if l, _ := kafka.StabilityFor("logs"); l != Beta {
		t.Errorf("kafka receiver logs stability = %q, want beta", l)
	}
	if l, _ := kafka.StabilityFor("profiles"); l != Development {
		t.Errorf("kafka receiver profiles stability = %q, want development", l)
	}
	if _, supported := kafka.StabilityFor("nonexistent"); supported {
		t.Error("kafka receiver claims a signal it does not carry")
	}
	if kafka.Module != "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/kafkareceiver" {
		t.Errorf("kafka receiver module = %q — the go.mod anchor was not recorded", kafka.Module)
	}

	// Lifecycle: the deprecated exporter carries its upstream notice.
	mezmo, ok := cat.Lookup(Exporter, "mezmo")
	if !ok {
		t.Fatal("exporter mezmo not imported")
	}
	if l, _ := mezmo.StabilityFor("logs"); l != Deprecated {
		t.Errorf("mezmo logs stability = %q, want deprecated", l)
	}
	if d, ok := mezmo.Deprecation["logs"]; !ok || d.Migration == "" || d.Date != "2026-07-30" {
		t.Errorf("mezmo deprecation notice not carried: %+v", mezmo.Deprecation)
	}

	// The coverage report accounts for what did NOT enter the Catalogue:
	// the class: pkg module, and the class-less observer helper package.
	wantExcluded := []Exclusion{
		{Dir: "extension/observer", Class: ""},
		{Dir: "pkg/golden", Class: "pkg"},
	}
	if len(cov.Excluded) != len(wantExcluded) {
		t.Fatalf("excluded = %+v, want %+v", cov.Excluded, wantExcluded)
	}
	for i, want := range wantExcluded {
		if cov.Excluded[i] != want {
			t.Errorf("excluded[%d] = %+v, want %+v", i, cov.Excluded[i], want)
		}
	}
	if !strings.Contains(cov.String(), "(no class)") {
		t.Error("the printed report does not label the class-less exclusion legibly")
	}
	wantMissing := "extension/tailstorage/pebbletailstorageextension/integrationtest"
	if len(cov.Missing) != 1 || cov.Missing[0] != wantMissing {
		t.Errorf("missing = %v, want exactly [%s]", cov.Missing, wantMissing)
	}
	if !strings.Contains(cov.String(), wantMissing) {
		t.Error("the printed coverage report does not list the missing module")
	}

	// internal/ directories are Go scaffolding, never components: the module
	// under extension/internal/ must be skipped entirely, not reported.
	for _, m := range cov.Missing {
		if strings.Contains(m, "internal/") {
			t.Errorf("internal module %q leaked into the coverage report", m)
		}
	}
}

// (class, type) is the primary key: the same type string in two classes is
// two components (ADR-0020 §3). `type` alone would collapse them.
func TestClassTypeIsThePrimaryKey(t *testing.T) {
	cat, _, err := Import(snapshotDir, snapshotSource)
	if err != nil {
		t.Fatal(err)
	}
	rcv, ok1 := cat.Lookup(Receiver, "kafka")
	exp, ok2 := cat.Lookup(Exporter, "kafka")
	if !ok1 || !ok2 {
		t.Fatal("kafka must exist as both a receiver and an exporter")
	}
	if rcv.Module == exp.Module {
		t.Error("receiver and exporter kafka resolved to the same module — the key collapsed")
	}
}

// deprecated_type aliases resolve on lookup, so a config using the old
// spelling never hits a false "not in Catalogue" (R-1 §7).
func TestDeprecatedTypeAliasesResolve(t *testing.T) {
	cat, _, err := Import(snapshotDir, snapshotSource)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		class Class
		alias string
		want  string
	}{
		"connector spanmetrics": {Connector, "spanmetrics", "span_metrics"},
		"receiver filelog":      {Receiver, "filelog", "file_log"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			comp, ok := cat.Lookup(tc.class, tc.alias)
			if !ok {
				t.Fatalf("alias %q did not resolve", tc.alias)
			}
			if comp.Type != tc.want {
				t.Errorf("alias %q resolved to %q, want %q", tc.alias, comp.Type, tc.want)
			}
		})
	}
}

// A module under a component root with no metadata.yaml is a gap the
// coverage report must surface — never an error, never silence.
func TestMissingMetadataIsReportedNeverSilent(t *testing.T) {
	root := t.TempDir()
	writeComponent(t, root, "receiver/widgetreceiver", "type: widget\nstatus:\n  class: receiver\n  stability:\n    beta: [logs]\n")
	write(t, root, "receiver/lonely/go.mod", "module example.com/collector/receiver/lonely\n")

	_, cov, err := Import(root, snapshotSource)
	if err != nil {
		t.Fatal(err)
	}
	if len(cov.Missing) != 1 || cov.Missing[0] != "receiver/lonely" {
		t.Fatalf("missing = %v, want [receiver/lonely]", cov.Missing)
	}
	if !strings.Contains(cov.String(), "receiver/lonely") {
		t.Error("the printed report does not name the gap")
	}
}

// A parsed component of a non-pipeline class — including one whose class we
// have never seen, since the upstream enum grew twice in two years — is
// excluded and recorded, not dropped and not fatal.
func TestNonPipelineClassesAreExcludedAndRecorded(t *testing.T) {
	root := t.TempDir()
	writeComponent(t, root, "receiver/widgetreceiver", "type: widget\nstatus:\n  class: receiver\n  stability:\n    beta: [logs]\n")
	writeComponent(t, root, "receiver/helperlib", "type: helperlib\nstatus:\n  class: pkg\n")
	writeComponent(t, root, "confmap/futurething", "type: futurething\nstatus:\n  class: brand_new_class\n")

	cat, cov, err := Import(root, snapshotSource)
	if err != nil {
		t.Fatal(err)
	}
	if cat.Len() != 1 {
		t.Fatalf("imported %d components, want 1", cat.Len())
	}
	if len(cov.Excluded) != 2 {
		t.Fatalf("excluded = %+v, want two entries", cov.Excluded)
	}
	classes := map[string]bool{}
	for _, e := range cov.Excluded {
		classes[e.Class] = true
	}
	if !classes["pkg"] || !classes["brand_new_class"] {
		t.Errorf("excluded classes not recorded verbatim: %+v", cov.Excluded)
	}
	if !strings.Contains(cov.String(), "brand_new_class") {
		t.Error("the printed report does not surface the unknown class")
	}
}

func TestMalformedMetadataFailsClosedNamingTheFile(t *testing.T) {
	root := t.TempDir()
	write(t, root, "receiver/badreceiver/go.mod", "module example.com/collector/receiver/badreceiver\n")
	write(t, root, "receiver/badreceiver/metadata.yaml", "{ this is: [not yaml")
	err := importErr(t, root)
	if err == nil || !strings.Contains(err.Error(), "badreceiver") {
		t.Fatalf("expected an error naming the file, got %v", err)
	}
}

func TestPipelineComponentWithoutStabilityFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeComponent(t, root, "receiver/widgetreceiver", "type: widget\nstatus:\n  class: receiver\n")
	err := importErr(t, root)
	if err == nil || !strings.Contains(err.Error(), "stability") {
		t.Fatalf("expected a stability error, got %v", err)
	}
}

func TestUnknownStabilityLevelFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeComponent(t, root, "receiver/widgetreceiver", "type: widget\nstatus:\n  class: receiver\n  stability:\n    rock_solid: [logs]\n")
	err := importErr(t, root)
	if err == nil || !strings.Contains(err.Error(), "rock_solid") {
		t.Fatalf("expected an unknown-level error, got %v", err)
	}
}

// One signal under two levels is upstream data that cannot be true; taking
// either level would be a silent guess.
func TestSignalUnderTwoLevelsFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeComponent(t, root, "receiver/widgetreceiver", "type: widget\nstatus:\n  class: receiver\n  stability:\n    alpha: [logs]\n    beta: [logs]\n")
	err := importErr(t, root)
	if err == nil || !strings.Contains(err.Error(), "single-valued") {
		t.Fatalf("expected a two-levels error, got %v", err)
	}
}

func TestDeprecatedSignalWithoutNoticeFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeComponent(t, root, "exporter/oldexporter", "type: old\nstatus:\n  class: exporter\n  stability:\n    deprecated: [logs]\n")
	err := importErr(t, root)
	if err == nil || !strings.Contains(err.Error(), "deprecation") {
		t.Fatalf("expected a missing-notice error, got %v", err)
	}
}

// Two directories claiming the same (class, type) would make lookups
// ambiguous; the import refuses, naming both modules.
func TestDuplicateKeyFailsClosedNamingBothModules(t *testing.T) {
	root := t.TempDir()
	meta := "type: widget\nstatus:\n  class: receiver\n  stability:\n    beta: [logs]\n"
	writeComponent(t, root, "receiver/widgetreceiver", meta)
	writeComponent(t, root, "receiver/widgetv2receiver", meta)
	err := importErr(t, root)
	if err == nil {
		t.Fatal("expected a duplicate-key error")
	}
	for _, mod := range []string{"widgetreceiver", "widgetv2receiver"} {
		if !strings.Contains(err.Error(), mod) {
			t.Errorf("duplicate error does not name %s: %v", mod, err)
		}
	}
}

// An alias equal to another component's real type would let one component
// shadow another's identity on lookup (ADR-0020 §10) — refused.
func TestAliasCollidingWithARealTypeFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeComponent(t, root, "receiver/widgetreceiver", "type: widget\nstatus:\n  class: receiver\n  stability:\n    beta: [logs]\n")
	writeComponent(t, root, "receiver/gadgetreceiver", "type: gadget\ndeprecated_type: widget\nstatus:\n  class: receiver\n  stability:\n    beta: [logs]\n")
	err := importErr(t, root)
	if err == nil || !strings.Contains(err.Error(), "deprecated_type") {
		t.Fatalf("expected an alias-collision error, got %v", err)
	}
}

func TestEmptyTreeIsAnError(t *testing.T) {
	if err := importErr(t, t.TempDir()); err == nil {
		t.Fatal("expected an error for a tree with no Go modules")
	}
}

func TestTreeWithNoComponentsIsAnError(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module example.com/collector\n")
	err := importErr(t, root)
	if err == nil || !strings.Contains(err.Error(), "no components") {
		t.Fatalf("expected a no-components error, got %v", err)
	}
}
