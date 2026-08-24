package schemaregistry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// snapshotDir is a representative custom registry: the OpenTelemetry
// conventions referenced, requirement levels tightened locally, the
// adopter's own namespaced attributes added, and the repository's own
// tooling files sitting beside the model.
var snapshotDir = filepath.Join("testdata", "registry-v1.4.0")

var snapshotSource = Source{
	Repository: "git.example.test/estate/registry",
	Ref:        "v1.4.0",
	Commit:     "3f2a1c8d5b7e9046a1c2d3e4f5061728394a5b6c",
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

// registryDir lays out a one-file registry: a manifest, and the model body
// given. It is the shortest thing the import will accept, so a test can say
// one thing at a time.
func registryDir(t *testing.T, model string) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "registry_manifest.yaml", "name: example-registry\nschema_url: https://schemas.example.test/registry/1.0.0\n")
	write(t, dir, "model/model.yaml", model)
	return dir
}

// importErr is Import reduced to its error, asserting that a failed import
// fails closed and returns no registry at all.
func importErr(t *testing.T, root string) error {
	t.Helper()
	reg, _, err := Import(root, snapshotSource)
	if err != nil && reg != nil {
		t.Fatal("Import failed but returned a registry: a failed import must fail closed")
	}
	return err
}

// The snapshot import is the acceptance surface: every shape a custom
// registry carries (a manifest with a dependency, declared attributes, enum
// members, a locally tightened reference, both requirement-level forms, both
// deprecation forms, an unknown group kind, and YAML that is not a model
// file) lands where it should.
func TestSnapshotImportsCompletely(t *testing.T) {
	reg, cov, err := Import(snapshotDir, snapshotSource)
	if err != nil {
		t.Fatalf("the snapshot registry does not import: %v", err)
	}

	if reg.Manifest.Name != "example-registry" {
		t.Errorf("manifest name is %q, want example-registry", reg.Manifest.Name)
	}
	if len(reg.Manifest.Dependencies) != 1 || reg.Manifest.Dependencies[0].Name != "otel" {
		t.Errorf("the manifest's dependency was not recorded: %+v", reg.Manifest.Dependencies)
	}
	if cov.Manifest != "registry_manifest.yaml" {
		t.Errorf("coverage records the manifest as %q", cov.Manifest)
	}

	wantCounts := map[Kind]int{AttributeGroup: 2, Span: 1, Metric: 1, Entity: 1}
	for kind, want := range wantCounts {
		if got := cov.Found[kind]; got != want {
			t.Errorf("coverage found %d %s groups, want %d", got, kind, want)
		}
		if got := len(reg.GroupsOfKind(kind)); got != want {
			t.Errorf("the registry holds %d %s groups, want %d", got, kind, want)
		}
	}
	if reg.Len() != 5 {
		t.Errorf("imported %d groups, want 5", reg.Len())
	}

	// A group kind the model does not know is excluded and reported, never
	// silently dropped and never a failed import.
	if len(cov.Excluded) != 1 || cov.Excluded[0].Kind != "profile" {
		t.Errorf("the unknown group kind was not excluded and reported: %+v", cov.Excluded)
	}
	if _, ok := reg.Group("profile.db.client"); ok {
		t.Error("a group of an unknown kind entered the registry")
	}

	// YAML that carries no groups is not a model file.
	if got := cov.Ignored; len(got) != 2 || got[0] != "ci.yaml" || got[1] != "docs/config.yml" {
		t.Errorf("ignored files are %v, want the two files carrying no groups", got)
	}

	// A declared attribute is found by id, and carries the file it came
	// from so remediation can name the line to edit.
	attr, group, ok := reg.Attribute("db.namespace")
	if !ok {
		t.Fatal("db.namespace is not defined in the registry")
	}
	if group.ID != "registry.db" || group.File != "model/db.yaml" {
		t.Errorf("db.namespace is declared in %s (%s), want registry.db in model/db.yaml", group.ID, group.File)
	}
	if attr.Type != "string" || attr.Stability != "stable" {
		t.Errorf("db.namespace is %+v, want a stable string", attr)
	}

	// An enum's members are recorded as literal values, which is the form
	// an observed attribute value is compared against.
	enum, _, ok := reg.Attribute("db.system.name")
	if !ok {
		t.Fatal("db.system.name is not defined in the registry")
	}
	if enum.Type != "enum" {
		t.Errorf("db.system.name has type %q, want enum", enum.Type)
	}
	var values []string
	for _, m := range enum.Members {
		values = append(values, m.Value)
	}
	if strings.Join(values, ",") != "mariadb,other_sql,postgresql" {
		t.Errorf("db.system.name members are %v, want the three declared values in id order", values)
	}

	// Both deprecation forms normalise to the same record.
	structured, _, _ := reg.Attribute("db.connection_string")
	if structured.Deprecation == nil || structured.Deprecation.RenamedTo != "db.namespace" {
		t.Errorf("the structured deprecation notice did not survive: %+v", structured.Deprecation)
	}
	prose, _, _ := reg.Attribute("enterprise.owner_email")
	if prose.Deprecation == nil || !strings.HasPrefix(prose.Deprecation.Note, "Replaced by the ownership tree") {
		t.Errorf("the prose deprecation notice did not survive: %+v", prose.Deprecation)
	}
}

