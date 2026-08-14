# ADR-0016: Universal ownership on authored objects; the Component as a first-class unit

- Status: accepted
- Date: 2026-08-12 (session G1)

## Context

Compliance must be attributable: "a team running these servers must run these
modules" only works if every finding lands on exactly one accountable party.
The session established that ownership in real estates is finer-grained than
"a team owns a collector": a gateway collector run by the data-flow team can
contain a PII-redaction processor that Infosec governs, and an exporter whose
endpoint and auth are the gateway team's concern even when it renders into a
downstream team's config.

## Decision

1. **Every authored object carries an owner** — CODEOWNERS-style. The
   authored set: **Component, Blueprint, Tier (topology position), Hop, Path,
   Service, Requirement, Exemption** (which already carried a mandatory
   owner). Ownership is an attribute, not a parallel hierarchy.
2. **The Component is a first-class object**: a configured instance of a
   catalogue type (receiver, processor, exporter, connector, extension),
   named, versioned, and ownable. A **Blueprint is a composition of
   Components**, and those Components may belong to different teams.
3. **Inheritance is by reference, never by copy.** A downstream blueprint
   references the gateway team's exporter Component; when the owning team
   changes it, every consumer re-renders with the change. (Prior art:
   Grafana Fleet Management's server-side merge; Bindplane's Library — whose
   lack of parameterisation is its most-cited gap.)
4. **Findings route to the owner of the object the finding is about**, not
   the owner of the file it renders into: a broken PII processor pages
   Infosec; a dead exporter pages the data-flow team; an unmet Service floor
   pages the Service's owner.
5. **Collectors are not ownable.** A running collector inherits owner,
   policy and obligations from the Tier it matched into by selector. Where
   one subset of a Tier's collectors needs a different owner, the selector
   mechanism already expresses it: split the Tier. This preserves the
   never-draw-collectors scale rule (ADR-0007) and the per-collector-state
   prohibition (ADR-0012).

## Consequences

- The rendered artefact for any collector is multi-owner by construction;
  review routing (e.g. GitHub CODEOWNERS on rendered paths and on
  component/blueprint sources) can enforce that a change to an Infosec-owned
  Component requires Infosec review — designed in G3.
- Owner/Team *hierarchy* semantics (nesting, roll-up, exemption authority,
  tenancy-to-git) remain open in G1 — this ADR fixes what is ownable, not
  how owners aggregate.
- Component versioning and its interaction with blueprint versioning is a G3
  question (OQ-10).
