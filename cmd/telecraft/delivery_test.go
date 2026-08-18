package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// deliveryFixture writes an intended artefact and an effective config into
// a temp dir and returns their paths. The effective config is the same
// document with cosmetic differences only — reordered keys, flow style,
// quoting — which must never read as divergence.
func deliveryFixture(t *testing.T) (intended, effective string) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "intended.yaml", `receivers:
  otlp:
    protocols:
      grpc: {}
exporters:
  otlphttp/out:
    endpoint: https://gateway.internal:4318
service:
  pipelines:
    traces:
      receivers:
        - otlp
      exporters:
        - otlphttp/out
  telemetry:
    resource:
      telecraft.commit: a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2
`)
	writeFile(t, dir, "effective.yaml", `service:
  telemetry:
    resource:
      "telecraft.commit": "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
  pipelines:
    traces: {receivers: [otlp], exporters: [otlphttp/out]}
exporters:
  otlphttp/out: {endpoint: "https://gateway.internal:4318"}
receivers:
  otlp: {protocols: {grpc: {}}}
`)
	return dir + "/intended.yaml", dir + "/effective.yaml"
}

// AC: a hand-committed (GitOps) collector gets identical treatment and its
// delivery path is visible — the printed status names the path, reads the
// stamps from both sides, and cosmetic YAML differences never read as
// divergence. The absent RemoteConfigStatus reading prints known=false,
// never failure.
func TestDeliveryCommandPrintsAGitCollectorsStatus(t *testing.T) {
	intended, effective := deliveryFixture(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"delivery", "-intended", intended, "-effective", effective, "-path", "git"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{
		"path              git",
		"profile           exact",
		"remote            known=false",
		"intended_commit   a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		"effective_commit  a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2",
		"comparison        in_sync",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "FAILED") {
		t.Errorf("a can't-report reading printed like failure:\n%s", out)
	}
}

// A real edit under the same commit stamp prints drifted with the layer-3
// localisation — the printer stays exit 0: it is a reading, not a gate.
func TestDeliveryCommandLocalisesDrift(t *testing.T) {
	intended, effective := deliveryFixture(t)
	raw, err := os.ReadFile(effective)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(raw), "https://gateway.internal:4318", "https://somewhere-else:4318", 1)
	if err := os.WriteFile(effective, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	code := run([]string{"delivery", "-intended", intended, "-effective", effective, "-path", "git"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "comparison        drifted") {
		t.Errorf("output lacks the drifted comparison:\n%s", out)
	}
	if !strings.Contains(out, "exporters.otlphttp/out.endpoint") {
		t.Errorf("output lacks the layer-3 localisation:\n%s", out)
	}
}

func TestDeliveryCommandRequiresItsFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"delivery"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	intended, effective := deliveryFixture(t)
	if code := run([]string{"delivery", "-intended", intended, "-effective", effective, "-path", "sideloaded"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d for an invented delivery path, want 2", code)
	}
}
