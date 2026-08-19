---
title: Writing documentation
description: The directory structure, the front matter schema, the navigation manifest, the house style, and how to add a page.
order: 7
---

# Writing documentation

The published documentation lives in `docs/`, beside the design corpus it
draws on. Anything a reader of the product needs goes here; anything only a
contributor needs goes in this section, so the rest stays clean for users.

## The directory structure

Four sections, each a directory of Markdown files with an `index.md`:

| Section | Audience and purpose |
|---|---|
| `docs/concepts/` | What the product is and how to think about it: the model, the vocabulary, the ideas a reader needs before the instructions make sense. |
| `docs/guides/` | Task-shaped instructions: how to do a thing, start to finish. |
| `docs/reference/` | Lookup material: commands, flags, schemas, endpoints, configuration. |
| `docs/contributing/` | Developer-facing material: this section. |

`docs/index.md` is the documentation home page.

The design corpus keeps its own directories (`docs/adr/`,
`docs/requirements/`, `docs/research/`, `docs/prototypes/`, plus
`docs/plan.md`, `docs/open-questions.md` and `docs/glossary.md`) and is not
part of the four-section structure. The [decisions page](decisions.md)
explains what each corpus is for. Link into the corpus from a documentation
page when a reader needs the reasoning; do not copy a decision into a page,
because the copy goes stale and the ADR does not.

## Front matter

Every page starts with exactly this YAML block, before any other content:

```yaml
---
title: Sentence case page title
description: One sentence saying what the page covers.
order: 3
---
```

| Field | Type | Meaning |
|---|---|---|
| `title` | string | The page title, in sentence case. It is the page's name in navigation, so make it name the page rather than summarise it. |
| `description` | string | One sentence saying what the page covers. It is the summary shown alongside the title. |
| `order` | integer | Where the page sits within its section. `index.md` is always `1`, and the remaining pages number upward from `2` in reading order. |

Numbers are unique within a section. If you insert a page in the middle,
renumber the pages after it rather than using a fractional or duplicate
value.

## The navigation manifest

`docs/nav.yaml` is the navigation manifest the site build consumes. It
declares the sections and the pages within them, in the order they appear.

A page is not published because it exists on disk: it is published because
`docs/nav.yaml` lists it. Adding a file without adding its entry ships a page
nobody can navigate to.

## House style

The house style is mandatory for every word a reader sees: documentation, UI
text, error messages, release notes, and commit-adjacent prose. It is the
[Google developer documentation style guide](https://developers.google.com/style)
with two overrides, and the overrides win wherever they clash.

**British English.** Colour, behaviour, organise, optimise, catalogue, grey.
Licence as a noun and license as a verb; practice as a noun and practise as a
verb. Dates run day month year: 18 August 2026. Code stays as code: an
identifier, an API field, a command, or quoted output keeps its exact
spelling, so a CSS `color` property is still `color` in a sentence about
colour.

**No em dashes.** Where an em dash would carry an aside or a break, use a
comma, brackets, a colon, or two sentences. Write ranges with "to" (10 to 20
files). This covers every dash used as punctuation, en dashes included.

The rules that come up constantly:

- **Sentence case for every heading**, including the page title.
- **Second person.** Address the reader as "you". Reserve "we" for a genuine
  recommendation from the project.
- **Active voice, with the actor visible.** "The server rejects the request",
  not "the request is rejected".
- **Present tense.** "The command prints", not "the command will print".
- **Code font for anything the machine reads or writes**: commands,
  filenames, paths, identifiers, values, output.
- **Numbered lists for ordered steps**, bulleted lists for everything else.
- **Serial comma**: a, b, and c.
- **Spell out zero to nine**; use numerals from 10, and always for versions
  and other technical values.
- **Link text names its destination.** A bare "here" or "this page" never
  carries a link.
- **Placeholders in upper snake case** (`PROJECT_ID`, `API_KEY`), explained at
  first use.
- **No filler that judges the reader's effort**: cut "simply", "easily",
  "just", "obviously". Cut "please" from instructions.
- **No marketing language.** State what a thing does and let that be the sell.
  No invented metrics, no superlatives, no claims you cannot verify.
- **Describe what exists today.** Nothing unreleased is announced or hinted
  at. If a page describes planned work as if it ships, that is a bug.

## The vendor-word lint

`go run ./tools/vendorlint` runs over `docs/**` as well as over the code, and
it fails CI. In the docs the rule narrows to one word: a bare `Fleet` appears
nowhere. Qualify the vendor's product instead, as Elastic Fleet,
`ElasticFleet`, `GrafanaFleetManagement`, or Datadog Fleet Automation.

Naming the banned word rather than using it is allowed: backticks, bold, and
curly quotes all read as quoting it. The research, prototype and branding
trees are excluded entirely, because they record external reality verbatim.

Run the lint before you push. It is the same command in CI, and it takes a
second.

## How to add a page

1. **Pick the section.** Concept, guide, reference, or contributing. If a
   page fits two, it usually wants splitting.
2. **Create the file** at `docs/<section>/<name>.md`, with a lower-case
   hyphenated filename that matches what the page is about.
3. **Write the front matter** exactly as above, with the next free `order`
   within the section.
4. **Write the page** in house style.
5. **Add it to `docs/nav.yaml`**, in the position its `order` claims.
6. **Verify.** Run `go run ./tools/vendorlint` and check that it comes back
   clean. Follow every link you added, including relative links between
   pages.
7. **Open the pull request**, following the
   [contribution flow](index.md#branches-and-pull-requests).
