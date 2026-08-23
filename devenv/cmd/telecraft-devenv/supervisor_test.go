package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The composition of a Supervisor configuration: the renderer's artefact
// plus the identity the operator supplies. Every assertion below is about
// the two halves staying distinguishable: the base is reproducible from
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
		t.Error("the base was mutated: composing one collector would leak into the next")
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

// prepare over the repository's own devenv directories. Nothing below
// needs Docker: prepare reads authored files and the rendered tree and
// writes beside them, which is the whole of it.

// devenvPath names one of the devenv's own directories from inside this
// package.
func devenvPath(parts ...string) string {
	return filepath.Join(append([]string{"..", ".."}, parts...)...)
}

func TestPrepareWritesOneDirectoryPerCollector(t *testing.T) {
	out := t.TempDir()
	var stdout, stderr bytes.Buffer

	code := run([]string{"prepare",
		"-estate", devenvPath("estate"),
		"-identity", devenvPath("identity"),
		"-foreign", devenvPath("foreign"),
		"-out", out,
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr.String())
	}
	// A served collector gets a composed Supervisor configuration; a
	// git-delivered one gets the operator's file and no Supervisor
	// configuration at all, because nothing serves it.
	for _, name := range []string{"gateway-1", "gateway-2", "edge-1", "unmatched-1"} {
		path := filepath.Join(out, name, "supervisor.yaml")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("no composed configuration for %s: %v", name, err)
		}
		if !strings.Contains(stdout.String(), "wrote "+path) {
			t.Errorf("stdout does not name %s:\n%s", path, stdout.String())
		}
	}
	local := filepath.Join(out, "appliance-1", localFileName)
	if _, err := os.Stat(local); err != nil {
		t.Errorf("no local file for the git-delivered collector: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "appliance-1", "supervisor.yaml")); !os.IsNotExist(err) {
		t.Error("a git-delivered collector was given a Supervisor configuration: nothing serves it")
	}
}

// -drift puts exactly one collector out of step, by merging the local file
// the Supervisor loads over the served artefact. Every other collector is
// composed as it would be without the flag.
func TestPrepareDriftsOnlyTheCollectorItNames(t *testing.T) {
	out := t.TempDir()
	var stdout, stderr bytes.Buffer

	code := run([]string{"prepare",
		"-estate", devenvPath("estate"),
		"-identity", devenvPath("identity"),
		"-foreign", devenvPath("foreign"),
		"-out", out,
		"-drift", "gateway-1",
		"-drift-overlay", devenvPath("drift", "overlay.yaml"),
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr.String())
	}
	drifted, err := os.ReadFile(filepath.Join(out, "gateway-1", "supervisor.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	steady, err := os.ReadFile(filepath.Join(out, "gateway-2", "supervisor.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(drifted, steady) {
		t.Fatal("the drift overlay changed nothing")
	}
	var got map[string]any
	if err := yaml.Unmarshal(drifted, &got); err != nil {
		t.Fatalf("the drifted configuration does not parse: %v", err)
	}
	if _, ok := got["agent"].(map[string]any)["config_files"]; !ok {
		t.Error("the local configuration the Supervisor merges did not land")
	}
	if strings.Contains(string(steady), "config_files") {
		t.Error("a collector prepare was not told to drift was drifted anyway")
	}
}

// The reported configs are a cache of the wire, so prepare clears them:
// pointing `telecraft delivery` at a file describing a collector that is no
// longer running would compare against something nobody is serving.
func TestPrepareClearsTheReportedConfigs(t *testing.T) {
	out := t.TempDir()
	stale := filepath.Join(out, "effective")
	if err := os.MkdirAll(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "gone-1.yaml"), []byte("service: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	code := run([]string{"prepare",
		"-estate", devenvPath("estate"),
		"-identity", devenvPath("identity"),
		"-foreign", devenvPath("foreign"),
		"-out", out,
	}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr.String())
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("the last run's reported configs survived a prepare")
	}
}

