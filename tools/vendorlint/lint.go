package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config mirrors vendorlint.yaml. Globs are slash-separated and relative to
// the scanned root; `**` crosses path separators, `*` stays within a segment.
type Config struct {
	Exclude []string `yaml:"exclude"`
	Scopes  []Scope  `yaml:"scopes"`
}

type Scope struct {
	Name    string   `yaml:"name"`
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
	Rules   []Rule   `yaml:"rules"`
}

// Rule flags every match of Pattern unless the match lies entirely inside a
// same-line match of one of the Allow patterns — so "Elastic Fleet" can
// permit the "Fleet" inside it while a bare "Fleet" stays an error.
type Rule struct {
	Pattern string   `yaml:"pattern"`
	Allow   []string `yaml:"allow"`
	Message string   `yaml:"message"`
}

type Finding struct {
	Path    string
	Line    int
	Scope   string
	Match   string
	Message string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: [%s] %q: %s", f.Path, f.Line, f.Scope, f.Match, f.Message)
}

type Result struct {
	Findings []Finding
	Scanned  int
}

// Run lints every regular file under root against the config at configPath
// (itself resolved relative to root).
func Run(root, configPath string) (Result, error) {
	raw, err := os.ReadFile(filepath.Join(root, configPath))
	if err != nil {
		return Result{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Result{}, fmt.Errorf("parsing %s: %w", configPath, err)
	}
	linter, err := compile(cfg)
	if err != nil {
		return Result{}, fmt.Errorf("compiling %s: %w", configPath, err)
	}

	var result Result
	fsys := os.DirFS(root)
	err = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || linter.excluded(path) {
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		if isBinary(data) {
			return nil
		}
		findings, matched := linter.lint(path, data)
		if matched {
			result.Scanned++
		}
		result.Findings = append(result.Findings, findings...)
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	sort.Slice(result.Findings, func(i, j int) bool {
		a, b := result.Findings[i], result.Findings[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Line < b.Line
	})
	return result, nil
}

type linter struct {
	exclude []*regexp.Regexp
	scopes  []compiledScope
}

type compiledScope struct {
	name    string
	include []*regexp.Regexp
	exclude []*regexp.Regexp
	rules   []compiledRule
}

type compiledRule struct {
	pattern *regexp.Regexp
	allow   []*regexp.Regexp
	message string
}

func compile(cfg Config) (*linter, error) {
	l := &linter{}
	var err error
	if l.exclude, err = compileGlobs(cfg.Exclude); err != nil {
		return nil, err
	}
	for _, s := range cfg.Scopes {
		cs := compiledScope{name: s.Name}
		if cs.include, err = compileGlobs(s.Include); err != nil {
			return nil, fmt.Errorf("scope %q: %w", s.Name, err)
		}
		if cs.exclude, err = compileGlobs(s.Exclude); err != nil {
			return nil, fmt.Errorf("scope %q: %w", s.Name, err)
		}
		for _, r := range s.Rules {
			cr := compiledRule{message: r.Message}
			if cr.pattern, err = regexp.Compile(r.Pattern); err != nil {
				return nil, fmt.Errorf("scope %q pattern %q: %w", s.Name, r.Pattern, err)
			}
			for _, a := range r.Allow {
				re, err := regexp.Compile(a)
				if err != nil {
					return nil, fmt.Errorf("scope %q allow %q: %w", s.Name, a, err)
				}
				cr.allow = append(cr.allow, re)
			}
			cs.rules = append(cs.rules, cr)
		}
		l.scopes = append(l.scopes, cs)
	}
	return l, nil
}

func (l *linter) excluded(path string) bool {
	return anyMatch(l.exclude, path)
}

// lint returns the findings for one file plus whether any scope claimed it.
func (l *linter) lint(path string, data []byte) ([]Finding, bool) {
	var active []compiledScope
	for _, s := range l.scopes {
		if anyMatch(s.include, path) && !anyMatch(s.exclude, path) {
			active = append(active, s)
		}
	}
	if len(active) == 0 {
		return nil, false
	}

	var findings []Finding
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		for _, s := range active {
			for _, r := range s.rules {
				for _, m := range violations(r, line) {
					findings = append(findings, Finding{
						Path:    path,
						Line:    lineNo,
						Scope:   s.name,
						Match:   m,
						Message: r.message,
					})
				}
			}
		}
	}
	return findings, true
}

// violations returns the rule-pattern matches on line that are not covered by
// an allow-pattern match.
func violations(r compiledRule, line string) []string {
	matches := r.pattern.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return nil
	}
	var allowed [][]int
	for _, a := range r.allow {
		allowed = append(allowed, a.FindAllStringIndex(line, -1)...)
	}
	var out []string
	for _, m := range matches {
		covered := false
		for _, span := range allowed {
			if m[0] >= span[0] && m[1] <= span[1] {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, line[m[0]:m[1]])
		}
	}
	return out
}

func anyMatch(res []*regexp.Regexp, path string) bool {
	for _, re := range res {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}

func compileGlobs(globs []string) ([]*regexp.Regexp, error) {
	var out []*regexp.Regexp
	for _, g := range globs {
		re, err := globToRegexp(g)
		if err != nil {
			return nil, fmt.Errorf("glob %q: %w", g, err)
		}
		out = append(out, re)
	}
	return out, nil
}

// globToRegexp converts a slash-separated glob to an anchored regexp.
// `**` matches across separators (a trailing `/**` also matches the bare
// prefix's children), `*` and `?` stay within one path segment.
func globToRegexp(glob string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		switch glob[i] {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(glob[i])))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// isBinary reports whether data looks like a binary file (NUL byte in the
// first 8KB); such files are skipped.
func isBinary(data []byte) bool {
	n := len(data)
	if n > 8192 {
		n = 8192
	}
	return bytes.IndexByte(data[:n], 0) >= 0
}
