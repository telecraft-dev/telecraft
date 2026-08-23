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
	estate, artefact := paletteFixture(t)
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"nothing at all": {
			args: []string{"palette"},
			want: "palette: -team, -estate and -catalogue are required",
		},
		"a team but no estate": {
			args: []string{"palette", "-team", "payments", "-catalogue", artefact},
			want: "palette: -team, -estate and -catalogue are required",
		},
		"a flag that does not exist": {
			args: []string{"palette", "-teams", "payments"},
			want: "flag provided but not defined: -teams",
		},
		"an estate with no teams.yaml": {
			args: []string{"palette", "-team", "payments", "-estate", t.TempDir(), "-catalogue", artefact},
			want: "palette: ",
		},
		"a Catalogue artefact that is not there": {
			args: []string{"palette", "-team", "payments", "-estate", estate, "-catalogue", filepath.Join(t.TempDir(), "catalogue-v0.0.0.json")},
			want: "palette: ",
		},
		// A team nobody declared has no palette to print, and inventing an
		// empty one would read as a team allowed nothing.
		"a team the tree does not know": {
			args: []string{"palette", "-team", "nosuch", "-estate", estate, "-catalogue", artefact},
			want: "palette: ",
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
				t.Errorf("a refused invocation printed a palette:\n%s", stdout.String())
			}
		})
	}
}

// An invocation with no subcommand, or one nobody implements, prints the
// usage banner rather than guessing. The banner names every subcommand,
// because it is the only list of them a user is shown.
func TestUsageNamesEverySubcommand(t *testing.T) {
	for name, args := range map[string][]string{
		"no subcommand":      nil,
		"unknown subcommand": {"conform"},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != 2 {
				t.Fatalf("exit %d, want 2", code)
			}
			for _, sub := range []string{"observe", "check", "palette", "render", "serve", "snapshot", "delivery", "passwd"} {
				if !strings.Contains(stderr.String(), "telecraft "+sub) {
					t.Errorf("the usage banner does not name %s:\n%s", sub, stderr.String())
				}
			}
			if stdout.Len() != 0 {
				t.Errorf("usage went to stdout:\n%s", stdout.String())
			}
		})
	}
}

// Deliberately uncovered: main itself. It is os.Exit around run, and a
// test that called it would take the test binary down with it.
