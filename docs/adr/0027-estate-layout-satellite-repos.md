# ADR-0027: Estate repo layout; satellite repos for private subtrees

- Status: accepted (amends ADR-0018; extends ADR-0019)
- Date: 2026-08-14 (session G3)

## Context

ADR-0018 chose one estate monorepo, path-per-team, and handed the directory
design to G3. The session also stress-tested a real adopter shape: one
central InfoSec function needing estate-wide compliance, plus a small number
of highly sensitive teams wanting their configuration content invisible to
other teams. Instance-per-domain (ADR-0019's isolation answer) silos the
roll-up; the requirement decomposed into **central verdicts, private
content**.

## Decision

### 1. Primary repo layout

```
teams.yaml                      # the Team-tree seam (ADR-0017) — always here
teams/
  <team-id>/
    allowlist.yaml              # the team's narrowing list (ADR-0021)
    grants/<name>.yaml          # grants this team authored for descendants
    components/<name>.yaml      # shared Components → id <team-id>/<name>
    blueprints/<name>.yaml      # may contain local Components (ADR-0024)
    tiers/<name>.yaml           # topology objects the team owns
    services/<name>.yaml
    requirements/<name>.yaml
rendered/
  <team-id>/<tier-name>.yaml    # one artefact per Tier (ADR-0025), platform-written
```

- **Team directories are flat** (`teams/<team-id>/`); the hierarchy lives
  only in `teams.yaml`. Nested paths were rejected: reparenting a team would
  rewrite every path and break every id. Generated CODEOWNERS still includes
  ancestors — it derives from the tree, not the directory shape.
- **`rendered/` is a separate top-level tree**: the one root the stateless
  OpAMP server reads (ADR-0013), a future Cohort target (OQ-1), protected
  wholesale ("humans never commit here"), and a legible authored-vs-generated
  boundary for drift's Intended reading (ADR-0005).
- **Catalogues are never estate-repo content** — they are instance-side
  artefacts with their own import pipeline (ADR-0020).

### 2. Satellite repos (the exception, not the path)

- **Estate = one primary repo + optional satellite repos, each mapped to
  exactly one Team subtree.** The trodden path is the monorepo; a satellite
  is a deliberate exception for teams managing their own components,
  blueprints and tiers as code without other teams seeing them.
- **Governance is never satellite-resident.** A satellite team exists in the
  primary repo's `teams.yaml` like everyone else; the subtree→repo mapping
  is declared centrally; Grants targeting the subtree live with their
  ancestors in the primary repo. Only authored content (and the subtree's
  rendered artefacts) moves out; a satellite's internal layout is identical
  to a monorepo subtree, so promotion back in is a mechanical move.
- **References are one-way: satellite → primary only.** Never primary →
  satellite, never satellite → satellite. A private team's objects can never
  become dependencies of the open estate, so privacy never blocks anyone
  else's render.
- **The server reads all repos' `rendered/` trees** (1 + k credentials, k
  small — ADR-0028's onboarding model). ADR-0018's rejection reasons were
  load-bearing against repo-per-team as *the model* (N repos, N-PR fan-out);
  they are bounded here, and the fan-out inverts: a shared-Component change
  re-renders satellite Tiers as a platform PR *in the satellite repo* —
  exactly the review the sensitive team wants.

### 3. The visibility principle

**Verdicts are estate-public; content may be subtree-private.** Compliance
numbers, finding kinds and object *ids* always roll up (ADR-0017 untouched —
nobody is privately non-compliant; central InfoSec sees the whole estate).
What gates is content: source bodies, rendered YAML, flyouts, diffs —
visible to subtree members only. **UI gating mirrors git reality, never
substitutes for it**: the satellite's forge permissions are the boundary and
the platform enforces the same subtree rule in UI/API. Stated honestly: on a
shared instance the platform's operators see everything; a team whose threat
model includes the operators still needs instance-per-domain (ADR-0019),
which remains the *stronger, distinct* isolation grade. Cross-instance
verdict federation for that case is registered as OQ-17.

## Consequences

- The **source-set abstraction** (estate = set of repos mapped to subtrees;
  single-repo the degenerate case) must be in schema and architecture from
  day one; satellite support itself is implemented in a later build phase
  (noted in `docs/plan.md`).
- Content gating adds object-level read authz to the console — scoped
  strictly to the subtree rule, not a general ACL system.

## Sources

- Session G3; ADR-0017, ADR-0018, ADR-0019, ADR-0021.
