package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The composition of a Supervisor configuration: the renderer's artefact
// plus the identity the operator supplies. Every assertion below is about
// the two halves staying distinguishable — the base is reproducible from
// the estate, the overlay is not, and a merge that quietly dropped either
// would make the devenv's collectors unlike the ones it claims to model.

func TestMergeMapsOverlayWinsAndMapsMergeRecursively(t *testing.T) {
	base := map[string]any{
		"server":       map[string]any{"endpoint": "ws://base/v1/opamp"},
		"capabilities": map[string]any{"accepts_remote_config": true, "reports_health": true},
		"agent":        map[string]any{"automatic_config_rollback": true},
	}
	over := map[string]any{
		"agent":        map[string]any{"executable": "/otelcol-contrib"},
		"capabilities": map[string]any{"reports_health": false},
	}

	got := mergeMaps(base, over)

	agent := got["agent"].(map[string]any)
	if agent["automatic_config_rollback"] != true {
		t.Error("the base's agent settings were dropped rather than merged into")
	}
	if agent["executable"] != "/otelcol-contrib" {
		t.Error("the overlay's agent settings did not land")
	}
	if caps := got["capabilities"].(map[string]any); caps["reports_health"] != false {
		t.Error("the overlay did not win on a key both halves set")
	} else if caps["accepts_remote_config"] != true {
		t.Error("merging one capability dropped the others")
	}
	if server := got["server"].(map[string]any); server["endpoint"] != "ws://base/v1/opamp" {
		t.Error("a key the overlay never mentions was not carried through")
	}
}

func TestMergeMapsLeavesItsInputsAlone(t *testing.T) {
	base := map[string]any{"agent": map[string]any{"rollback": true}}
	over := map[string]any{"agent": map[string]any{"executable": "/otelcol"}}

	mergeMaps(base, over)

	if _, ok := base["agent"].(map[string]any)["executable"]; ok {
		t.Error("the base was mutated — composing one collector would leak into the next")
	}
}

func TestMergeMapsReplacesSequences(t *testing.T) {
	base := map[string]any{"agent": map[string]any{"args": []any{"--one", "--two"}}}
	over := map[string]any{"agent": map[string]any{"args": []any{"--only"}}}

	got := mergeMaps(base, over)["agent"].(map[string]any)["args"].([]any)

	// Appending would mean an overlay could never shorten a list, and these
	// are settings rather than accumulations.
	if len(got) != 1 || got[0] != "--only" {
		t.Errorf("sequence merged rather than replaced: %v", got)
	}
}

func TestComposeJoinsTheRenderedArtefactToTheIdentity(t *testing.T) {
	root := estateFixture(t, "platform", "gateway", ""+
		"server:\n"+
		"  endpoint: ws://host.docker.internal:4320/v1/opamp\n"+
		"capabilities:\n"+
		"  accepts_remote_config: true\n"+
		"storage:\n"+
		"  directory: /var/lib/telecraft/supervisor\n")
	identity := identityFixture(t, ""+
		"base_tier: platform/gateway\n"+
		"overlay:\n"+
		"  agent:\n"+
		"    executable: /otelcol-contrib\n"+
		"    description:\n"+
		"      identifying_attributes:\n"+
		"        telecraft.tier: gateway\n")

	composed, err := compose(root, identity)
	if err != nil {
		t.Fatalf("compose: %v", err)
	}

	var got map[string]any
	if err := yaml.Unmarshal(composed, &got); err != nil {
		t.Fatalf("the composed configuration does not parse: %v", err)
	}
	if server := got["server"].(map[string]any); server["endpoint"] != "ws://host.docker.internal:4320/v1/opamp" {
		t.Error("the endpoint the renderer decided did not survive composition")
	}
	if storage := got["storage"].(map[string]any); storage["directory"] != "/var/lib/telecraft/supervisor" {
		t.Error("the durable storage directory did not survive composition")
	}
	desc := got["agent"].(map[string]any)["description"].(map[string]any)
	attrs := desc["identifying_attributes"].(map[string]any)
	if attrs["telecraft.tier"] != "gateway" {
		t.Error("the identity the operator supplies did not land")
	}
	if !strings.Contains(string(composed), "Generated: edit the identity file") {
		t.Error("the composed file does not say it is generated, so somebody will edit it")
	}
}

func TestComposeRefusesAnIdentityWithNoBase(t *testing.T) {
	root := estateFixture(t, "platform", "gateway", "server:\n  endpoint: ws://x/v1/opamp\n")
	identity := identityFixture(t, "overlay:\n  agent:\n    executable: /otelcol-contrib\n")

	_, err := compose(root, identity)

	if err == nil {
		t.Fatal("an identity naming no base_tier composed anyway")
	}
	if !strings.Contains(err.Error(), "base_tier") {
		t.Errorf("the refusal does not name what is missing: %v", err)
	}
}

