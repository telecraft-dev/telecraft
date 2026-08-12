# Naming (G0)

- Status: **shortlist prepared — decision pending G0 grill**
- Date: 2026-08-12

## Why rename

"Amp-Up" fails twice. It promises OpAMP, which the product does not lead with
(serving is one of three rungs). And it collides with **Sourcegraph Amp**, a
major agentic-coding brand in 2026 (40,000+ teams adopted in two months;
`amp`/`ampcode` own the search results). A rename is cheap until the repo is
published, which is now. Identity work (`identity.md`) starts once the name
lands.

## What a good name carries

The product is a control plane that governs, renders and checks telemetry
routing while never carrying the traffic. The domain model's central bet is
the **Hop**; the vocabulary is already railway-flavoured (interchange,
on-ramp, Stage, Path). Names were sought in two veins: the Hop, and railway
signalling infrastructure (the apparatus beside the track that governs what
proceeds, without carrying the train).

## Shortlist (checked 2026-08-12)

| Candidate | Metaphor | GitHub | npm | Collisions found |
|---|---|---|---|---|
| **Hopyard** | Where hops are grown and trained up wires — the place Hops are cultivated and governed. Brewing double-meaning gives warm OSS branding (logo: hop cone as network graph). | only empty/tiny repos; `hopyard` user squatted, no activity | free | a hop-farm website, a craft-bar template. Effectively clean. |
| **Telhop** | Telemetry + Hop, literally. Short, typeable, unambiguous in search. | zero repos found | free | none found. Cleanest of all, at the cost of being the least evocative. |
| **Signalpost** | The railway signal that governs whether traffic proceeds — governance beside the track, never in the path. | only empty repos (nice omen: one 0★ repo is an OTel-stack installer) | free | railway-simulation hobby projects; `signalbox.io` is an unrelated company (rail data API) — adjacent word, not this word. |
| Stagecraft | Craft applied to Stages. | org name taken; alphagov + telus-labs repos exist (small) | — | telus-labs/stagecraft is an active AI-dev-pipeline tool. Weaker. |
| Pointsman | The operator of railway points, deciding which path traffic takes. | tiny repos only | — | obscure term, and gendered — flagged, not recommended. |

## Rejected in sweep

- **Otelier** — otelier.io is an operating hotel-tech SaaS; trademark risk.
- **Gantry** — Gantry5 theme framework (1k★).
- **Trellis** — two 13k★ projects, incl. an AI agent harness.
- **Switchyard** — NVIDIA NeMo Switchyard (active) + JBoss SwitchYard.
- **Signalbox** — signalbox.io holds the domain and an API product.
- **Waymark** — multiple active products (WordPress mapping, waymark.dev).

## Recommendation to grill

**Hopyard** first (evocative, clean, brandable, names the design's central
bet), **Telhop** second (cleanest, plainest), **Signalpost** third (best
governance metaphor, slightly crowded by the adjacent signalbox.io).

Renames that follow the decision: repo name, Go module path, the commit-stamp
resource key (`ampup.commit` → `<name>.commit`, ADR-0013), doc references to
"Amp-Up", README pitch.

Still to check before the decision is final: `.dev`/`.io` domain availability
for the chosen name, and a basic trademark search (EUIPO/USPTO word-mark
lookup) — five minutes each, do them for the winner only.

## Sources

- [Amp Code Review 2026](https://baeseokjae.github.io/posts/amp-code-review-2026/)
- [Sourcegraph: Agentic Coding in 2026](https://sourcegraph.com/blog/agentic-coding)
- [Signalbox API docs](https://docs.signalbox.io/docs) (the adjacent-name company)
- GitHub/npm availability checked live via `gh search repos`, `gh api /users/<name>`, npm registry.
