package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/telecraft-dev/telecraft/internal/catalogue"
)

// paletteFixture writes an estate directory (teams, allow-lists, a grant)
// and a Catalogue artefact, returning both paths.
func paletteFixture(t *testing.T) (estate, artefact string) {
	t.Helper()
	estate = t.TempDir()

	writeFile(t, estate, "teams.yaml", `
teams:
  - id: org
    name: Org
    owners: [org-lead]
    teams:
      - id: payments
        name: Payments
        owners: [payments-lead]
`)
	writeFile(t, estate, "allow-lists.yaml", `
allow_lists:
  - team: org
    owner: org-lead
    allow:
      - receiver/*
      - processor/batch
`)
	writeFile(t, estate, "grants.yaml", `
grants:
  - id: kafka-for-payments
    owner: org-lead
    team: payments
    adds:
      - exporter/kafka
`)

	comp := func(class catalogue.Class, typ string) catalogue.Component {
		return catalogue.Component{
			Class:     class,
			Type:      typ,
			Module:    "example.com/otelcol/" + string(class) + "/" + typ,
			Stability: map[string]catalogue.Level{"traces": catalogue.Beta},
		}
	}
	cat := &catalogue.Catalogue{
		FormatVersion: catalogue.FormatVersion,
		Source:        catalogue.Source{Repository: "example.com/otelcol", Ref: "v0.158.0"},
		Components: []catalogue.Component{
			comp(catalogue.Receiver, "otlp"),
			comp(catalogue.Processor, "batch"),
			comp(catalogue.Processor, "attributes"),
			comp(catalogue.Exporter, "kafka"),
		},
	}
	artefact, _, err := cat.Write(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return estate, artefact
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// AC: a CLI prints any team's effective palette — components, provenance,
// the Grant named with who granted it to whom.
func TestPaletteCommandPrintsAnEffectivePalette(t *testing.T) {
	estate, artefact := paletteFixture(t)
	var stdout, stderr bytes.Buffer

	code := run([]string{"palette", "-team", "payments", "-estate", estate, "-catalogue", artefact}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr.String())
	}
	out := stdout.String()

	for _, want := range []string{
		"team       payments",
		"catalogue  v0.158.0",
		"allowed    3 of 4 components",
		"receiver/otlp",
		"processor/batch",
		"grant kafka-for-payments (granted by org to payments)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "processor/attributes") {
		t.Errorf("output holds a component outside the effective list:\n%s", out)
	}
}

func TestPaletteCommandFailsClosedOnBadPolicy(t *testing.T) {
	estate, artefact := paletteFixture(t)
	writeFile(t, estate, "allow-lists.yaml", `
allow_lists:
  - team: org
    owner: org-lead
    allow: [receiver/nosuch]
`)
	var stdout, stderr bytes.Buffer
	code := run([]string{"palette", "-team", "payments", "-estate", estate, "-catalogue", artefact}, &stdout, &stderr)
	if code == 0 {
		t.Fatal("an invalid policy printed a palette")
	}
	if !strings.Contains(stderr.String(), "selects nothing") {
		t.Errorf("stderr does not carry the validation error:\n%s", stderr.String())
	}
}

func TestPaletteCommandRequiresItsFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"palette"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
}

func TestRunWithNoArgsPrintsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("stderr does not contain usage line:\n%s", stderr.String())
	}
}

func TestRunRefusesUnknownSubcommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"nosuchcommand"}, &stdout, &stderr); code != 2 {
		t.Fatalf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "usage:") {
		t.Errorf("stderr does not contain usage line:\n%s", stderr.String())
	}
}

func TestEnvOrReturnsEnvVarWhenSet(t *testing.T) {
	const key = "TELECRAFT_TEST_ENVVAR_SENTINEL"
	t.Setenv(key, "from-env")
	if got := envOr(key, "fallback"); got != "from-env" {
		t.Errorf("got %q, want %q", got, "from-env")
	}
}

// The passwd branch of run hardcodes os.Stdin, so it cannot be exercised
// through the run entrypoint without touching the process's real stdin.
// passwd's own behaviour is fully covered by the tests in passwd_test.go,
// which call runPasswd directly with a controlled reader.