func TestComposeRefusesATierThatRendersNoSupervisorArtefact(t *testing.T) {
	root := t.TempDir()
	identity := identityFixture(t, "base_tier: platform/git-delivered\noverlay: {}\n")

	_, err := compose(root, identity)

	if err == nil {
		t.Fatal("a Tier with no serving block composed a Supervisor configuration anyway")
	}
	// A Tier renders a supervisor artefact only when it declares serving,
	// so the absent file is the estate saying "this one is not served".
	if !strings.Contains(err.Error(), "serving block") {
		t.Errorf("the refusal does not explain why the artefact is absent: %v", err)
	}
}

func TestComposeRefusesAnUnqualifiedTierID(t *testing.T) {
	root := t.TempDir()
	identity := identityFixture(t, "base_tier: gateway\noverlay: {}\n")

	if _, err := compose(root, identity); err == nil {
		t.Fatal("a Tier id with no team composed anyway")
	}
}

func TestComposeRefusesAnUnknownField(t *testing.T) {
	root := estateFixture(t, "platform", "gateway", "server:\n  endpoint: ws://x/v1/opamp\n")
	identity := identityFixture(t, "base_tier: platform/gateway\nidentifying_attributes:\n  a: b\n")

	// Strict loading, like every authored file in an estate: a
	// misremembered key is an error naming the file, never a collector
	// that silently reports nothing.
	if _, err := compose(root, identity); err == nil {
		t.Fatal("an identity file with a top-level typo composed anyway")
	}
}

// estateFixture writes one rendered Supervisor artefact and returns the
// estate root holding it.
func estateFixture(t *testing.T, team, tier, body string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "rendered", team)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, tier+".supervisor.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// identityFixture writes one identity file and returns its path.
func identityFixture(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "collector.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWithOverlayKeepsTheHeaderAndMergesBelowIt(t *testing.T) {
	root := estateFixture(t, "platform", "gateway", "server:\n  endpoint: ws://x/v1/opamp\n")
	identity := identityFixture(t, "base_tier: platform/gateway\noverlay:\n  agent:\n    executable: /otelcol-contrib\n")
	composed, err := compose(root, identity)
	if err != nil {
		t.Fatal(err)
	}

	drifted, err := withOverlay(composed, map[string]any{
		"agent": map[string]any{"config_files": []any{"/etc/telecraft/drift/local.yaml"}},
	})
	if err != nil {
		t.Fatalf("withOverlay: %v", err)
	}

	if !strings.HasPrefix(string(drifted), "# Composed by telecraft-devenv") {
		t.Error("the generated header was lost, so the file no longer says not to edit it")
	}
	var got map[string]any
	if err := yaml.Unmarshal(drifted, &got); err != nil {
		t.Fatalf("the drifted configuration does not parse: %v", err)
	}
	agent := got["agent"].(map[string]any)
	if agent["executable"] != "/otelcol-contrib" {
		t.Error("merging the drift overlay dropped the identity overlay beneath it")
	}
	if files, ok := agent["config_files"].([]any); !ok || len(files) != 1 {
		t.Errorf("the local configuration the Supervisor merges did not land: %v", agent["config_files"])
	}
}

func TestWithOverlayRefusesADocumentItDidNotWrite(t *testing.T) {
	// Merging into an arbitrary file would silently rewrite whatever it
	// was pointed at, and the header is the only marker that this tool
	// owns the file.
	if _, err := withOverlay([]byte("server:\n  endpoint: ws://x\n"), map[string]any{}); err == nil {
		t.Fatal("a file with no generated header was merged into anyway")
	}
}

func TestReadOverlayParsesAValidFragment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overlay.yaml")
	if err := os.WriteFile(path, []byte("agent:\n  executable: /otelcol\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readOverlay(path)
	if err != nil {
		t.Fatalf("readOverlay: %v", err)
	}
	agent, ok := got["agent"].(map[string]any)
	if !ok {
		t.Fatalf("agent key missing or wrong type: %T", got["agent"])
	}
	if agent["executable"] != "/otelcol" {
		t.Errorf("executable %q, want /otelcol", agent["executable"])
	}
}

func TestReadOverlayFailsWhenFileIsAbsent(t *testing.T) {
	if _, err := readOverlay("/nonexistent/overlay.yaml"); err == nil {
		t.Fatal("no error for a file that does not exist")
	}
}

func TestReadOverlayFailsOnInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte(":\tbad\t:yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readOverlay(path); err == nil {
		t.Fatal("no error for invalid YAML")
	}
}