// A reference is not a copy (ADR-0034 §2): a span group demands attributes
// and may tighten their level locally, and the definition stays in one
// place.
func TestReferencesCarryTheLocallyTightenedLevel(t *testing.T) {
	reg, cov, err := Import(snapshotDir, snapshotSource)
	if err != nil {
		t.Fatal(err)
	}
	span, ok := reg.Group("span.db.client")
	if !ok {
		t.Fatal("span.db.client is missing")
	}

	levels := map[string]Level{}
	conditions := map[string]string{}
	for _, a := range span.Attributes {
		if a.Defines() {
			t.Errorf("span.db.client defines %q: a signal group references attributes, it does not declare them", a.ID)
		}
		levels[a.Ref] = a.Level
		conditions[a.Ref] = a.Condition
	}
	if levels["db.namespace"] != Required {
		t.Errorf("db.namespace is %q on the span, want the locally tightened required", levels["db.namespace"])
	}
	if levels["server.port"] != Recommended {
		t.Errorf("server.port is %q on the span, want recommended", levels["server.port"])
	}
	if levels["db.operation.name"] != ConditionallyRequired {
		t.Errorf("db.operation.name is %q, want conditionally_required", levels["db.operation.name"])
	}
	if conditions["db.operation.name"] != "if the operation name is readily available" {
		t.Errorf("the condition did not survive: %q", conditions["db.operation.name"])
	}

	// An attribute defined in a dependency registry is referenced, never
	// resolved: the dependency is not in this tree and is never fetched.
	unresolved := map[string]bool{}
	for _, r := range cov.Unresolved {
		unresolved[r.Attribute] = true
	}
	for _, want := range []string{"server.address", "server.port", "service.name"} {
		if !unresolved[want] {
			t.Errorf("%s is not reported as coming from a dependency registry: %+v", want, cov.Unresolved)
		}
	}
	if _, _, ok := reg.Attribute("server.address"); ok {
		t.Error("server.address resolved: a reference is not a definition")
	}
	if cov.References != 9 {
		t.Errorf("counted %d references, want 9", cov.References)
	}
	if cov.Attributes != 7 {
		t.Errorf("counted %d defined attributes, want 7", cov.Attributes)
	}
}

// The manifest identifies the registry, and the import says so plainly when
// it is pointed somewhere that is not a registry root.
func TestManifestIsRequiredAndEitherNameIsAccepted(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "model/model.yaml", "groups:\n  - id: registry.a\n    type: attribute_group\n")
	err := importErr(t, dir)
	if err == nil {
		t.Fatal("a tree with no manifest imported as a registry")
	}
	for _, name := range manifestNames {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("the error does not name %s: %v", name, err)
		}
	}

	write(t, dir, "manifest.yaml", "name: example-registry\n")
	reg, cov, err := Import(dir, snapshotSource)
	if err != nil {
		t.Fatalf("the older manifest name was rejected: %v", err)
	}
	if cov.Manifest != "manifest.yaml" {
		t.Errorf("coverage records the manifest as %q", cov.Manifest)
	}
	if reg.Manifest.Name != "example-registry" {
		t.Errorf("manifest name is %q", reg.Manifest.Name)
	}
}

