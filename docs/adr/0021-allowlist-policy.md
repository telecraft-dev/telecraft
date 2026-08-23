# ADR-0021: Allow-list policy: narrowing inheritance with owned Grants

- Status: accepted
- Date: 2026-08-13 (session G2)

## Context

The Catalogue states what exists (ADR-0020); the Allow-list states what a
Team may use. G1 fixed the Team structure as a strict tree (ADR-0017) with
ownership on every authored object (ADR-0016). This ADR pins how allow-lists
scope, compose down the tree, and where they live.

## Decision

1. **Unit: the component, `(class, type)`.** Signal-level nuance is not the
   Allow-list's job; per-signal judgement lives in the stability floors
   (ADR-0023), which evaluate each (component, signal) a Blueprint actually
   uses. Per-signal allow entries are a possible future extension, adopted
   only when a real adopter produces a case floors cannot cover.
2. **Narrowing-only inheritance.** A Team's effective allow-list is its
   parent's effective list intersected with its own declared list (if any).
   Descendants can only subtract. Additions are an organisational
   conversation with the ancestor who owns the wider list, by design.
3. **Grants: parent-authored scoped exceptions.** An ancestor may attach a
   Grant adding named Catalogue entries to a specific descendant Team's
   effective list. A Grant is an authored object (ADR-0016): owned,
   versioned, reviewable. It applies to the target Team's subtree and can be
   narrowed back out below like anything else. Everything a team may use
   therefore traces to either the root list surviving intersection, or a
   named Grant: the audit chain is total.
4. **Default posture: allow.** Absent any authored list, the effective list
   is the whole active Catalogue. Governance pressure comes from floors,
   lifecycle findings and conformance, not from an empty-by-default shop.
   Deprecated/unmaintained components remain in the default-allow: they
   produce findings, never day-one bans (a newly activated catalogue must
   not break re-renders, ADR-0020 §8). An instance-level default-deny
   toggle (synthesising an empty root list) is noted for hardened adopters,
   not built in v1.
5. **Allow-lists and Grants are authored files in the estate repo**:
   git source of truth (ADR-0003), reviewed and versioned like `teams.yaml`.
   The instance reads them; the evaluator judges with them. They are never
   rows edited live in the instance database.

## Consequences

- "May team T use component X?" is answerable, and attributable, entirely
  from git history.
- The Palette (ADR-0022) is derivable per user: Catalogue ∩ effective
  allow-list.
- The allow-list check is the single hard-blocking rule at render
  (ADR-0022); its escape hatch is a Grant, fast and auditable, so no
  break-glass override exists.

## Sources

- Session G2; ADR-0016, ADR-0017 (structure); ADR-0003 (git residency).
