package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheChartInThisRepositoryIsClean is the check that makes the rest of
// this file worth having: the next change to the chart, or to the flag set
// under it, fails `go test ./...` on the author's own machine rather than
// on a runner, or in an adopter's cluster.
func TestTheChartInThisRepositoryIsClean(t *testing.T) {
	result, err := Run("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range result.Findings {
		t.Errorf("%s", f)
	}
	if result.Files == 0 {
		t.Fatal("the check read no chart files")
	}
}

// TestDriftIsCaught seeds each failure the check exists for into a copy of
// the real chart. A lint whose findings are never exercised reports clean
// because it looks at nothing.
func TestDriftIsCaught(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(t *testing.T, root string)
		wants string
	}{
		{
			name: "a flag the command does not define",
			edit: func(t *testing.T, root string) {
				replace(t, root, "charts/telecraft/templates/deployment.yaml",
					"            - -fetch-interval", "            - -poll-interval")
			},
			wants: "which `telecraft serve` does not define",
		},
		{
			name: "the listen port moved in the command",
			edit: func(t *testing.T, root string) {
				replace(t, root, "cmd/telecraft/serve.go", `"listen", "127.0.0.1:4320"`, `"listen", "127.0.0.1:4330"`)
			},
			wants: `"telecraft.opampPort" is 4320 and the -listen flag defaults to port 4330`,
		},
		{
			name: "the server stopped serving a probe",
			edit: func(t *testing.T, root string) {
				replace(t, root, "internal/instance/api.go", `mux.HandleFunc("GET /readyz"`, `mux.HandleFunc("GET /ready"`)
			},
			wants: "the chart probes /readyz, which the server does not serve",
		},
		{
			name: "a second replica",
			edit: func(t *testing.T, root string) {
				replace(t, root, "charts/telecraft/values.yaml", "replicaCount: 1", "replicaCount: 2")
			},
			wants: "replicaCount is 1, and it is 2",
		},
		{
			name: "nothing refuses a second replica",
			edit: func(t *testing.T, root string) {
				replace(t, root, "charts/telecraft/templates/deployment.yaml",
					`{{- if ne (int .Values.replicaCount) 1 }}`, `{{- if false }}`)
				replace(t, root, "charts/telecraft/templates/deployment.yaml",
					`{{- fail "replicaCount is 1.`, `{{- print "replicaCount is 1.`)
			},
			wants: "nothing refuses a second replica",
		},
		{
			name: "a rolling update",
			edit: func(t *testing.T, root string) {
				replace(t, root, "charts/telecraft/templates/deployment.yaml", "type: Recreate", "type: RollingUpdate")
			},
			wants: "the update strategy is Recreate",
		},
		{
			name: "the image comes from somewhere else",
			edit: func(t *testing.T, root string) {
				replace(t, root, "charts/telecraft/values.yaml", "  repository: "+Registry, "  repository: docker.io/telecraft/telecraft")
			},
			wants: "image.repository is " + Registry,
		},
		{
			name: "a chart dependency to resolve at install time",
			edit: func(t *testing.T, root string) {
				appendTo(t, root, "charts/telecraft/Chart.yaml", "\ndependencies:\n  - name: postgresql\n    version: 1.0.0\n    repository: https://charts.example\n")
			},
			wants: "An install resolves no second chart repository",
		},
		{
			name: "a DaemonSet arrives by the back door",
			edit: func(t *testing.T, root string) {
				write(t, root, "charts/telecraft/templates/collector.yaml", "apiVersion: apps/v1\nkind: DaemonSet\nmetadata:\n  name: collector\n")
			},
			wants: "the chart deploys a DaemonSet",
		},
		{
			name: "a values file carrying a secret",
			edit: func(t *testing.T, root string) {
				replace(t, root, "charts/telecraft/values.yaml",
					"  telemetry:\n    endpoint: \"\"",
					"  telemetry:\n    endpoint: \"\"\n    apiKey: \"a-real-one\"")
			},
			wants: "server.telemetry.apiKey holds a value",
		},
		{
			name: "a default image tag",
			edit: func(t *testing.T, root string) {
				replace(t, root, "charts/telecraft/values.yaml", "  repository: "+Registry+"\n  tag: \"\"",
					"  repository: "+Registry+"\n  tag: \"v0.7.0\"")
			},
			wants: "image.tag is empty by default",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := copyTree(t)
			c.edit(t, root)
			result, err := Run(root)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, f := range result.Findings {
				got = append(got, f.String())
			}
			for _, f := range got {
				if strings.Contains(f, c.wants) {
					return
				}
			}
			t.Errorf("no finding contains %q; findings were:\n%s", c.wants, strings.Join(got, "\n"))
		})
	}
}

// TestASecretShapedKeyThatHoldsANameIsNotAFinding pins the distinction the
// values file rests on: naming a Secret is what the chart is for, and
// carrying one is what it refuses.
func TestASecretShapedKeyThatHoldsANameIsNotAFinding(t *testing.T) {
	values := map[string]any{
		"server": map[string]any{
			"secrets": map[string]any{"secretName": "telecraft-secrets"},
		},
		"estate": map[string]any{
			"sync": map[string]any{"credentialSecret": "estate-credential"},
		},
	}
	if found := secretShapedValues(values, nil); len(found) != 0 {
		t.Errorf("names read as values: %v", found)
	}

	values["server"].(map[string]any)["token"] = "ghp_notarealone"
	found := secretShapedValues(values, nil)
	if len(found) != 1 || found[0] != "server.token" {
		t.Errorf("secretShapedValues = %v, want [server.token]", found)
	}
}

// copyTree gives each case its own copy of the three trees the check reads,
// so a seeded edit never reaches the working tree.
func copyTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"charts", "cmd/telecraft", "internal/instance"} {
		if err := os.CopyFS(filepath.Join(root, filepath.FromSlash(dir)), os.DirFS(filepath.Join("../..", filepath.FromSlash(dir)))); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func replace(t *testing.T, root, path, old, new string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	body, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), old) {
		t.Fatalf("%s does not contain %q, so the case seeds nothing", path, old)
	}
	if err := os.WriteFile(full, []byte(strings.Replace(string(body), old, new, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendTo(t *testing.T, root, path, text string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	body, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, append(body, text...), 0o644); err != nil {
		t.Fatal(err)
	}
}

func write(t *testing.T, root, path, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
