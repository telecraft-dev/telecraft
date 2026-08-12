# ADR-0011: No Helm or Kustomize rendering in v1

- Status: accepted (seeded)
- Date: 2026-08-12 (decided during prior shaping)

## Context

Reading configs that live inside Helm charts requires rendering them. Priced
under the reuse rule: Argo CD's repo-server is gRPC-only, unauthenticated, and
needs 37 Kubernetes replace directives to build against — a dependency Argo
maintainers' own adjacent projects refused in writing. The build-it fallback
(Helm Go SDK + Kustomize) was sized at 1,500–3,000 lines of production
service, mostly git/auth/values plumbing.

## Decision

No Helm or Kustomize rendering in v1. The renderer reads and writes plain
otelcol YAML at a stable path (ADR-0002).

**Known cost, accepted and stated so it is not rediscovered**: Helm is at 77%
production use; an adopter whose collector config lives inside a chart loses
the intended-versus-declared comparison. Their declared and observed readings
still work.

Kept on the table, deliberately not built: talking gRPC to an adopter's
*existing* Argo repo-server for rendered manifests (additive, Argo-users only,
inherits an unauthenticated internal API). Never ship as an Argo CD config
management plugin — a CMP cannot be read-only, and a blank render with
`prune: true` deletes resources.

## Sources

- Tickets 09, 20; research dossiers `2026-08-04-20-findings.md`.
