# ADR-0014: GitHub is the v1 host; authentication is a GitHub App with attributable actions

- Status: accepted (seeded); amended by ADR-0019: GitHub is the first-party
  forge integration, not an assumption; air-gapped deployments authenticate
  via ADR-0019 and attribute commits from identity claims
- Date: 2026-08-12 (decided during prior shaping)

## Context

Git is the source of truth (ADR-0003); GitHub specifically is the v1 host. A
shared service account writing commits destroys the audit trail, the exact
failure that helped disqualify Backstage's Kubernetes write path (ADR-0006).
Repeating it would be self-inflicted.

## Decision

The platform authenticates to GitHub as a **GitHub App**, not a personal or
shared token. Commits and pull requests are attributable to the human who
acted. The PR is the approval; git history is the audit trail.

Other git hosts are a seam concern for later: nothing in the core may assume
GitHub beyond the host integration boundary.

## Consequences

- The auth model (who edits the library, who assigns a tier, who only looks)
  and its mapping onto GitHub App attribution is designed in session G1
  together with the team hierarchy.

## Sources

- Requirement R6.3 of the compiled shaping requirements; ticket 18.
