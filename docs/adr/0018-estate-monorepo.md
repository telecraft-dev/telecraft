# ADR-0018: One estate monorepo, path-per-team

- Status: accepted
- Date: 2026-08-12 (session G1)

## Context

Rendered artefacts and authored sources must live somewhere with team
boundaries (OQ-8), and the Component-inheritance model (ADR-0016) makes the
choice load-bearing: cross-team references are either in-repo paths or
cross-repo plumbing.

## Decision

All authored sources (Components, Blueprints, topology, requirements,
`teams.yaml`) and all rendered artefacts live in **one estate repository**,
with team boundaries as directories. Stated as the v1 default, not a
forever-rule.

Why this way:

- **Review routing comes free**: ownership metadata generates per-path code
  ownership (ADR-0019), so a PR whose render touches Infosec's Component
  requires Infosec review — the forge does the routing.
- Cross-team Component inheritance is an in-repo reference; the stateless
  OpAMP server reads one repository; a rollout Cohort (OQ-1) can be a
  directory.
- Repo-per-team was rejected: the renderer would read N repos to produce one
  artefact, an owning team's change fans out as N PRs, the server needs N
  credentials, and no forge's review routing spans repos — we would rebuild
  it ourselves.

**Visibility cost, accepted knowingly**: all teams can read each other's
configuration. The session's own reasoning: hiding a Component's source is
pointless when its content renders into every consumer's config anyway — and
credentials never live in components at all (rendered YAML carries
`${env:...}` indirection; structure is visible, secrets are not). An adopter
needing hard read isolation runs one platform instance per isolation domain,
which is cleaner than multi-repo plumbing.

## Consequences

- G3 designs the directory layout inside this decision.
- PR volume at large-estate scale is a G4/rollout concern, watched there.
