# P1 — Blueprint composer

- Date: 2026-08-13
- Status: verdict recorded
- Prototype: `.proto/p1-blueprint-composer/` (throwaway, gitignored, fixture
  data; single self-contained `index.html`, variants switchable via
  `?variant=` / ←→ keys)

## Question

Which mental model survives phase ordering + allow-lists? Plus the G2 probes:
does the greyed-vs-hidden palette read (ADR-0022 §5), is the one hard block
legible as different-in-kind from warnings (§3), and does the Environment
toggle match intuition (ADR-0023)?

## What was built

Four variants over one shared blueprint (fixtures deliberately exercising
every G2 decision: a deprecated receiver with migration text, an alpha
processor greyed in production, a not-allowed exporter already present so
Save is blocked with "request a Grant", an adopter-Grant component, hidden
palette entries with an admitted count) — all judged by ONE validation
engine re-run on every click (ADR-0022's one-rulebook rule, demonstrated):

- **A · Catalogue-first** — palette → per-signal canvas → findings dock
- **B · Requirement-first** — requirements as the primary surface; composing
  = discharging them; coverage bar; one-click suggestion adds
- **C · Signal lanes** — one lane per signal, per-(component, signal)
  stability, per-lane targeted adds and floor labels
- **D · Node canvas** — components as nodes, per-signal coloured edges,
  Simulate pulses, live YAML flyout (added mid-session on reaction)

## Verdict

**Three complementary surfaces ship; C merged into A.** A and C proved to be
the same mental model at different densities, so they merged: **A ·
Composer** — palette left (click = add to every supported signal), blueprint
right as per-signal lanes carrying C's floor chip, per-(component, signal)
stability and per-lane "+ add to «signal»", findings as a full-width
multi-column strip across the bottom. **B** ships as the compliance overview
("what checkout owes"). **D** ships as the flow view — and explicitly as an
*authoring* surface (add/remove), not read-only.

Structural choices that survived contact:

- **Greyed-vs-hidden palette** read without explanation; the hidden-count
  admission line ("2 components hidden by your allow-list") stays.
- **The one hard block** (Save disabled + "request a Grant") read as
  different-in-kind from warnings.
- **Environment toggle**: production→staging clearing floor findings and
  greying while deprecation and allow-list findings persist matched
  intuition exactly.
- **D's edge routing**: processors staggered vertically by which signals use
  them (shared → centre, logs-only → logs level) with 90° orthogonal
  Manhattan edges and per-signal bend offsets. Curved bypass arcs were
  rejected twice; straight-past-the-node is what makes "skipped" legible.
- **YAML flyout** (all variants): live rendered config, pushes the app aside
  rather than covering it, click-off closes, read-only — git is where hand
  edits belong.
- **Simulate** (D): per-journey dots — born at a receiver, traversing the
  full chain to an exporter, signal groups staggered. All-at-once per-edge
  pulses read wrong.

## What it changed

- **G3**: the composer's output object is the Blueprint; A's lane structure
  (per-signal chains + collector-wide extensions) is the shape the schema
  must serialise. Phase ordering (memory_limiter first, batch last) was
  maintained naturally in A and D via ordering findings — no dedicated
  ordering UI needed.
- **G7**: composer IA starts from three surfaces (A build / B owe / D flow)
  plus the YAML flyout. New OQ-14 requirement recorded: a **flat,
  filter-first estate list** (every config/collector with status, "show all
  non-compliant", sliceable by finding kind — the InfoSec workflow), distinct
  from P2's two hierarchy roll-up views.
- **G6/G7 parked product idea**: real simulate — synthetic telemetry through
  the *rendered* config with per-node before/after — the cosmetic version
  already communicates flow but overpromises nothing yet.
- Reaction probes and the merge history live in
  `.proto/p1-blueprint-composer/QUESTIONS.md`.