// Everything malformed fails the import closed, naming what is wrong. A
// silently wrong registry would corrupt every conformance judgement made
// against it.
func TestImportFailsClosed(t *testing.T) {
	cases := map[string]struct{ model, want string }{
		"duplicate group id": {
			"groups:\n  - id: registry.a\n    type: attribute_group\n  - id: registry.a\n    type: span\n",
			"declared twice",
		},
		"duplicate attribute id": {
			"groups:\n  - id: registry.a\n    type: attribute_group\n    attributes:\n      - id: a.one\n        type: string\n  - id: registry.b\n    type: attribute_group\n    attributes:\n      - id: a.one\n        type: string\n",
			"defined in both",
		},
		"attribute listed twice in one group": {
			"groups:\n  - id: registry.a\n    type: attribute_group\n    attributes:\n      - id: a.one\n        type: string\n      - id: a.one\n        type: int\n",
			"listed twice",
		},
		"both id and ref": {
			"groups:\n  - id: span.a\n    type: span\n    attributes:\n      - id: a.one\n        ref: a.two\n",
			"either defines an attribute or references one",
		},
		"neither id nor ref": {
			"groups:\n  - id: span.a\n    type: span\n    attributes:\n      - type: string\n",
			"neither an id nor a ref",
		},
		"no group id": {
			"groups:\n  - type: attribute_group\n",
			"no id",
		},
		"unknown requirement level": {
			"groups:\n  - id: span.a\n    type: span\n    attributes:\n      - ref: a.one\n        requirement_level: mandatory\n",
			"unknown requirement level",
		},
		"conditionally required with no condition": {
			"groups:\n  - id: span.a\n    type: span\n    attributes:\n      - ref: a.one\n        requirement_level: conditionally_required\n",
			"carries no condition",
		},
		"condition at a level that takes none": {
			"groups:\n  - id: span.a\n    type: span\n    attributes:\n      - ref: a.one\n        requirement_level:\n          required: whenever\n",
			"takes none",
		},
		"enum with no members": {
			"groups:\n  - id: registry.a\n    type: attribute_group\n    attributes:\n      - id: a.one\n        type:\n          note: not an enum\n",
			"declares no members",
		},
		"enum member with no id": {
			"groups:\n  - id: registry.a\n    type: attribute_group\n    attributes:\n      - id: a.one\n        type:\n          members:\n            - value: red\n",
			"member has no id",
		},
		"empty deprecation notice": {
			"groups:\n  - id: registry.a\n    type: attribute_group\n    attributes:\n      - id: a.one\n        type: string\n        deprecated: \"\"\n",
			"cannot say where to move",
		},
		"malformed yaml": {
			"groups:\n  - id: registry.a\n   type: attribute_group\n",
			"model/model.yaml",
		},
		"no groups at all": {
			"# a model file with nothing in it\n",
			"no groups",
		},
	}
	for name, tc := range cases {
		err := importErr(t, registryDir(t, tc.model))
		if err == nil {
			t.Errorf("%s: imported cleanly", name)
			continue
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error does not mention %q: %v", name, tc.want, err)
		}
	}
}

// A registry with no name is not a registry: the manifest is what names the
// version every requirement will reference.
func TestManifestMustNameTheRegistry(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "registry_manifest.yaml", "description: nameless\n")
	write(t, dir, "model/model.yaml", "groups:\n  - id: registry.a\n    type: attribute_group\n")
	err := importErr(t, dir)
	if err == nil || !strings.Contains(err.Error(), "manifest.name is empty") {
		t.Fatalf("a nameless registry imported: %v", err)
	}
}
