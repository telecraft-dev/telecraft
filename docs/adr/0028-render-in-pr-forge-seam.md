# ADR-0028: Render-in-PR; the forge is a seam; repo onboarding by credential; CI fails closed

- Status: accepted (generalises ADR-0014)
- Date: 2026-08-14 (session G3)

## Context

A composer Save must become a reviewed git change (ADR-0003); rendered
artefacts must stay consistent with authored sources; ADR-0016 promised
CODEOWNERS-routed review of cross-team impact; ADR-0022 left CI's
fail-open-or-closed default to G3. And none of it may assume GitHub
(ADR-0019: GitLab, Gitea, bare git across an air gap are all real).

## Decision

1. **Render in the PR.** Save opens a change proposal carrying the authored
   change **plus the bot-refreshed rendered diffs**, re-rendered on every
   push to the branch. Reviewers judge the blast radius — the rendered diff
   is the actual config change reaching collectors. Render-post-merge was
   rejected: reviewers approve blind, consumer CODEOWNERS never fires before
   merge, and a refused render strands main half-applied.
2. **Main is always consistent.** At every commit, `rendered/` is a
   deterministic function of the authored trees; CI recomputes and fails on
   mismatch; humans never commit to `rendered/` (protected path).
3. **Block-at-render ≡ block-at-merge.** An allow-list violation means the
   render refuses, the rendered tree cannot be produced, and the PR is red
   and unmergeable — ADR-0022 §3's one hard block, enforced mechanically.
4. **Ownership truth is in-repo; the forge is a seam.** `teams.yaml` +
   object `owner:` fields are the source; CODEOWNERS files are *generated
   projections* in the configured forge's dialect — caches whose loss loses
   nothing. The **forge adapter** is the domain seam; "GitHub App"
   (ADR-0014) is its first implementation, not the requirement — ADR-0014 is
   restated forge-neutrally: *the platform authors changes through a forge
   adapter with verifiable bot identity where the forge offers one.*
   Capability ladder: full (GitHub/GitLab: proposals, review routing,
   annotations, verified attribution) → partial (Gitea-class) → bare git
   (branch push over SSH, manual merge, git-author attribution unverified —
   validation and the render gate still hold; forge-enforced human review is
   what that adopter forfeited).
5. **Repo onboarding is Argo-style: URL + credential, per repo** — primary
   and each satellite (ADR-0027). Two capability levels: the **git
   transport floor** (SSH deploy key or HTTPS token — clone/fetch/push;
   forge-agnostic, air-gap-safe; sufficient for full governance) and the
   **optional forge-adapter credential** layered on where a forge API
   exists.
6. **CI default: fail closed, with bounded retry.** The validation/render
   check retries with backoff (configurable, sane defaults) to ride out
   transient unavailability; if the instance stays unreachable, PRs into
   estate repos do not merge. Rationale: the renderer *is* the instance — an
   open-fail merge wouldn't just skip policy, it would break invariant §2;
   and the one hard block must not be bypassable by outage timing. The
   availability coupling is stated honestly: platform-path config changes
   stall while the instance is down; the emergency route is the delivery
   paths that never depended on the platform (Foreign-path hand delivery),
   reconciled and reported by drift + continuous evaluation afterwards.
   There is no break-glass edit to `rendered/`.
7. **Fail-open is a per-repo opt-in, never the default**, and carries a
   mandatory companion: a post-recovery **reconciliation sweep** — re-render,
   mismatch repair, retroactive findings for anything merged past a
   would-have-blocked violation. Governance regained retroactively, never
   silently.

## Consequences

- Concurrent PRs conflict in `rendered/`; the bot re-renders on rebase —
  routine bot work, and the PR-volume-at-scale concern stays parked with G4
  (ADR-0018).
- The validation API contract (ADR-0022) gains its CI client semantics:
  versioned, authenticated (ADR-0019), reachable from CI runners inside the
  same network for air-gapped estates.
- Composer saves need a branch-per-draft convention; naming is an
  implementation detail, not a domain concept.

## Sources

- Session G3; ADR-0003, ADR-0014, ADR-0016, ADR-0018, ADR-0022; Argo CD's
  repository-credential model as prior art.
