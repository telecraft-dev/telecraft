# ADR-0035: `InventoryProvider` floors; `never_seen` teeth and `under_populated`

- Status: accepted
- Date: 2026-08-14 (session G5)

## Context

A Tier's selector says what shape should match, never how many; "team
running N servers must run these modules" (REQ-012) needs a count from
somewhere. The somewheres differ per substrate (OQ-5): the Kubernetes API
can answer, a VM inventory might, bare metal may have nothing, and 51% of
collector estates include VMs, 18% bare metal (ADR-0012), so Kubernetes
cannot be assumed. ADR-0030 shipped `never_seen` neutral and promised G5
the cardinality teeth. The design worry raised in-session: most
environments cannot state a fixed N at all, especially under dynamic
scaling. An exact-count expectation would rot the moment an autoscaler
breathes.

## Decision

1. **A new, deliberately narrow seam: `InventoryProvider`.** One question:
   given this Tier's selector, how many instances should match. A count
   plus an as-of timestamp, `Known: false` when it cannot say (ADR-0008
   discipline). Separate from `EstateProvider` by design: the estate seam
   reads the population that exists (keyed on the collector); this seam
   reads what should exist, from the substrate (K8s API, CMDB, cloud
   inventory). Different source, different auth, different deployment
   shape. First-party implementation: `Kubernetes`, answering live from
   the API ("how many nodes match this Tier's workload selector right
   now"), so the expectation floats with the autoscaler by construction.
2. **Expectations are floors, never equalities.** The declared form is
   `min_expected: N` on the Tier, reviewable in git, for substrates with
   no API ("at least 12 boxes in that rack"). The only finding is a
   shortfall; surplus is never a finding. Source ranking per Tier:
   **derived** (live, floats) > **declared** (static floor) > **absent** (no
   provider, no declaration, no teeth): `never_seen` keeps its v1
   neutrality and nobody is forced to guess. The platform never invents a
   count. When both sources exist they are compared, not silently
   resolved: a declared floor above live reality usually means a shrunk
   fleet someone should notice, a visible finding.
3. **Shortfall findings are persistence-dampened.** Scale events have an
   honest transient (nodes joined, DaemonSet pods still scheduling) where
   seen < expected for minutes; a shortfall must persist beyond a
   configurable grace window (order of minutes) before any finding raises.
4. **`never_seen` keeps its exact G4 meaning and gains one escalation
   rule**: floor > 0 and zero matches persisting past the window →
   escalates from neutral to violation-grade ("expected ≥40, seen 0").
   With no floor, neutrality is untouched: a freshly authored Tier
   awaiting its DaemonSet remains a normal Tuesday.
5. **`under_populated` is a new sibling finding class**, not a degree of
   `never_seen`: collectors present but below the floor, persisting
   ("expected ≥40, seen 12"). The Tier has readings, health and delivery
   status: conflating the two would make "12 of 40 running" read as
   "nothing exists", misdescribing both.
6. **Both classes are Tier-attached, routed to the Tier's owner, and join
   the delivery finding kind** in ADR-0017's roll-up: a population
   shortfall is a delivery problem (config never reached machines that
   should have it), never a conformance problem with any Service.
   Escalated findings enter the denominator; un-toothed neutral states
   stay excluded (P2's rule). Exemptions apply as everywhere: a planned
   authoring-to-deployment gap is what an owned, expiring Exemption is
   for.
7. **Age makes the neutral case useful too**: a `never_seen` that has
   persisted for a long time (surface shows its age, "never matched in
   90 days") is the platform's stale-config signal: authored Tiers that
   were never used, candidates for deletion. A presentation affordance,
   not a new finding class.

## Consequences

- The seam count grows to five (`EstateProvider`, `TelemetryProvider`,
  `RegistryProvider`, forge adapter, `InventoryProvider`); the new seam
  needs the same contract-test discipline OQ-6 demands of `EstateProvider`.
- The `Kubernetes` implementation needs API access scoped to node/workload
  reads: deployment documentation, not model surface.
- P4 cards and the estate views gain "seen / floor" counts on Tier cards
  (P3 already put counts there); the G7 flat list can filter on both new
  classes.
- ADR-0030's declared seam ("G4 ships the class, G5 ships the count") is
  fulfilled with no amendment to 0030 needed.

## Sources

- Session G5; OQ-5; ADR-0008, ADR-0012, ADR-0017, ADR-0030; REQ-012; P2/P3
  verdicts.
