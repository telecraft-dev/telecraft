package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-telemetry/opamp-go/protobufs"
)

// The wire readers. Everything below runs over messages built here rather
// than over a live collector: what a tap does with a message is the devenv's
// own logic, and the serving package owns getting the message to it.

// effectiveConfig builds one reported effective configuration, keyed by
// file name. The Supervisor reports a single entry keyed by the empty
// string; anything else is keyed by name.
func effectiveConfig(files map[string]string) *protobufs.AgentToServer {
	config := &protobufs.AgentConfigMap{ConfigMap: map[string]*protobufs.AgentConfigFile{}}
	for name, body := range files {
		config.ConfigMap[name] = &protobufs.AgentConfigFile{Body: []byte(body)}
	}
	return &protobufs.AgentToServer{EffectiveConfig: &protobufs.EffectiveConfig{ConfigMap: config}}
}

func TestReportedConfigsWritesEachCollectorsConfigVerbatim(t *testing.T) {
	dir := t.TempDir()
	r := &reportedConfigs{dir: dir}
	body := "receivers:\n  otlp:\n    protocols:\n      grpc: {}\n"

	r.Report("conn-1", map[string]string{"service.instance.id": "gateway-1"}, effectiveConfig(map[string]string{"": body}))

	// `telecraft delivery` reads these documents through the normaliser, so
	// anything this tap rewrote would read as drift the collector never
	// had.
	written, err := os.ReadFile(filepath.Join(dir, "gateway-1.yaml"))
	if err != nil {
		t.Fatalf("no reported config was filed: %v", err)
	}
	if string(written) != body {
		t.Errorf("the reported config did not survive verbatim:\n%s", written)
	}
}

// The first message on a connection can arrive without the agent
// description. There is nothing to file it under yet, and the server has
// already asked for full state, so the next message carries the identity.
func TestReportedConfigsWaitsForAnIdentityThenRemembersIt(t *testing.T) {
	dir := t.TempDir()
	r := &reportedConfigs{dir: dir}
	body := "service: {}\n"

	r.Report("conn-1", nil, effectiveConfig(map[string]string{"": body}))
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) > 0 {
		t.Errorf("a config with no collector to file it under was written anyway: %v", entries)
	}

	r.Report("conn-1", map[string]string{"service.instance.id": "gateway-1"}, effectiveConfig(map[string]string{"": body}))
	// The identity is remembered per connection, so a later message that
	// carries none is still filed under the right collector.
	r.Report("conn-1", nil, effectiveConfig(map[string]string{"": "service: {a: b}\n"}))

	written, err := os.ReadFile(filepath.Join(dir, "gateway-1.yaml"))
	if err != nil {
		t.Fatalf("no reported config was filed: %v", err)
	}
	if !strings.Contains(string(written), "a: b") {
		t.Errorf("the later message was not filed under the remembered identity:\n%s", written)
	}
}

// A message carrying no effective configuration files nothing: the tap is
// a cache of what collectors reported, not of what they did not.
func TestReportedConfigsIgnoresAMessageWithNoConfig(t *testing.T) {
	dir := t.TempDir()
	r := &reportedConfigs{dir: dir}
	identity := map[string]string{"service.instance.id": "gateway-1"}

	r.Report("conn-1", identity, &protobufs.AgentToServer{})
	r.Report("conn-1", identity, effectiveConfig(nil))

	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		t.Errorf("a message carrying no configuration wrote a file anyway: %v", entries)
	}
}

// The remembered identity dies with the connection, so a reconnecting
// collector's first message waits for a fresh one rather than being filed
// under the last one this connection wore.
func TestReportedConfigsForgetsAClosedConnection(t *testing.T) {
	dir := t.TempDir()
	r := &reportedConfigs{dir: dir}
	r.Report("conn-1", map[string]string{"service.instance.id": "gateway-1"}, effectiveConfig(map[string]string{"": "service: {}\n"}))

	r.Closed("conn-1")
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	r.Report("conn-1", nil, effectiveConfig(map[string]string{"": "service: {b: c}\n"}))

	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		t.Errorf("a closed connection's identity outlived it: %v", entries)
	}
}

// The Supervisor reports one entry keyed by the empty string; a collector
// reporting by file name has its entries taken in lexical order, so the
// choice is at least stable between refreshes.
func TestCollectorConfigPicksTheCollectorsOwnDocument(t *testing.T) {
	for name, tc := range map[string]struct {
		files map[string]string
		want  string
		ok    bool
	}{
		"nothing reported": {files: nil},
		"the Supervisor's one entry": {
			files: map[string]string{"": "service: {}\n"},
			want:  "service: {}\n",
			ok:    true,
		},
		"entries keyed by file name": {
			files: map[string]string{"zz-override.yaml": "service: {z: z}\n", "aa-base.yaml": "service: {a: a}\n"},
			want:  "service: {a: a}\n",
			ok:    true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, ok := collectorConfig(effectiveConfig(tc.files).GetEffectiveConfig())
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if string(got) != tc.want {
				t.Errorf("body = %q, want %q", got, tc.want)
			}
		})
	}
}

// Identities come off the wire, so a collector could report one with a
// path separator in it. A devenv is not the place to discover that by
// writing outside its own directory.
func TestSafeNameKeepsAnIdentityInsideItsOwnDirectory(t *testing.T) {
	for name, tc := range map[string]struct{ id, want string }{
		"an ordinary id":      {id: "gateway-1", want: "gateway-1"},
		"a dotted id":         {id: "host.example.internal", want: "host.example.internal"},
		"a path separator":    {id: "../../etc/passwd", want: "..-..-etc-passwd"},
		"a Windows separator": {id: `a\b`, want: "a-b"},
		"nothing at all":      {id: "", want: "collector"},
		"only dots":           {id: "..", want: "collector"},
		"only separators":     {id: "///", want: "---"},
	} {
		t.Run(name, func(t *testing.T) {
			got := safeName(tc.id)
			if got != tc.want {
				t.Errorf("safeName(%q) = %q, want %q", tc.id, got, tc.want)
			}
			if strings.ContainsAny(got, `/\`) {
				t.Errorf("safeName(%q) = %q, which still names a path", tc.id, got)
			}
		})
	}
}

// One wire, several readers: the server takes a single tap, so fanning out
// is the devenv's concern and every reader has to see every message.
func TestTapsFanOneWireOutToEveryReader(t *testing.T) {
	dir, other := t.TempDir(), t.TempDir()
	first, second := &reportedConfigs{dir: dir}, &reportedConfigs{dir: other}
	fan := taps{first, second}
	identity := map[string]string{"service.instance.id": "gateway-1"}

	fan.Report("conn-1", identity, effectiveConfig(map[string]string{"": "service: {}\n"}))

	for _, d := range []string{dir, other} {
		if _, err := os.Stat(filepath.Join(d, "gateway-1.yaml")); err != nil {
			t.Errorf("one reader did not see the message: %v", err)
		}
	}

	fan.Closed("conn-1")
	for _, r := range []*reportedConfigs{first, second} {
		if _, remembered := r.named["conn-1"]; remembered {
			t.Error("a reader did not see the connection close")
		}
	}
}
