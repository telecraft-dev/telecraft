# ADR-0030: First boot: the Unmatched artefact; `never_seen` is a Tier-attached finding class

- Status: accepted
- Date: 2026-08-14 (session G4)

## Context

ADR-0010's consequence: first boot with no cache and no local config
yields a healthy collector running a `nop` pipeline (silent nothing),
and the server must never serve an empty config map. OQ-2 asked what the
platform shows for that collector, and whether "expected but never seen"
is a conformance outcome or a separate finding class, given it has no
collector to attach to. ADR-0007 already supplies the shape:
selector-as-expectation.

## Decision

1. **A matched first boot is the ordinary path**: connect → selector
   match → serve the Tier artefact → `APPLYING` → `APPLIED`. Nothing new
   to design.
2. **A served collector matching no selector receives the Unmatched
   artefact**: a distinguished, root-team-owned rendered config at head
   (`rendered/_estate/unmatched.yaml`), commit-stamped, self-telemetry
   on, no data pipelines. Non-empty by construction, honouring ADR-0010
   rule 6. The one case where the platform is actually talking to an
   ungoverned thing is not wasted: the collector becomes maximally
   visible (stamped, health-reporting, labelled governed-by-nobody),
   which is what makes the onboard CTA rich ("alive, version X, on node
   Y, since Tuesday"). Not-knowing is a rendered, visible state, not an
   absence. Naming discipline: this is *not* the quarantine destination.
   That term is reserved for the data-level routing pattern (ADR-0031).
3. **`never_seen` is a separate finding class attached to the Tier**,
   never an eighth conformance outcome. The seven outcomes are
   per-requirement crosses needing an Effective or Observed reading;
   a Tier whose selector has matched nothing has neither: forcing it in
   would repeat one fact across every requirement, noise wearing a
   verdict's clothes. Instead the selector *is* the expectation
   (ADR-0007), and the finding is the selector's: one per Tier, "this
   Tier's selector has matched no collector in any reading". **Neutral in
   v1**: a freshly authored Tier awaiting its DaemonSet is a normal
   Tuesday: visible, excluded from compliance denominators, presented
   like P2's no-verdict states, never red. When G5 lands cardinality
   (OQ-5: the substrate says *N expected*), the same finding class gains
   teeth: "expected 40, seen 0" may then escalate. G4 ships the class;
   G5 ships the count.

## Consequences

- The root team owns one governance artefact by convention; the renderer
  emits it unconditionally.
- The estate view can show unmatched served collectors with live health
  and identity rather than a bare row (feeds ADR-0031's CTA).
- The `never_seen` ↔ cardinality seam is declared for G5; no amendment
  needed when counts arrive.

## Sources

- Session G4; ADR-0007, ADR-0010, ADR-0013; OQ-2, OQ-5; P2 verdict.
