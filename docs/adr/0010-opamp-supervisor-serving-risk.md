# ADR-0010: Serving uses the upstream OpAMP Supervisor; alpha risk accepted; renderer hard rules

- Status: accepted (seeded)
- Date: 2026-08-12 (decided during prior shaping)

## Context

The in-process `opamp` extension is report-only: it cannot accept remote
config. Applying remote config requires the OpAMP Supervisor beside every
served collector, a second process that takes over the collector process,
which makes serving a migration, not an addition. The Supervisor has been
alpha since creation with no graduation criteria and a five-person bench; its
packaging for VMs and Windows is genuinely mature (signed packages, systemd
units, MSI, biweekly train), while Kubernetes has nothing upstream and a
sidecar is structurally unsupported (the Supervisor forks and signals its
child). The vendor market is split: two vendors require the upstream
Supervisor in production; six built embedded alternatives.

## Decision

- The serving path requires the upstream OpAMP Supervisor beside every served
  collector. The alpha dependency is **accepted with eyes open**; the largest
  unknown (long-running stability at scale) is unresolved in either direction.
- **The platform serves everything; GitOps is an alternative, not a
  fallback.** One artefact, two delivery paths, chosen per collector by the
  adopter. Mixed substrates are the default mode (half of large fleets span
  Kubernetes and VMs).
- Renderer and install-guidance hard rules:
  1. Additional OpAMP extensions are named `opamp/<something>`, never bare
     `opamp`: a bare `opamp` block silently overrides the Supervisor's
     injected endpoint.
  2. The renderer emits a node-unique identifying attribute via the Kubernetes
     Downward API: a DaemonSet renders one manifest for all nodes.
  3. Supervisor storage goes on a durable volume; an ephemeral directory mints
     a new identity per pod replacement.
  4. `automatic_config_rollback: true` in guidance: revert-on-failure is off
     by default upstream.
  5. `accepts_remote_config` capabilities are off by default upstream and must
     be explicitly enabled in guidance.
  6. **The server must never serve an empty config map**: the Supervisor
     then reports APPLIED-and-healthy while running nothing. This belongs in
     the server's tests.

## Consequences

- First boot with no cache and no local config yields a healthy collector
  running a `nop` pipeline: silent nothing. What the platform shows for that
  collector is open (OQ-2) and drove the register decision (ADR-0007's
  selector-as-expectation).
- Coexistence with a second reporting extension (e.g. ElasticFleet) is proven
  live but rests on behaviour the OpAMP spec does not sanction; re-check on
  collector upgrades. Pin `instance_uid` explicitly or every config push
  enrols a phantom agent.

## Sources

- Tickets 01, 06, 21, 23; research dossier `2026-08-04-23-findings.md`.
