package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Finding names one place the image drifted from the posture ADR-0068 §2
// decided.
type Finding struct {
	Path    string
	Line    int
	Message string
}

func (f Finding) String() string {
	if f.Line == 0 {
		return fmt.Sprintf("%s: %s", f.Path, f.Message)
	}
	return fmt.Sprintf("%s:%d: %s", f.Path, f.Line, f.Message)
}

// documented paths inside the image. They are documented in the sense that
// the install guide prints them, so moving one is a change to a published
// page and not only to a COPY line.
const (
	binaryPath    = "/usr/local/bin/telecraft"
	licencePath   = "/usr/share/telecraft/LICENSE"
	cataloguePath = "/usr/share/telecraft/catalogues/"
)

// stagedPrefix is where the assembled image takes everything from. A COPY
// reaching outside it is a source tree travelling into the context.
const stagedPrefix = "dist/image/"

var digestPin = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)

// Run checks the Dockerfile at the root of the repository against what the
// image is decided to be, and against the flag defaults it mirrors.
func Run(root string) ([]Finding, error) {
	path := filepath.Join(root, "Dockerfile")
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	instructions, err := parse(string(body))
	if err != nil {
		return nil, err
	}

	var findings []Finding
	add := func(line int, format string, args ...any) {
		findings = append(findings, Finding{Path: "Dockerfile", Line: line, Message: fmt.Sprintf(format, args...)})
	}

	// One stage, because the image is assembled rather than compiled: the
	// binary inside it is the binary the release attached, not a second
	// build of the same commit.
	var stages []instruction
	for _, in := range instructions {
		if in.name == "FROM" {
			stages = append(stages, in)
		}
	}
	switch len(stages) {
	case 0:
		add(0, "no base image, so nothing is assembled")
	case 1:
		if !digestPin.MatchString(stages[0].args) {
			add(stages[0].line, "the base is not pinned by digest, so the image stops being a function of the tag: name it as image:tag@sha256:...")
		}
	default:
		add(stages[1].line, "a second build stage, so the image compiles what it holds; the binaries are staged into %s and copied in", stagedPrefix)
	}

	// Nothing runs and nothing is fetched while the image builds, so the
	// only network a build needs is the pull of the base layers.
	for _, in := range instructions {
		switch in.name {
		case "RUN", "SHELL", "ONBUILD":
			add(in.line, "%s executes during the build; the image is assembled from files that are already built", in.name)
		case "ADD":
			add(in.line, "ADD can fetch a URL during the build; copy a staged file with COPY instead")
		}
	}

	// Every source is staged, and the three documented destinations are
	// where the install guide says they are.
	var copied []string
	for _, in := range instructions {
		if in.name != "COPY" {
			continue
		}
		fields := strings.Fields(in.args)
		for _, f := range fields {
			if strings.HasPrefix(f, "--from=") {
				add(in.line, "COPY --from reads another stage, and there is only one")
			}
		}
		var plain []string
		for _, f := range fields {
			if !strings.HasPrefix(f, "--") {
				plain = append(plain, f)
			}
		}
		if len(plain) < 2 {
			add(in.line, "COPY needs a source and a destination")
			continue
		}
		for _, src := range plain[:len(plain)-1] {
			if !strings.HasPrefix(src, stagedPrefix) {
				add(in.line, "COPY reads %s, which is outside the staged context %s", src, stagedPrefix)
			}
		}
		copied = append(copied, plain[len(plain)-1])
	}
	for _, want := range []string{binaryPath, licencePath, cataloguePath} {
		if !contains(copied, want) {
			add(0, "nothing is copied to %s, which the install guide tells a reader to look at", want)
		}
	}

	// The binary comes from one file per architecture, so both entries of
	// the index are one pass over binaries that already exist.
	if !strings.Contains(string(body), stagedPrefix+"telecraft-linux-${TARGETARCH}") {
		add(0, "the binary is not copied from %stelecraft-linux-${TARGETARCH}, so one of the two architectures holds the other's binary", stagedPrefix)
	}

	// A non-root user, named by number because nothing resolves a name at
	// start.
	user := last(instructions, "USER")
	switch {
	case user == nil:
		add(0, "no USER, so the process runs as root")
	case strings.HasPrefix(user.args, "root"), strings.HasPrefix(user.args, "0:"), user.args == "0":
		add(user.line, "USER %s runs the process as root", user.args)
	}

	// The entrypoint is the binary and the default command is serve, so the
	// image is the whole CLI and running it with no arguments serves.
	entrypoint := last(instructions, "ENTRYPOINT")
	switch {
	case entrypoint == nil:
		add(0, "no ENTRYPOINT, so the image runs the base's command")
	case !strings.HasPrefix(entrypoint.args, "["):
		add(entrypoint.line, "ENTRYPOINT is in shell form and the image carries no shell; write it as a JSON array")
	case !strings.Contains(entrypoint.args, `"`+binaryPath+`"`):
		add(entrypoint.line, "ENTRYPOINT is not %s, so the image is not the CLI", binaryPath)
	}
	cmd := last(instructions, "CMD")
	switch {
	case cmd == nil:
		add(0, `no CMD, so the image needs an argument before it serves; the default command is ["serve"]`)
	case strings.Join(strings.Fields(cmd.args), "") != `["serve"]`:
		add(cmd.line, `CMD is %s, and the default command is ["serve"]`, cmd.args)
	}

	// The image binds every interface on both addresses, keeping each
	// flag's default port. The ports are read from the flags rather than
	// repeated here, so a default that moves in the command fails this
	// check instead of leaving the image listening somewhere else.
	defaults, err := serveDefaults(root)
	if err != nil {
		return nil, err
	}
	env := environment(instructions)
	for flag, variable := range map[string]string{"http": "TELECRAFT_HTTP", "listen": "TELECRAFT_LISTEN"} {
		port, ok := defaults[flag]
		if !ok {
			return nil, fmt.Errorf("reading the default of -%s from cmd/telecraft/serve.go", flag)
		}
		want := "0.0.0.0:" + port
		got, set := env[variable]
		switch {
		case !set:
			add(0, "%s is not set, so -%s keeps its loopback default and the address answers nothing from outside the container", variable, flag)
		case got != want:
			add(0, "%s is %s, and -%s defaults to port %s", variable, got, flag, port)
		}
	}

	ignore, err := checkIgnore(root)
	if err != nil {
		return nil, err
	}
	findings = append(findings, ignore...)
	return findings, nil
}

