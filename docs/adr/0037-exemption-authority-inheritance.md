# ADR-0037: Exemption authority via requirement-owner review; subtree scope

- Status: accepted
- Date: 2026-08-14 (session G5)

## Context

ADR-0017 parked exemption authority within the team tree for G5. An
Exemption is a loosening, and every loosening mechanism so far obeys a
direction: allow-lists narrow down the tree; only ancestor-authored Grants
widen (ADR-0021). Unilateral self-waiver undermines the requirement's
author; a central approval committee rebuilds the ticket queue the product
exists to kill.

## Decision

1. **An Exemption is an ordinary authored, git-resident object** (mandatory
   owner and expiry stand, REQ-014). Its validity rule: the PR introducing
   it must be approved by the **owner of the Requirement being waived** (or
   that owner's ancestor team). Mechanically free: generated forge
   code-ownership (ADR-0014, REQ-017) routes exemption files touching
   requirement R to R's owning team. Self-forgiveness is impossible by
   construction, not by policy prose: no approval workflow is built; the
   forge already has one. On forges without code-ownership the platform
   merge gate (REQ-017's fallback) enforces the same rule.
2. **Subject scope: one object or one Team subtree, always exactly one
   Requirement per Exemption** (glossary rule stands). Subtree scope is the
   onboarding case: "everything under `payments/` waived from the
   completeness requirement until March 1" is one reviewable file, not
   forty copies. No narrowing semantics exist or are needed: an exemption
   waives the count, it never forbids complying.
3. **Renewal is a fresh PR.** Expiry means the file stops counting;
   extension re-triggers the same review. An expired Exemption left in the
   tree is an authoring finding: dead config, same spirit as the aged
   `never_seen` signal (ADR-0035 §7).
4. **Visibility is unchanged and remains the real safeguard**: waived
   counts ride every roll-up level (ADR-0017). An exemption-heavy subtree
   cannot look clean. Authority controls who may loosen; visibility
   ensures loosening never hides.

## Consequences

- "An Exemption is to Requirements what a Grant is to Allow-lists": the
  loosening mechanisms are now symmetric, both ancestor/owner-gated, both
  git-resident, both visible.
- Code-ownership generation must map exemption files to the waived
  requirement's owning team, a renderer/layout concern for the estate
  repo (extends ADR-0018/0027 generated ownership).
- Grace Periods (class-scoped onboarding windows) are untouched: they are
  platform-applied windows, not authored waivers; the two remain distinct.

## Sources

- Session G5; ADR-0014, ADR-0016, ADR-0017, ADR-0018, ADR-0021, ADR-0027;
  REQ-014, REQ-017.
