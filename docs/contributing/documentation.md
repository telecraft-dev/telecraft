---
title: Writing documentation
description: The directory structure, the front matter schema, the navigation manifest, the house style, the rules for interface text, and how to add a page.
order: 9
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
explains what each corpus is for.

Published pages never point into the corpus. A concept, guide, or reference
page, the glossary, the documentation home page, `README.md`, and
`SECURITY.md` do not cite an ADR number, a requirement id, a research note,
or a prototype verdict, and they do not link into those directories. A
reader of the product needs what Telecraft does, not the argument that
produced it. Where a page needs a reason, it gives the reason in its own
words. This Contributing section is the only place that links into the
corpus, and it does so through the decisions page. Do not copy a decision
into a page either: the copy goes stale and the ADR does not.

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
files). This covers every dash used as punctuation, en dashes included, and
it applies to the whole repository: code comments, test messages, YAML and
CSS comments, fixtures, and the decision corpus, not only the published
pages. The vendor-word lint enforces it (see [the lints](#the-lints)).

**No decision references in anything a user sees.** `ADR-0042`, `REQ-031`,
`§3`, `OQ-3`, and issue numbers belong in code comments and in the corpus.
They never appear in published pages, console text, CLI help and output,
error messages, findings and their remediation text, or the comments the
renderer writes into generated artefacts. Give the reason in plain words if
the reader needs it, otherwise leave it out.

**No rationale in anything a user sees.** Stripping the reference is not
enough: the argument has to go too. A surface reports, it does not defend
itself. "Ratios never blend, and waived counts ride every level" justifies a
design decision to a reader who never questioned it, and prose that argues
reads as a prototype whatever it is attached to. State the reading and stop.
A page may explain what the product does and what follows for the reader; it
does not argue that the design is right or narrate the alternatives that
lost. The reasoning belongs in the decision record and in a comment beside
the code that implements it, which is where a reader who wants it will look.

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

## Interface text

Console surfaces, CLI help and output, error messages, and findings with their
remediation text are held to everything above, plus the rules here. They are
the surfaces a reader meets without having chosen to read anything, so they
carry the least room for a sentence that is not pulling its weight.

**An instrument, not a dashboard.** That line comes from the visual identity
and it governs the words as much as the drawing: label the reading and stop.

**A heading names what a thing is, not what to do about it.** "Tiers with
findings" is a label; "Look here first" is an instruction, and an instruction
in a heading reads as scaffolding somebody forgot to remove.

**A control says exactly what happens, and the confirmation says it happened
in the same words.**

**Link text names its destination in the reader's words**, not the product's
internal name for a surface. "See all Tiers" beats "Open the whole shelf": the
reader wants the Tiers, and the shelf is our word for where they are kept.

**Outside the glossary, prefer the plain word.** Governed domain nouns are
exact, and they stay: a Tier is a Tier on every surface, and so are
Environment, Rollout, Blueprint, and Service Class. Incidental vocabulary is
not exact and is where jargon accumulates unnoticed. Where the glossary
already holds the concept under a different word, use the glossary's word: it
is the one a reader can look up.

**Change a term on every surface that shows it.** Two surfaces naming one
thing two ways is a defect even when each is defensible alone, and it is worse
when one surface links to the other.

Worked examples from the pass over the console's landing surface:

| Instead of | Write | Because |
|---|---|---|
| Ratios never blend, and waived counts ride every level | *(nothing)* | Argues for a decision nobody questioned |
| Look here first | Tiers with findings | A label, not an instruction |
| A concern to claim, not a failure, and in no compliance denominator | 3 served, 2 foreign. | Editorial, and "denominator" is internal |
| match no Tier selector | don't match any Tier | "Selector" is an implementation word |
| Each row aggregates its whole subtree | Totals include every team below | "Subtree" is a data structure, not a team |
| 1 waived | 1 exempt | Exemption is the glossary term, so "exempt" leads somewhere |
| no verdict to give | no result yet | Affected |
| 3 more not shown here (Home draws 6) | Showing 6 of 9 | The page was talking about itself |

## Vocabulary

The [glossary](../glossary.md) is the ubiquitous language, and it binds
code, documentation, and the console alike. Its terms are not synonyms to
vary for readability: a Tier is a Tier on every surface.

- Upstream vocabulary is adopted as is. Where OpenTelemetry or OpAMP has a
  word, Telecraft uses it; a local synonym is a lint error.
- Seam names are domain terms (`TelemetryProvider`, `EstateProvider`).
  Implementations are qualified with the vendor's product, never the
  company: `ElasticFleet`, `Elasticsearch`, `GrafanaFleetManagement`.
- A Service Class is never written "Tier N". A Tier is a position in the
  topology.
- Every capitalised domain term an ADR introduces must appear in the
  glossary, defined for a reader who has not read the ADR.

## The lints

`go run ./tools/vendorlint` runs over `docs/**` as well as over the code, and
it fails CI. It is a pattern lint configured in `vendorlint.yaml`, and three
of its scopes are about prose:

- In the docs the vendor-word rule narrows to one word: a bare `Fleet`
  appears nowhere. Qualify the vendor's product instead, as Elastic Fleet,
  `ElasticFleet`, `GrafanaFleetManagement`, or Datadog Fleet Automation.
  Naming the banned word rather than using it is allowed: backticks, bold,
  and curly quotes all read as quoting it. The research, prototype and
  branding trees are excluded from this rule, because they record external
  reality verbatim.
- The `prose` scope flags an em dash or en dash anywhere in the repository,
  except in generated Catalogue artefacts, which record upstream text.
- The `published` scope flags `ADR-`, `REQ-`, `OQ-`, and `§` references on
  any published page, in `README.md`, and in `SECURITY.md`.

`go run ./tools/docslint` checks the front matter of every published page.

Run both before you push. They are the same commands CI runs, and they take
a second.

## How to add a page

1. **Pick the section.** Concept, guide, reference, or contributing. If a
   page fits two, it usually wants splitting.
2. **Create the file** at `docs/<section>/<name>.md`, with a lower-case
   hyphenated filename that matches what the page is about.
3. **Write the front matter** exactly as above, with the next free `order`
   within the section.
4. **Write the page** in house style.
5. **Add it to `docs/nav.yaml`**, in the position its `order` claims.
6. **Verify.** Run `go run ./tools/vendorlint` and `go run ./tools/docslint`
   and check that both come back clean. Follow every link you added, including relative links between
   pages.
7. **Open the pull request**, following the
   [contribution flow](index.md#branches-and-pull-requests).
