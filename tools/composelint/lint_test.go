package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// root is two directories up, which is where the deployment directory, the
// ignore file and the command that owns the address flags all live.
func root() string { return filepath.Join("..", "..") }

// TestTheDeploymentInTheTreeIsClean is the self-test: the check runs over
// this repository, so an edit to the compose file that breaks what the
// guide documents fails `go test ./...` rather than an adopter's first
// `docker compose up`.
func TestTheDeploymentInTheTreeIsClean(t *testing.T) {
	findings, err := Run(root())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		t.Errorf("%s", f)
	}
}

// Each case below is a real edit somebody could make while meaning well.
func TestTheRulesCatchWhatTheyAreFor(t *testing.T) {
	cases := []struct {
		name   string
		from   string
		to     string
		expect string
	}{
		{
			name:   "an estate fetched with git the image does not carry",
			from:   "      - -estate=/estate",
			to:     "      - -repo=https://forge.example/acme/estate.git",
			expect: "-repo fetches an estate with git",
		},
		{
			name:   "the estate mounted writable",
			from:   "      - ${ESTATE_DIR}:/estate:ro",
			to:     "      - ${ESTATE_DIR}:/estate",
			expect: "mounts /estate writable",
		},
		{
			name:   "a mount of a file that is not there",
			from:   "      - ./proxy/telecraft.conf:/etc/nginx/conf.d/default.conf:ro",
			to:     "      - ./proxy/nginx.conf:/etc/nginx/conf.d/default.conf:ro",
			expect: "there is no such file beside this one",
		},
		{
			name:   "the session key dropped from the service",
			from:   "    secrets:\n      - session-key\n",
			to:     "    secrets: []\n",
			expect: "so a restart signs everybody out",
		},
		{
			name:   "an image reference a mirror cannot replace with one value",
			from:   "    image: ${TELECRAFT_IMAGE}",
			to:     "    image: ghcr.io/telecraft-dev/telecraft:release",
			expect: "name the whole reference in one variable",
		},
		{
			name:   "a terminator that starts without being asked for",
			from:   "    profiles: [tls]\n",
			to:     "",
			expect: "put it behind a profile",
		},
		{
			name:   "a published port nothing listens on",
			from:   `      - "${CONSOLE_ADDRESS}:4321"`,
			to:     `      - "${CONSOLE_ADDRESS}:8080"`,
			expect: "nothing publishes port 4321",
		},
		{
			name:   "a secret carried as a value",
			from:   "      TELECRAFT_FETCH_INTERVAL: ${TELECRAFT_FETCH_INTERVAL}",
			to:     "      TELECRAFT_TELEMETRY_API_KEY: ${TELEMETRY_API_KEY}",
			expect: "reads as secret material carried as a value",
		},
		{
			name:   "a secret directory the secrets block does not reach",
			from:   "      - -secrets-dir=/run/secrets",
			to:     "      - -secrets-dir=/etc/telecraft/secrets",
			expect: "compose presents its secrets at /run/secrets",
		},
		{
			name:   "a secret read from the environment",
			from:   "  session-key:\n    file: ${SECRETS_DIR}/session-key",
			to:     "  session-key:\n    environment: SESSION_KEY",
			expect: "a secret is a file the deployment places",
		},
		{
			name:   "a variable the example file never names",
			from:   "${ESTATE_DIR}:/estate:ro",
			to:     "${ESTATE_PATH}:/estate:ro",
			expect: "does not name it, so a copied .env leaves it empty",
		},
		{
			name:   "a variable the example file names and nothing reads",
			from:   "${ESTATE_DIR}:/estate:ro",
			to:     "${ESTATE_PATH}:/estate:ro",
			expect: "ESTATE_DIR is named here and compose.yaml reads nothing of that name",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			findings, err := Run(plant(t, c.from, c.to))
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range findings {
				if strings.Contains(f.Message, c.expect) {
					return
				}
			}
			t.Errorf("no finding said %q; got %v", c.expect, findings)
		})
	}
}

// TestTheImageTheExampleNamesIsPinned covers the half of the image rule that
// lives in the environment file: the reference an operator gets by copying
// .env.example names one build rather than whichever ran last.
func TestTheImageTheExampleNamesIsPinned(t *testing.T) {
	for _, c := range []struct {
		name      string
		reference string
		expect    string
	}{
		{"no tag at all", "ghcr.io/telecraft-dev/telecraft", "names no tag and no digest"},
		{"the latest tag", "ghcr.io/telecraft-dev/telecraft:latest", "runs the latest tag"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := plant(t, "", "")
			path := filepath.Join(dir, deployDir, envName)
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			const pinned = "TELECRAFT_IMAGE=ghcr.io/telecraft-dev/telecraft:release"
			if !strings.Contains(string(body), pinned) {
				t.Fatalf("the example no longer holds %q, so this case is checking nothing", pinned)
			}
			write(t, path, strings.Replace(string(body), pinned, "TELECRAFT_IMAGE="+c.reference, 1))

			findings, err := Run(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, f := range findings {
				if strings.Contains(f.Message, c.expect) {
					return
				}
			}
			t.Errorf("no finding said %q; got %v", c.expect, findings)
		})
	}
}

// TestAFilledInEnvHasToBeIgnored covers the one rule that is not about the
// compose file itself: the copy an operator fills in holds the paths and
// the mirror of one host and belongs to that host.
func TestAFilledInEnvHasToBeIgnored(t *testing.T) {
	dir := plant(t, "", "")
	write(t, filepath.Join(dir, ".gitignore"), "/dist/\n")

	findings, err := Run(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if strings.Contains(f.Message, "one `git add -A` from the repository") {
			return
		}
	}
	t.Errorf("a tracked .env went unreported; got %v", findings)
}

// plant copies the deployment directory, the ignore file and the serve
// command into a fresh root, with one substitution applied to the compose
// file.
func plant(t *testing.T, from, to string) string {
	t.Helper()
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, deployDir, "proxy"), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root(), deployDir, composeName))
	if err != nil {
		t.Fatal(err)
	}
	compose := string(body)
	if from != "" {
		if !strings.Contains(compose, from) {
			t.Fatalf("the compose file no longer holds %q, so this case is checking nothing", from)
		}
		compose = strings.Replace(compose, from, to, 1)
	}
	write(t, filepath.Join(dir, deployDir, composeName), compose)

	copyFile(t, filepath.Join(root(), deployDir, envName), filepath.Join(dir, deployDir, envName))
	copyFile(t, filepath.Join(root(), deployDir, "proxy", "telecraft.conf"), filepath.Join(dir, deployDir, "proxy", "telecraft.conf"))
	copyFile(t, filepath.Join(root(), ".gitignore"), filepath.Join(dir, ".gitignore"))

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
