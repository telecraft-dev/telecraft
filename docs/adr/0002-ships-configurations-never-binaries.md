# ADR-0002: Ships configurations, never binaries; exactly one artefact

- Status: accepted (seeded)
- Date: 2026-08-12 (decided during prior shaping)

## Context

A control plane that distributes binaries inherits a registry, CVE patching,
and a version matrix against the collector's biweekly release train. A proposal
to publish a Supervisor-plus-collector container image was examined and
rejected during shaping. Independently, a survey of every working delivery
target (GitOps controllers, AWS SSM, config management, cloud OS policy)
found that all of them accept an opaque file and none want to understand the
YAML, so applier-agnosticism costs nothing.

## Decision

- No component of this platform ever sits in the telemetry path. If the
  platform is down, no telemetry stops flowing.
- The platform ships configurations, never binaries: no collector
  distribution, no gateway, no container image, no Helm chart, no rendered
  DaemonSet. How a binary reaches a host is documented, never owned.
- The renderer exports **exactly one artefact**: plain otelcol YAML at a
  stable repository path, plus a ~25-line `supervisor.yaml` where the serving
  path (ADR-0013) is used.
- The renderer never knows what applies the result: a GitOps controller, Helm,
  Ansible, SSM, a person, or the platform's own OpAMP server.

## Consequences

- Adoption cost stays low and the last surviving non-goal, "nothing in the
  telemetry path", carries the adoption argument.
- Kubernetes adopters who want serving must supply their own
  Supervisor-plus-collector image (upstream ships none; see ADR-0010).

## Sources

- Shaping premises 3, 11; tickets 09, 21
  (`shaping-tickets/09-write-path-how-config-reaches-collectors.md`,
  `shaping-tickets/21-supervisor-adoption-cost.md`).
