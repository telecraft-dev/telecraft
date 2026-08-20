// The documentation front-matter check.
//
// Every published page opens with a YAML block of `title`, `description`
// and `order` (docs/contributing/documentation.md). That block is not
// decoration: `docs/nav.yaml` and the front matter together are the whole
// contract between this repository and whatever builds telecraft.dev, and
// the site is built in a different repository (issue #74). So a page whose
// front matter does not parse does not fail here — it fails over there,
// after the change has merged, in a build nobody watching this repository
// is looking at.
//
// That is exactly what happened to docs/reference/estate-layout.md, whose
// description read `The estate repository layout: root files, …` — an
// unquoted value with a colon in it, which YAML reads as a nested mapping.
// It took the whole documentation build down and was invisible here.
//
// The check is deliberately narrow. It reads the block, and it does not
// judge the prose: house style belongs to review, and a lint that argued
// about wording would be ignored.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// matter is the block every published page carries. The fields are
// required together: a page with a title and no description is published
// with an empty summary rather than rejected, which is the kind of quiet
// degradation this check exists to prevent.
type matter struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Order       *int   `yaml:"order"`
}

// nav names the directories that are the working corpus rather than the
// published documentation. They are excluded here for the same reason the
// site excludes them: they are a record of how the product was argued into
// existence, they assume the whole history, and they were never written to
// the page schema.
type nav struct {
	NotPublished []string `yaml:"not_published"`

	// Pages declares the handful of documents that sit outside a section
	// directory. The manifest carries their title and order itself, so
	// those pages are published without front matter and are correct that
	// way — `docs/glossary.md` is one. Checking them against the page
	// schema would report a page that works.
	Pages []struct {
		Path string `yaml:"path"`
	} `yaml:"pages"`
}

func main() {
	root := "docs"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	skip, manifest, err := manifestScope(filepath.Join(root, "nav.yaml"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "docslint: %v\n", err)
		os.Exit(1)
	}

	var problems []string
	scanned := 0

	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if skipped(rel, skip) {
			return nil
		}
		scanned++
		// A page the manifest describes is still checked when it carries
		// front matter — a block that does not parse is a defect wherever
		// the title comes from — but its absence is not a finding.
		if problem := check(path, manifest[filepath.ToSlash(rel)]); problem != "" {
			problems = append(problems, problem)
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "docslint: %v\n", err)
		os.Exit(1)
	}

	sort.Strings(problems)
	for _, p := range problems {
		fmt.Fprintln(os.Stderr, p)
	}
	if len(problems) > 0 {
		fmt.Fprintf(os.Stderr, "\ndocslint: %d page(s) with front matter the site cannot read\n", len(problems))
		os.Exit(1)
	}
	fmt.Printf("docslint: clean (%d published pages)\n", scanned)
}

// unpublished reads the working-corpus list out of the navigation manifest
// rather than keeping a second copy of it here. A directory added to
// `not_published` stops being checked without anyone remembering to say so
// twice.
func manifestScope(path string) ([]string, map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the navigation manifest: %w", err)
	}
	var n nav
	if err := yaml.Unmarshal(data, &n); err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	described := map[string]bool{}
	for _, p := range n.Pages {
		described[p.Path] = true
	}
	return n.NotPublished, described, nil
}

func skipped(rel string, skip []string) bool {
	rel = filepath.ToSlash(rel)
	for _, s := range skip {
		if rel == s || strings.HasPrefix(rel, strings.TrimSuffix(s, "/")+"/") {
			return true
		}
	}
	return false
}

// check reports the one problem worth reporting per page, in the words the
// author needs to fix it. A parse failure names the YAML error, because
// "invalid front matter" without it sends the reader looking at the wrong
// line.
func check(path string, describedByManifest bool) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("%s: %v", path, err)
	}
	text := string(data)

	if !strings.HasPrefix(text, "---\n") {
		if describedByManifest {
			return ""
		}
		return fmt.Sprintf("%s: no front matter — every published page opens with title, description and order (docs/contributing/documentation.md)", path)
	}
	end := strings.Index(text[4:], "\n---")
	if end < 0 {
		return fmt.Sprintf("%s: the front matter block is never closed", path)
	}

	var m matter
	if err := yaml.Unmarshal([]byte(text[4:4+end]), &m); err != nil {
		return fmt.Sprintf("%s: front matter is not valid YAML — %v\n    a value containing a colon must be quoted: description: \"Layout: root files\"", path, err)
	}

	var missing []string
	if strings.TrimSpace(m.Title) == "" {
		missing = append(missing, "title")
	}
	if strings.TrimSpace(m.Description) == "" {
		missing = append(missing, "description")
	}
	if m.Order == nil {
		missing = append(missing, "order")
	}
	if len(missing) > 0 && !describedByManifest {
		return fmt.Sprintf("%s: front matter is missing %s", path, strings.Join(missing, " and "))
	}
	return ""
}
