# ADR-0029: Staged rollout as git state; the Rollout object and Cohorts

- Status: accepted (amends ADR-0025 §1)
- Date: 2026-08-14 (session G4)

## Context

ADR-0013 left staged rollout as "the hard residue": a stateless server
serving one repo path per Tier cannot decide that this collector gets the
new config and that one does not. ADR-0003 required rollout to be
expressible as git state or not at all; ADR-0025 made the artefact
inventory enumerable (one rendered artefact per Tier). OQ-1, the largest
undesigned piece, asked what a Cohort is, who computes membership, what a
rollout looks like as PRs, what halts it, and what the Foreign population
gets. Tier/Environment promotion (prod binds v4 while staging trials v5,
ADR-0025 §3) is a separate, already-solved mechanism this ADR does not
touch.

## Decision

1. **The default is the flat rebind.** Rebinding a Tier (v4 → v5) is one
   PR; every collector in the Tier picks the new artefact up on next poll.
   A Rollout is the *opt-in* instrument for staging, never mandatory
   ceremony.
2. **A Rollout is an authored, owned object** (owner = the Tier's owner,
   at `teams/<team-id>/rollouts/<name>.yaml`) targeting exactly one Tier:
   a *from* binding, a *to* binding, and ordered stages, each stage a
   Cohort spec plus exit criteria (minimum soak, health conditions).
   **One active Rollout per Tier**; while it is active, a direct rebind PR
   fails render validation: the Rollout file is the only door.
   *Amendment to ADR-0025 §1*: a Tier binds exactly one Blueprint version,
   or exactly two, *from* and *to*, while an owned Rollout is active;
   never more.
3. **Both artefacts live in `rendered/` at head** (`<tier>.yaml` and
   `<tier>@next.yaml`). ADR-0028 §2 survives mechanically: the Rollout
   spec is an authored input to the render function, so the dual artefacts
   are its deterministic output. Everything the server needs is at head:
   no commit-pinning (git history as hot serving state), no rollout
   branches (fragmented review, "main is consistent" suspended).
4. **Cohorts subdivide a single Tier's population**, never a Tier itself
   (a Tier is a policy position, not a rollout wave). Three spec forms,
   mixable per stage: **enumerated hosts** (a selector enumerating
   identifying-attribute values: "the three boxes I trust"), **attribute
   selector** (`region: eu-west-1`), and **fractional** (`percent: 5` via
   a stable hash of the same identifying-attribute set used for Tier
   matching (node-stable, never `instance_uid`), so widening 5 → 50 is a
   strict superset and no collector flaps backwards). Membership is a
   **pure function evaluated by the server per connect**; the same
   function ships as a library so CI and the console can *preview*
   membership against the current estate snapshot: information for the
   reviewer, never the authoritative decision. Accepted openly: a
   fractional cohort is statistically 5%, not exactly 5%. Rejected:
   materialised membership lists in git (requires a roster the platform
   does not hold; stale on every autoscale; unreviewable noise).
5. **Stages advance by platform-proposed, human-merged PRs.** When a
   stage's exit criteria are met, the platform opens the advance PR
   through the forge adapter, bot-attributed (ADR-0028), evidence in the
   body ("soaked 24h, 213/213 APPLIED, 0 FAILED"). A human merges. The
   final advance completes the Rollout: Tier flipped to single-bound *to*,
   Rollout closed, `@next` artefact retired. Manual advancement needs no
   design: it is editing your own file.
6. **Halting is passive; aborting is a proposal.** If criteria fail, the
   advance is simply never proposed. Nothing to race, no control loop:
   collectors that individually broke have already self-reverted
   (`automatic_config_rollback`, ADR-0010) and report `FAILED`. v1 halt
   signals, cohort-scoped: (a) a cohort member reporting `FAILED` for the
   *to* artefact's hash; (b) went-dark-after-apply: reporting, took the
   new config, silent within the soak window (the crash-loop signature
   that never reports `FAILED`). The halt-condition set is explicitly
   extensible: G6 expectation regressions ("applied fine, traces stopped")
   plug in without amendment. Past a configurable threshold (default:
   any cohort `FAILED` blocks advance; ≥10% failed-or-dark proposes abort),
   the platform opens an **abort PR** reverting the Tier to
   single-bound *from*.
7. **The Foreign population reads everything, blocks nothing.** Same git
   state (both artefacts addressable at head; the adopter's tooling maps
   them to hosts however it likes); advisory membership (the same cohort
   function over reported attributes, crossed with the commit stamp,
   ADR-0013) rendered as **lag, never failure**: foreign delivery timing
   was never ours. Advance evidence is computed over collectors *actually
   running the `to` artefact* (either path, per the stamp); foreign
   members still on *from* are displayed but never block. Where delivery
   status is permanently `UNSET` (ElasticFleet, ADR-0008) the `FAILED`
   signal is honestly unavailable; went-dark and Observed-side signals
   work identically.
8. **HA writers coordinate through git, not a leader.** Platform-authored
   branches have deterministic names
   (`telecraft/rollout/<team>/<name>/advance-<stage>`); a git ref update
   is compare-and-swap, so racing replicas converge (loser sees the branch
   exists, no-ops) and duplicate proposals are structurally impossible.
   The PR-volume-at-scale concern parked by ADR-0018/0028 lands here:
   bot traffic is real on a wide estate; mitigation is per-Tier path
   disjointness, routine bot rebase of `rendered/`, and the adopter's own
   forge merge queue: the platform adds no serialisation of its own.

## Consequences

- Cohort loses its ⚠ in the glossary; Rollout enters it. The
  cohort-as-git-state hypothesis is confirmed: every step (start,
  advance, halt-evidence, abort, completion) is a commit on one small
  file plus deterministic rendered output.
- P5 (rollout cohort progress prototype) remains optional per
  `docs/plan.md`; the rollout ledger view consumes the same one-model
  discipline as P3's surfaces.
- The membership function's attribute set must be pinned and versioned
  with the Rollout schema: changing it mid-rollout would reshuffle
  fractional cohorts.

## Sources

- Session G4; ADR-0003, ADR-0005, ADR-0008, ADR-0010, ADR-0013,
  ADR-0025, ADR-0027, ADR-0028; OQ-1.
