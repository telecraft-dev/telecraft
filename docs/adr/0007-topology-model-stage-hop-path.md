# ADR-0007: The topology model (Stage, Hop, Path); collectors are never drawn

- Status: accepted (seeded)
- Vocabulary note: written pre-ADR-0015: read Stage as Tier, Criticality Tier as Service Class, Classification as Sensitivity, Declared as Effective, Application as Service
- Date: 2026-08-12 (decided during prior shaping)

## Context

Topology canvases fail at estate scale: gateway-plus-many-collectors becomes a
hairball at 50 collectors, unusable at 500. Separately, a survey of eight
graphical tools found nobody authors config by manipulating a topology graph,
and the tier boundary is universally a string (an endpoint typed into an
exporter, a port typed into a receiver), never an object the two ends share.
Holding the boundary as an object *is* established practice in Kubernetes
operators (kube-logging's `LoggingRoute` is the closest prior art); reconciling
it against delivered telemetry is not.

## Decision

Authored objects:

- **Stage**: a position in the pipeline (edge, gateway, any tier a design
  needs). There will never be many. Carries the policy that applies to
  everything at that position.
- **Hop**: the directed edge between two Stages (or Stage to destination). A
  first-class object; the design's central bet. Trust is a property of the
  Hop, not the Stage (one gateway receives both trusted and untrusted
  traffic); attributes crossing an untrusted Hop are stripped and re-derived,
  and the renderer generates that automatically. The Hop schema starts from
  `LoggingRoute` (Apache-2.0), not from scratch.
- **Path**: one application's route through the Stage graph. Multiple Paths
  per application is normal (browser traffic via gateway on-ramp *and* pod
  logs via edge DaemonSet). A Path generates the delivery expectation.
- **Criticality Tier ⊥ Classification**: two orthogonal first-class axes,
  both carrying the adopter's own names and values. Criticality drives
  requirements and rendered collection; Classification drives routing and
  redaction. Never conflated. "Tier" means Criticality Tier and nothing else.

Derived, read-only: **Collector**. A running process is never drawn on the
canvas: it connects, is matched into a Stage by selector, and inherits that
Stage's policy. This is what keeps the authored graph at a handful of nodes
regardless of estate size. A workload that runs no collector at all (gateway
on-ramp emitter) is representable as a Path straight to a gateway Stage.

Delivery path (served vs git-delivered) is a **visible property of a
collector**, because the two have different remedies.

## Consequences

- Anything requiring per-collector canvas nodes contradicts the scale
  argument and is rejected at design time.
- Positioning must not claim novelty for holding the boundary as an object,
  only for reconciling it against delivered telemetry.

## Sources

- Tickets 07, 14, 04, 17, 21.
