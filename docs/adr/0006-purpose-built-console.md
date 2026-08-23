# ADR-0006: A small purpose-built console; Backstage rejected with the door held open

- Status: accepted (seeded)
- Date: 2026-08-12 (decided during prior shaping)

## Context

Backstage was evaluated as the primary authoring surface and rejected on
verified grounds: its catalogue is read-and-browse by design (no POST/PUT
creates an entity; relations are output-only, so a first-class Hop has no
home); every line of a graph editor would be ours anyway (it reuses the shell,
not the catalogue); the adopter cost is an internal developer portal staffed
indefinitely (vendor sizing: 3 FTE for 6 to 12 months to production); and its
Kubernetes write path attributes every write to the Backstage service account,
an audit-trail regression for a product whose pitch includes an authoritative
audit trail.

## Decision

The authoring surface is a **small purpose-built console**. Three constraints
(good design regardless) keep the Backstage door open at zero cost:

1. Intent lives in custom resources (authored objects only; see ADR-0012).
2. The console sits on a documented API.
3. Nothing assumes Backstage identity or entity refs.

If a second surface is ever wanted, price Headlamp first (kubernetes-sigs;
runtime plugins, writes run as the signed-in user so RBAC and audit are the
cluster's). Any Backstage plugin is read-only: an entity provider mirroring
Applications, Stages and Paths plus a conformance card.

## Consequences

- The console owns its own UI stack, chosen fresh in session G7, not
  inherited from the prior Preact prototype console.
- The documented API is a Phase 2 deliverable, not an afterthought.

## Sources

- Ticket 18 (`shaping-tickets/18-backstage-as-surface-and-register.md`) and
  research dossier `2026-08-04-18-findings.md`.
