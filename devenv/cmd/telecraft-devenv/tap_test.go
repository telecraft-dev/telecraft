package main

// reportedConfigs.Report and reportedConfigs.Closed (tap.go:55 and tap.go:85)
// are not tested here because they are driven entirely by live OpAMP
// connections. A collector must connect, send an EffectiveConfig message, and
// then disconnect for those two methods to be called. That requires the full
// Docker compose environment and is explicitly excluded by the issue (a
// "requires Docker" path is a valid reason to leave it uncovered).

import (
	"testing"

	"github.com/open-telemetry/opamp-go/protobufs"
)

// safeName is the only barrier between a collector-reported identity and the
// filesystem. These cases cover what it must allow, what it must replace,
// and the degenerate inputs that would otherwise produce an unusable name.

func TestSafeNamePreservesAlphanumericAndAllowedCharacters(t *testing.T) {
	cases := []string{
		"coll-a",
		"collector_1",
		"a.b.c",
		"ABC123",
	}
	for _, name := range cases {
		if got := safeName(name); got != name {
			t.Errorf("safeName(%q) = %q, want unchanged", name, got)
		}
	}
}

func TestSafeNameReplacesSpecialCharacters(t *testing.T) {
	// Slashes, spaces and other separators would escape the devenv directory.
	for input, want := range map[string]string{
		"team/collector": "team-collector",
		"has space":      "has-space",
		"star*name":      "star-name",
	} {
		if got := safeName(input); got != want {
			t.Errorf("safeName(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSafeNameReturnsCollectorForAllDotsOrBlankInput(t *testing.T) {
	// A name of only dots is indistinguishable from a relative path, and a
	// blank name has no useful content — both fall back to a fixed sentinel
	// rather than writing to "." or "".
	for _, input := range []string{"", ".", ".."} {
		if got := safeName(input); got != "collector" {
			t.Errorf("safeName(%q) = %q, want %q", input, got, "collector")
		}
	}
}

// taps fans one wire to several readers. Every reader must receive every
// call, so a reader that is not notified would silently miss all deliveries.

type recordingTap struct {
	reports int
	closes  int
}

func (r *recordingTap) Report(_ any, _ map[string]string, _ *protobufs.AgentToServer) {
	r.reports++
}

func (r *recordingTap) Closed(_ any) {
	r.closes++
}

func TestTapsRouteToAllReaders(t *testing.T) {
	a, b := &recordingTap{}, &recordingTap{}
	ts := taps{a, b}

	ts.Report(nil, nil, &protobufs.AgentToServer{})
	ts.Report(nil, nil, &protobufs.AgentToServer{})
	ts.Closed(nil)

	if a.reports != 2 || b.reports != 2 {
		t.Errorf("reports: a=%d b=%d, want 2 each", a.reports, b.reports)
	}
	if a.closes != 1 || b.closes != 1 {
		t.Errorf("closes: a=%d b=%d, want 1 each", a.closes, b.closes)
	}
}

// collectorConfig picks one body from a reported config map. The Supervisor
// uses the empty string as the key for its own configuration; anything else
// is a named file, and the lexically first is taken for stability.

func makeConfigMap(files map[string][]byte) *protobufs.EffectiveConfig {
	m := make(map[string]*protobufs.AgentConfigFile, len(files))
	for k, v := range files {
		m[k] = &protobufs.AgentConfigFile{Body: v}
	}
	return &protobufs.EffectiveConfig{
		ConfigMap: &protobufs.AgentConfigMap{ConfigMap: m},
	}
}

func TestCollectorConfigPicksEmptyKeyFirst(t *testing.T) {
	ec := makeConfigMap(map[string][]byte{
		"":      []byte("supervisor-body"),
		"other": []byte("other-body"),
	})
	body, ok := collectorConfig(ec)
	if !ok {
		t.Fatal("collectorConfig returned ok=false")
	}
	if string(body) != "supervisor-body" {
		t.Errorf("got %q, want supervisor-body", body)
	}
}

func TestCollectorConfigPicksLexicallyFirstWhenNoEmptyKey(t *testing.T) {
	ec := makeConfigMap(map[string][]byte{
		"z-config.yaml": []byte("z-body"),
		"a-config.yaml": []byte("a-body"),
	})
	body, ok := collectorConfig(ec)
	if !ok {
		t.Fatal("collectorConfig returned ok=false")
	}
	if string(body) != "a-body" {
		t.Errorf("got %q, want a-body (lexically first key)", body)
	}
}

func TestCollectorConfigRefusesEmptyMap(t *testing.T) {
	ec := makeConfigMap(map[string][]byte{})
	_, ok := collectorConfig(ec)
	if ok {
		t.Fatal("collectorConfig returned ok=true for an empty config map")
	}
}
