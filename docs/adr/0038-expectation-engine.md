# ADR-0038: The Expectation engine — machinery behind existing verdicts, never new vocabulary

- Status: accepted
- Date: 2026-08-17 (session G6)

## Context

REQ-051 is the differentiator: from the Intended config at a SHA, derive
what telemetry should arrive, and check it — green means "the config
worked", never merely "the config applied". The glossary pinned Expectation
as derived-never-authored, keyed per (Service, Environment) for data claims
and per Tier for pipeline claims, and left engine mechanics to this
session. The root tension: ADR-0004's outcome cross already contains
`broken_pipeline` and `not_delivered` — words that sound exactly like
expectation-red — but the cross is per (Service, Environment, Requirement),
and a Tier-keyed pipeline claim ("this instantiated exporter is silent")
has no Requirement to attach an outcome to. G5 refused an eighth outcome
once already (ADR-0034); P4 proved expectation-red legible beside
delivery-red and conformance-red as a *band*, not as new words.

## Decision

1. **The engine is machinery, not vocabulary.** Data claims are the
   computation that decides the Observed leg of ADR-0004's cross — a
   Service's "expected traces never landed" *is* `not_delivered`; the
   Expectation is why the evaluator knew to look. Pipeline claims join the
   Tier-attached finding family ADR-0030/0035 established. No eighth
   outcome, no fourth reading. "Expectation" names the derived input;
   surfaces' Expectation band shows outcomes and findings *sourced from*
   expectations, and microcopy carries that.
2. **Three claim kinds, derived literal-only.** (a) **Arrival claims**, per
   (Service, Environment, signal): the signal should land, derived from the
   Service's Paths through the rendered pipelines; feeds `not_delivered`.
   (b) **Enrichment claims**, per (Service, Environment): attributes the
   rendered config *explicitly, literally* inserts or upserts (static
   `resource`/`attributes`/`transform` actions with constant values) should
   be present on landed telemetry. Anything requiring knowledge of a
   component's runtime behaviour (`k8sattributes`, `resourcedetection`)
   yields **no claim** — therefore `unknown`, never red. (c)
   **Self-telemetry claims**, per Tier: each instantiated component should
   emit its own telemetry under R-4's join keys, with R-4's caveats
   (identity-dropping singletons, `capabilities`/`fanout` synthetic kinds,
   no pipeline id on receivers/exporters) modelled as expected shapes,
   never as failures. The principle: **the engine claims only what it can
   read off the artefact at a SHA — never what it believes about component
   semantics.** A curated, Catalogue-resident behaviour-model layer is the
   named post-v1 extension seam (OQ-18), not a v1 feature: a false
   expectation-red would poison trust in the whole band.
3. **Derivation runs at evaluation time, as a pure function of the
   artefact.** No expectations file is committed: a materialised copy is a
   drift surface against the artefact it restates, and a committed file
   would invite authored semantics by social pressure — derived-never-
   authored is easier to defend if the claim set never looks like a file.
   The render-in-PR check (ADR-0028) *displays* the expectation diff
   ("this change adds an arrival claim for traces"), impact-report style
   (ADR-0020 precedent) — computed twice, stored never. Memoisation is
   in-memory, keyed by (SHA, Tier) at most, confirmed loseable;
   evaluator-internal, so ADR-0032's closed cache list stands unamended.
   Rollouts come free: a dual-bound Tier's cohorts are judged against the
   artefact each collector's stamp says it runs.
4. **Timing semantics.** (a) Claims are judged against the artefact the
   collector reports running — the stamped SHA, never head. Delivery
   failure is structurally unable to cascade into expectation-red: an
   unapplied config's claims are never in force; the running artefact's
   claims still are. This mechanises ADR-0004's qualifying rule
   (`broken_pipeline` with `applied` is a pipeline fault; with drift, a
   delivery fault in pipeline clothes). The named consequence: a collector
   pinned to an ancient artefact is judged green against ancient claims —
   correct band separation; the staleness is delivery/drift's red
   (ADR-0005, ADR-0026) and must never be "fixed" by blending. (b) A
   **settle window** per claim kind follows APPLIED at a new SHA: claims
   read neutral-pending, never red or green; self-telemetry settles in
   seconds, arrival/enrichment longer. (c) Shortfalls are
   persistence-dampened with ADR-0035's mechanism — one knob vocabulary,
   no parallel invention. (d) Observation windows are per (claim, signal)
   with adopter-overridable defaults, generous enough to survive overnight
   quiet; `Known: false` readings ⇒ `unknown`, never red.
5. **Finding placements — the `expectation` finding kind.**
   (a) Requirement-backed data claims produce no finding of their own:
   they are the machinery behind the cross, whose severity and routing the
   Requirement world already owns. (b) **Unbacked data claims** (the
   config implies a signal no Requirement demands, and it never arrives)
   raise a Service-attached, expectation-kind finding, **advisory-grade,
   never violation** — no human demanded the signal, so it cannot fail
   compliance. Remediation text is honest about the fork: *fix the
   pipeline, or delete the dead lane* — doubling as dead-config detection,
   the aged-`never_seen` move (ADR-0035 §7). Routed to the Service owner,
   with the Tier's pipeline findings attached as context, because helper
   metrics are component totals (R-4): the engine cannot honestly localise
   where on the Path the data died. (c) **Pipeline claims** raise
   Tier-attached, Tier-owner-routed, expectation-kind findings that *can*
   reach violation-grade once dampened: an instantiated exporter at 100%
   send-failure, or a component silent past its settle window, is "the
   config didn't work" in its sharpest form. The asymmetry is deliberate:
   a dead lane may be intentional; a failing instantiated component never
   is. `expectation` joins ADR-0017's ratio-plus-worst as a new finding
   kind — no blended number; Exemptions (ADR-0037) apply unmodified.

## Consequences

- P4's Expectation band has a data source with zero new verdict words;
  the card contract (ADR-0041) carries band states, not colours.
- OQ-12 gains a note: expectation derivation is recomputed per evaluation
  and inherits the evaluator's cost ceiling.
- OQ-18 opened: the component behaviour-model layer for wider enrichment
  claims, post-v1.
- Glossary gains Claim (three kinds), Settle window, and the `expectation`
  finding kind; the Expectation entry's "engine mechanics are G6's" is
  resolved to this ADR.

## Sources

- Session G6; REQ-051/052; ADR-0004, 0005, 0013, 0017, 0020, 0022, 0025,
  0026, 0028, 0029, 0030, 0032, 0033, 0034, 0035, 0037;
  `docs/research/2026-08-14-r4-self-telemetry-attributes.md`; P3/P4
  verdicts.
