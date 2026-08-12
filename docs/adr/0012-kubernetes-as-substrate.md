# ADR-0012: Kubernetes is the control plane's substrate; no custom resource per collector

- Status: accepted (seeded)
- Date: 2026-08-12 (decided during prior shaping)

## Context

Governed collectors do not have to live in Kubernetes (51% of collector
estates include VMs, 18% bare metal); only the governance does. Separately,
one-custom-resource-per-collector was disqualified by Kubernetes' own CRD
documentation — not etcd size, but the published criteria (">1000s of
objects", ">10s of requests/sec sustained", "avoid using a Custom Resource as
data storage for monitoring data"), the impossibility of watching external
objects (polling arithmetic fails by an order of magnitude), and write churn
(a 30-second OpAMP heartbeat across 5,000 collectors is ~167 events/sec).

## Decision

- Kubernetes is the control plane's substrate, not the managed population.
- The few **authored** objects (Stages, Hops, Paths — ADR-0007) may be custom
  resources; **per-collector state may not**. If per-collector reads ever need
  a Kubernetes API, the documented alternative is an aggregated API server
  with its own storage.
- The OTel Operator's OpAMP bridge is adoptable as a northbound reporting
  adaptor (an OpAMP *client*; it does not remove the need to build the
  server). Config history is git's job (ADR-0003) — etcd compacts at five
  minutes and nothing in the operator persists a render.

## Sources

- Tickets 07, 17, 20; research dossier `2026-08-04-17-findings.md`.