// checkIgnore reads .dockerignore. The context is the staged directory and
// nothing else, so a build cannot pick up a stale file from a working tree.
func checkIgnore(root string) ([]Finding, error) {
	path := filepath.Join(root, ".dockerignore")
	body, err := os.ReadFile(path)
	if err != nil {
		return []Finding{{Path: ".dockerignore", Message: "there is none, so the whole working tree is the build context"}}, nil
	}
	var excludesAll, admitsStaged bool
	for _, line := range strings.Split(string(body), "\n") {
		switch strings.TrimSpace(line) {
		case "*":
			excludesAll = true
		case "!" + strings.TrimSuffix(stagedPrefix, "/"), "!" + stagedPrefix + "**":
			admitsStaged = true
		}
	}
	var findings []Finding
	if !excludesAll {
		findings = append(findings, Finding{Path: ".dockerignore", Message: "nothing excludes the working tree; the context is the staged directory and nothing else"})
	}
	if !admitsStaged {
		findings = append(findings, Finding{Path: ".dockerignore", Message: "the staged directory is not admitted back, so every COPY fails"})
	}
	return findings, nil
}

// serveDefaults reads the port each address flag defaults to, from the
// command that owns them.
func serveDefaults(root string) (map[string]string, error) {
	body, err := os.ReadFile(filepath.Join(root, "cmd", "telecraft", "serve.go"))
	if err != nil {
		return nil, err
	}
	pattern := regexp.MustCompile(`fs\.String\("(http|listen)", "[^":]*:(\d+)"`)
	defaults := map[string]string{}
	for _, m := range pattern.FindAllStringSubmatch(string(body), -1) {
		defaults[m[1]] = m[2]
	}
	return defaults, nil
}

// instruction is one Dockerfile instruction with its continuations joined.
type instruction struct {
	name string
	args string
	line int
}

// parse reads the instructions, joining continuation lines and dropping
// comments. It is not a Dockerfile parser: it answers which instructions
// are present and what they carry, which is all the rules ask.
func parse(body string) ([]instruction, error) {
	var out []instruction
	var pending string
	var start int
	for i, raw := range strings.Split(body, "\n") {
		line := strings.TrimSpace(raw)
		if pending == "" && (line == "" || strings.HasPrefix(line, "#")) {
			continue
		}
		if pending == "" {
			start = i + 1
		}
		if strings.HasSuffix(line, `\`) {
			pending += strings.TrimSuffix(line, `\`) + " "
			continue
		}
		full := strings.TrimSpace(pending + line)
		pending = ""
		name, args, _ := strings.Cut(full, " ")
		out = append(out, instruction{name: strings.ToUpper(name), args: strings.TrimSpace(args), line: start})
	}
	if pending != "" {
		return nil, fmt.Errorf("the file ends inside a continued instruction at line %d", start)
	}
	return out, nil
}

// environment collects every variable the ENV instructions set.
func environment(instructions []instruction) map[string]string {
	env := map[string]string{}
	for _, in := range instructions {
		if in.name != "ENV" {
			continue
		}
		for _, pair := range strings.Fields(in.args) {
			name, value, ok := strings.Cut(pair, "=")
			if !ok {
				continue
			}
			env[name] = strings.Trim(value, `"`)
		}
	}
	return env
}

func last(instructions []instruction, name string) *instruction {
	var found *instruction
	for i := range instructions {
		if instructions[i].name == name {
			found = &instructions[i]
		}
	}
	return found
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
