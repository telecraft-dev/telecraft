# ADR-0022: One evaluator as a service; one hard rule at render

- Status: accepted
- Date: 2026-08-13 (session G2)

## Context

Three policy signals now exist — allow-list membership (ADR-0021), stability
floors (ADR-0023), lifecycle (deprecated/unmaintained, ADR-0020) — and four
places they could act: the composer Palette, the render step, CI on the
estate repo, and continuous evaluation of Effective configs. The danger is
four drifting reimplementations, or walls where warnings belong.

## Decision

1. **One evaluator, exposed as a validation API on the instance.** The rule
   engine lives in one place. Callers: the composer (continuously, as the
   user edits — advisory), the save/render step (enforcing), CI (a thin
   client submitting proposed config for judgement — never a vendored copy
   of the evaluator), and the continuous evaluation loop over Effective
   configs. Vendoring the evaluator into CI is rejected because policy state
   (active catalogue, allow-lists, Grants, floors) lives with the instance;
   a vendored copy would judge with stale policy by construction.
2. **Validation is continuous, not save-triggered.** The API is stateless —
   draft blueprint + context in, findings out — so the composer shows
   findings and palette states live during editing. Save calls the same
   endpoint with enforcement on.
3. **Exactly one rule hard-blocks: an allow-list violation, at render.** A
   Blueprint referencing a component outside the team's effective list does
   not render into the estate repo. This is the one rule with a total
   authority chain and a fast, auditable escape hatch (request a Grant).
4. **Floors and lifecycle never hard-block — they produce findings** with
   mandatory remediation, routed to the object's owner (ADR-0016). Breaches
   have legitimate temporary states (catalogue update downgraded a
   long-running component; exemption pending); blocking would let a routine
   catalogue activation freeze config work, violating ADR-0020 §8.
   Adopter-configurable escalation of findings to blocks is deliberately
   deferred: it tightens rather than loosens, so it can be added later
   without breaking the model.
5. **Palette semantics: show, grey, hide.** Allowed components are shown;
   components breaching the floor for the current context are visible but
   greyed with the reason ("alpha — below this Service's floor");
   non-allowed components are hidden entirely. Greying teaches the policy;
   hiding banned entries avoids noise. The Palette is pure presentation of
   the evaluator's verdicts — it enforces nothing.
6. **Foreign collectors get identical verdicts.** Continuous evaluation
   judges Effective configs by the same rulebook; a Foreign config using a
   non-allowed component receives the same finding a composed one would,
   routed to the same owner. Can't-block ≠ don't-report.

## Consequences

- The validation API is a public contract for CI; it needs versioning, an
  auth story (ADR-0019), and network reachability from CI runners — inside
  the same closed network for air-gapped estates. Validation is unavailable
  when the instance is down; CI checks fail open or closed per repo policy
  (G3 decides the default with the PR workflow).
- The composer's live findings, the save gate, CI annotations and the
  estate view can never disagree — they are the same call.
- Load: the API must tolerate per-keystroke-ish traffic from composers;
  debouncing is a client concern, statelessness the server's.

## Sources

- Session G2; ADR-0016 (routing), ADR-0020, ADR-0021, ADR-0023.
