# ADR-0032: The server's cache inventory is a closed list; git-the-tool, never git-the-service

- Status: accepted
- Date: 2026-08-14 (session G4)

## Context

ADR-0013 said the server stores nothing; OQ-15 was the checkable
remainder: what the serving path may hold, where, for how long — cache,
not record — with ADR-0005's layer-1 digest confirmed loseable. Session
G4 also pressure-tested HA (do scaled-out replicas contend on git?) and
standalone/local operation (can a single instance run without a git
service?).

## Decision

1. **The serving path may hold exactly three things — all rebuildable,
   none durable:**
   1. **Repo snapshot**: the fetched estate repo(s) at last-known head,
      plus the compiled selector index derived from it. Refreshed by poll
      (bounded staleness = the fetch interval, default ~30s) with an
      optional webhook fast-path. Loss = re-fetch. A cache *of* git,
      never a fork of it.
   2. **Per-connection layer-1 digest** (ADR-0005): the raw-bytes digest
      of each connected collector's last-reported effective config, in
      process memory only, dying with the connection or process.
      **Confirmed loseable**: cost of loss is one extra parse per
      collector on reconnect — the ordinary cold-start cost, so restart
      is a non-event by construction.
   3. **Nothing else.** No database, no external cache service, no
      durable per-collector record. Cohort membership is computed per
      connect (ADR-0029 §4); artefact choice is a pure function of
      (head, reported attributes); the commit stamp is read *from* the
      collector (ADR-0013).
   Stated as a testable invariant: any proposed server-side storage must
   be derivable from git plus live connections, or it is a design
   regression requiring an amendment to this ADR.
2. **HA needs no coordination.** Read path: N replicas are N independent
   read-only clones; worst case is two replicas one fetch-interval apart —
   the same bounded staleness a single instance has. Write path (the
   forge-adapter side, not serving): platform-authored branches carry
   deterministic names, and a git ref update is compare-and-swap, so
   racing replicas converge and duplicate proposals are impossible
   (ADR-0029 §8). No leader election, no queue of ours.
3. **The git dependency is git-the-tool, never git-the-service.** A local
   bare repository (`file:///…/estate.git`) fully satisfies ADR-0028's
   git transport floor: the server fetches from it, the render bot pushes
   branches to it, merges are plain `git merge`. That is the "bare git"
   rung of the capability ladder — no change-proposal UI, manual merges,
   attribution unverified — but validation, the render gate, drift and
   serving all hold. A single binary plus a directory is a complete
   standalone instance: the local-development mode and the air-gap
   posture (ADR-0019) are the same shape.

## Consequences

- Server tests assert the closed list (no writes outside the two caches)
  alongside ADR-0010's never-serve-empty rule.
- The fetch interval and webhook fast-path are the only freshness knobs;
  "why is my merge not live yet" has a one-line answer.
- Local mode ships with `telecraft init`-style scaffolding of a bare repo
  as a first-class path, not a development hack.

## Sources

- Session G4; ADR-0005, ADR-0010, ADR-0013, ADR-0019, ADR-0028,
  ADR-0029; OQ-15.
