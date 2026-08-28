package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// root is two directories up, which is where the Dockerfile, the ignore
// file and the command that owns the address flags all live.
func root() string { return filepath.Join("..", "..") }

// TestTheImageInTheTreeIsClean is the self-test: the check runs over this
// repository, so a Dockerfile edit that breaks the posture fails
// `go test ./...` on the author's own machine rather than in a build that
// needs a daemon.
func TestTheImageInTheTreeIsClean(t *testing.T) {
	findings, err := Run(root())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		t.Errorf("%s", f)
	}
}

// plant copies the repository's Dockerfile, ignore file and serve command
// into a fresh root, with one substitution applied to the Dockerfile. Every
// case below is a real edit somebody could make while meaning well.
func plant(t *testing.T, from, to string) string {
	t.Helper()
	dir := t.TempDir()

	body, err := os.ReadFile(filepath.Join(root(), "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := string(body)
	if from != "" {
		if !strings.Contains(dockerfile, from) {
			t.Fatalf("the Dockerfile no longer holds %q, so this case is checking nothing", from)
		}
		dockerfile = strings.Replace(dockerfile, from, to, 1)
	}
	write(t, filepath.Join(dir, "Dockerfile"), dockerfile)

	copyFile(t, filepath.Join(root(), ".dockerignore"), filepath.Join(dir, ".dockerignore"))
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "telecraft"), 0o755); err != nil {
		t.Fatal(err)
	}
	copyFile(t, filepath.Join(root(), "cmd", "telecraft", "serve.go"), filepath.Join(dir, "cmd", "telecraft", "serve.go"))
	return dir
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	body, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	write(t, to, string(body))
}

func TestAnImageThatDriftedIsReported(t *testing.T) {
	cases := []struct {
		name string
		from string
		to   string
		want string
	}{
		{
			name: "the base loses its digest",
			from: "@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab",
			to:   "",
			want: "not pinned by digest",
		},
		{
			name: "a build stage arrives",
			from: "ARG TARGETARCH",
			to:   "FROM golang:1.26 AS build\n\nARG TARGETARCH",
			want: "a second build stage",
		},
		{
			name: "something runs during the build",
			from: "EXPOSE 4321 4320",
			to:   "RUN chmod +x /usr/local/bin/telecraft\n\nEXPOSE 4321 4320",
			want: "executes during the build",
		},
		{
			name: "a file is copied from outside the staged context",
			from: "COPY dist/image/LICENSE",
			to:   "COPY LICENSE",
			want: "outside the staged context",
		},
		{
			name: "the licence stops travelling",
			from: "COPY dist/image/LICENSE /usr/share/telecraft/LICENSE",
			to:   "COPY dist/image/LICENSE /LICENSE",
			want: "/usr/share/telecraft/LICENSE",
		},
		{
			name: "one architecture takes the other's binary",
			from: "telecraft-linux-${TARGETARCH}",
			to:   "telecraft-linux-amd64",
			want: "holds the other's binary",
		},
		{
			name: "the process runs as root",
			from: "USER 65532:65532",
			to:   "USER root",
			want: "as root",
		},
		{
			name: "the entrypoint is written in shell form",
			from: `ENTRYPOINT ["/usr/local/bin/telecraft"]`,
			to:   "ENTRYPOINT /usr/local/bin/telecraft",
			want: "carries no shell",
		},
		{
			name: "the default command stops being serve",
			from: `CMD ["serve"]`,
			to:   `CMD ["check"]`,
			want: `the default command is ["serve"]`,
		},
		{
			name: "the console address keeps the loopback default",
			from: "ENV TELECRAFT_HTTP=0.0.0.0:4321 \\\n    TELECRAFT_LISTEN=0.0.0.0:4320",
			to:   "ENV TELECRAFT_LISTEN=0.0.0.0:4320",
			want: "TELECRAFT_HTTP is not set",
		},
		{
			name: "an address drifts from the flag it mirrors",
			from: "TELECRAFT_LISTEN=0.0.0.0:4320",
			to:   "TELECRAFT_LISTEN=0.0.0.0:4322",
			want: "-listen defaults to port 4320",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			findings, err := Run(plant(t, c.from, c.to))
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range findings {
				if strings.Contains(f.Message, c.want) {
					return
				}
			}
			t.Errorf("no finding said %q; got %v", c.want, findings)
		})
	}
}

// TestAWorkingTreeContextIsReported covers the other half of the assembly
// rule: the ignore file is what keeps a source tree, a node_modules and a
// .git out of every build.
func TestAWorkingTreeContextIsReported(t *testing.T) {
	dir := plant(t, "", "")
	write(t, filepath.Join(dir, ".dockerignore"), "node_modules\n")

	findings, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	var said bool
	for _, f := range findings {
		if strings.Contains(f.Message, "the context is the staged directory") {
			said = true
		}
	}
	if !said {
		t.Errorf("an ignore file that admits the working tree went unreported; got %v", findings)
	}
}