// Everything prepare reads fails closed with the cause on stderr, at exit
// 1: a half-prepared environment starts collectors against configurations
// nobody composed.
func TestPrepareFailsClosedOnEachInputItReads(t *testing.T) {
	empty := t.TempDir()
	noBaseTier := t.TempDir()
	if err := os.WriteFile(filepath.Join(noBaseTier, "gateway-1.yaml"), []byte("overlay: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, []byte("in the way\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"an identity directory that is not there": {
			args: []string{"-identity", filepath.Join(empty, "nowhere")},
			want: "prepare: ",
		},
		// A devenv with no collectors starts nothing, and saying so beats
		// composing an environment with nothing in it.
		"an identity directory with no collectors in it": {
			args: []string{"-identity", empty},
			want: "the devenv has no collectors to compose",
		},
		"a foreign directory that is not there": {
			args: []string{"-foreign", filepath.Join(empty, "nowhere")},
			want: "prepare: ",
		},
		"an identity file naming no base": {
			args: []string{"-identity", noBaseTier},
			want: "base_tier",
		},
		"a drift overlay that is not there": {
			args: []string{"-drift", "gateway-1", "-drift-overlay", filepath.Join(empty, "nowhere.yaml")},
			want: "prepare: ",
		},
		"an out directory it cannot create": {
			args: []string{"-out", filepath.Join(blocked, "run")},
			want: "prepare: ",
		},
	} {
		t.Run(name, func(t *testing.T) {
			args := []string{"prepare",
				"-estate", devenvPath("estate"),
				"-identity", devenvPath("identity"),
				"-foreign", devenvPath("foreign"),
				"-out", t.TempDir(),
			}
			var stdout, stderr bytes.Buffer
			if code := run(append(args, tc.args...), &stdout, &stderr); code != 1 {
				t.Fatalf("exit %d, want 1\nstderr:\n%s", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("stderr lacks %q:\n%s", tc.want, stderr.String())
			}
		})
	}
}

func TestPrepareRejectsAFlagThatDoesNotExist(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := run([]string{"prepare", "-identities", "somewhere"}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "flag provided but not defined: -identities") {
		t.Errorf("stderr does not name the flag that does not exist:\n%s", stderr.String())
	}
}

// A drift overlay that is not a configuration fragment is named back with
// the file it was read from.
func TestReadOverlayNamesTheFileItCannotParse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overlay.yaml")
	if err := os.WriteFile(path, []byte("agent: [unclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := readOverlay(path)

	if err == nil {
		t.Fatal("an overlay that does not parse was read anyway")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the error does not name the file: %v", err)
	}
}

// A file that is nothing but comment lines has no document to merge into,
// which is the other half of the header check.
func TestWithOverlayRefusesADocumentThatIsAllHeader(t *testing.T) {
	if _, err := withOverlay([]byte("# Composed by telecraft-devenv"), map[string]any{}); err == nil {
		t.Fatal("a file with no document below its header was merged into anyway")
	}
}

// The header is only the marker; the document below it still has to parse
// before anything is merged into it.
func TestWithOverlayRefusesABodyThatDoesNotParse(t *testing.T) {
	composed := []byte("# Composed by telecraft-devenv\nagent: [unclosed\n")

	if _, err := withOverlay(composed, map[string]any{}); err == nil {
		t.Fatal("a document that does not parse was merged into anyway")
	}
}

// The base is the renderer's own artefact, so a base that does not parse
// is named by path: the identity file is not what is wrong with it.
func TestComposeNamesTheBaseArtefactItCannotParse(t *testing.T) {
	root := estateFixture(t, "platform", "gateway", "server: [unclosed\n")
	identity := identityFixture(t, "base_tier: platform/gateway\noverlay: {}\n")

	_, err := compose(root, identity)

	if err == nil {
		t.Fatal("a base artefact that does not parse composed anyway")
	}
	if !strings.Contains(err.Error(), "gateway.supervisor.yaml") {
		t.Errorf("the error does not name the artefact: %v", err)
	}
}

// Deliberately uncovered in prepare: the branch that fails when the last
// run's reported configs cannot be cleared, and the one that fails when a
// git-delivered collector's local file cannot be written. Both need a
// directory the process may read but not write, which a test running as
// the owner of everything it created cannot arrange portably.
