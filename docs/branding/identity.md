# Identity

- Status: **decided** — direction seeded in [`naming.md`](naming.md), settled
  by ADR-0047
- Date: 2026-08-19

The name decision closed with a four-line identity direction and a note that
it was a seed for this file. This is that file. The implementable half —
tokens, type scale, marks, palette values — lives in
[`design-system.md`](design-system.md); the decisions and their reasoning live
in [ADR-0047](../adr/0047-visual-identity-and-design-tokens.md).

## What Telecraft looks like, in one line

An instrument, not a dashboard.

## The direction

**Warm-industrial craftsmanship, read through precision rather than
nostalgia.** The product's whole thesis is a reading measured against a floor,
so the visual language is the language of measurement: datum lines, hairlines,
plates rather than boxes, and numbers set to line up. Where the interface has
personality it comes from being exact, not from being decorated.

Held from the naming decision, unchanged:

- Wordmark-first. The mark supports the word; it does not replace it.
- Workshop and precision references — calipers, blueprint linework — which
  also nod to Blueprints as a domain object.
- Telemetry-signal motifs.
- Serious in the documentation, warm in the community surfaces, one mark
  across both.

### Explicitly avoided

- **Blocky or pixel styling.** The "-craft" suffix invites a Minecraft echo;
  the identity is the mitigation for it, so this is not negotiable.
- **Railway kitsch** and **hop or beer imagery**, both inherited from
  metaphor families rejected during naming.
- **The observability default**: a dark hero with one acid accent, a gradient,
  and a grid of vendor logos.

## The mark

Three reading bands, stacked and decreasing, measured against a vertical
datum in the brand amber. The bands are the favicon's original idea — the
card face's three readings (ADR-0041 §2) — and they survive; the datum is
what they are read against, which is the product's thesis drawn literally.
Tone falls with length, so the single-colour version loses nothing it needs.

The wordmark is `telecraft` in Atkinson Hyperlegible Next at weight 600,
outlined from the vendored face, tracked −12/1000, with one drawn
intervention: the f–t crossbars in "craft" join into a single bar, on the
syllable that says craft.

The pack lives in [`pack/`](pack/): icons, bare marks, wordmarks, and
horizontal and stacked lockups, each drawn for both grounds plus a
single-colour version, written by `pack/tools/build.py` from the `tokens.css`
values and never edited by hand. Clear space, minimum sizes and the usage
rules are in [`pack/README.md`](pack/README.md). `console/public/favicon.svg`
is the canonical icon, written by the same script, and the retired navy
`#1e2b3a` is gone (issue #101) — every colour in the mark is now one the
palette check can see.

## Voice

The documentation already has a voice and holds it consistently. It is
recorded here so it survives contact with a second contributor.

- **State the cost.** Every claim in the README names what it asks of the
  reader. A benefit with no cost attached reads as marketing and is not our
  register.
- **Short declaratives carry the weight.** "Green means the config worked."
  "Nothing sits in the telemetry path." Say the thing, then stop.
- **Specific beats impressive.** Name the file, the number, the ADR. Prefer
  "roughly one card fewer per row" to "a minor layout impact".
- **No superlatives, no invented metrics.** If a number is not measured, it is
  not written.
- **Say what is not decided.** Open questions are a register in this project,
  not an embarrassment.
- **British English**, matching the corpus and the code: `colour`, `licence`,
  `judgement`, `normalise`, and `--colour-bg` in the tokens.

Interface text follows the same rules, plus one: a control says exactly what
happens, and the confirmation says it happened in the same words.

## Where this applies

Four surfaces, one system: the console, the documentation, `telecraft.dev`,
and the command line. The last of those has no visual design at all today and
is the first Telecraft surface most people meet, since Conformance is the
first rung and costs a connection string. Whether it gains human-readable
output is a product decision, recorded as open.
