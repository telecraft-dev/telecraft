# ADR-0031: Ungoverned in the estate view: two referents, both drain by onboarding

- Status: accepted (ratifies the G2 early verdicts on OQ-3)
- Date: 2026-08-14 (session G4)

## Context

OQ-3: a discovered collector matching no selector is the ungoverned
quadrant. Does it appear in the estate view, and how, without reading as
failure? G2 banked two early verdicts; P2 settled denominator discipline;
P3 settled placement. G4's job was ratify-and-sharpen. The sharpening:
"ungoverned" names two different facts, and conflating them would misroute
remediation.

## Decision

1. **Two referents, named apart.**
   - **Ungoverned collector** (population-level): a discovered collector
     matching no Tier selector, served (it runs the Unmatched artefact,
     ADR-0030, so it is stamped and health-visible) or foreign (read via
     `EstateProvider`, e.g. an ElasticFleet-managed collector nobody
     authored a Tier for).
   - **Ungoverned data** (signal-level): telemetry from unrecognised
     `service.name`s arriving through *governed* pipelines.
2. **Ungoverned collectors appear in the estate view** with an explicit
   **onboard-me CTA**, excluded from compliance denominators: concern,
   never failure. Placement per the P3 verdict: the dedicated band above
   governed Tiers. **Foreign-but-governed stays fully legitimate**: no
   stigma attaches to the delivery path, only to matching no selector.
3. **Ungoverned data is handled by the quarantine pattern**: gateway
   Blueprints may include an authored routing rule sending telemetry from
   unrecognised `service.name`s to a short-retention **quarantine
   destination**, which the platform observes and flags ("unknown sources
   arriving: onboard them"). A rendered governance pattern, never a
   platform runtime capability (ADR-0002: the platform is not in the data
   path). It drains by onboarding, never by retention growth.
4. **Both referents drain the same way**: someone authors or widens a
   selector, or registers the Service, which is exactly what the CTA
   proposes. How the CTA's authoring flow looks is G7's problem; this ADR
   stops at the model.

## Consequences

- The estate view's denominator discipline (P2) is unchanged: ungoverned
  and no-verdict states never dilute or inflate compliance.
- The quarantine destination needs a documented reference pattern (routing
  rule + retention guidance) in the Blueprint library, not platform code.
- Glossary gains the referent split; "ungoverned" bare is only usable
  where context fixes the referent.

## Sources

- Session G4; G2 early verdicts (OQ-3 register notes); ADR-0002,
  ADR-0008, ADR-0030; P2 and P3 verdicts.
