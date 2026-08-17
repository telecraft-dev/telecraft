# ADR-0039: Self-telemetry ingestion — a rendered pattern over the `TelemetryProvider` seam

- Status: accepted
- Date: 2026-08-17 (session G6)

## Context

REQ-053 pins the seam: self-telemetry rides `TelemetryProvider`, no
privileged side channel — the platform reads collector health from the
adopter's backend like any other telemetry. The collector's defaults will
not make that true: R-4 (verified against v0.158.0 source) found the
default surface is a localhost Prometheus pull nobody scrapes, OTLP-out is
opt-in per collector, the new `otelcol.component.*` scheme is
scope-attributes-only and alpha-gated on metrics, and the join keys the
ecosystem actually uses are the legacy datapoint attributes. Separately,
`service.instance.id` rotates per process start, and the Foreign delivery
path has no OpAMP status channel — self-telemetry resource attributes are
the only way the platform learns what a Foreign collector runs.

## Decision

1. **Every rendered artefact configures OTLP-push of internal telemetry**
   — metrics (level `normal`) via a periodic reader, internal logs via a
   batch processor — to the self-telemetry destination. Internal traces
   stay off in v1 (experimental upstream; no claim consumes them). The
   localhost Prometheus default is left untouched: local debugging keeps
   working. This is a **rendered pattern** in the live-check-tap stance
   (ADR-0034): pattern-and-reading, never a platform runtime component.
   The renderer owns the `service::telemetry` block; Blueprints do not
   author it (boundary against ADR-0024's collector-wide section).
   Self-telemetry is **mandatory in every rendered artefact** — ADR-0030
   already made the Unmatched artefact self-telemetry-only, and a governed
   Tier reporting less than the ungoverned fallback would be absurd. The
   ingest volume this creates is a named, accepted cost (metered like
   everything else, ADR-0040).
2. **The self-telemetry destination is declared once, estate-level, by the
   adopter**, resolved per Tier at render, with one routing rule: **a
   Tier's self-telemetry must never depend on that Tier's own data
   pipelines** — own exporter, own connection; a gateway never exports its
   own health through itself. Edge Tiers whose only network path runs
   through a gateway may transit it as ordinary OTLP data; the shared fate
   is topological reality, and its consequence is named: transport loss
   reads as `Known: false` ⇒ `unknown` at the reading layer, and
   pipeline-claim reds are dampened (ADR-0038 §4) so a transport blip
   never paints the estate red.
3. **One platform-owned normalisation layer**, keyed `(kind, id)`, maps
   readings back to the rendered YAML. Legacy datapoint attributes
   (`receiver`/`scraper`/`processor`/`exporter`, full `type/name`) are the
   **primary** join keys for metrics; `otelcol.component.*` scope
   attributes are primary for logs and the alternate for metrics.
   Pipeline membership of receivers and exporters is derived from the
   rendered config topology, never from telemetry (no pipeline id exists
   on those surfaces). `capabilities`/`fanout` synthetic kinds,
   identity-dropping singletons (`otlp` receiver, `memory_limiter`),
   two-level scraper identity, and the `otel.signal` vs `otelcol.signal`
   prefix trap are modelled as expected shapes — R-4's caveats §5.1–5.7
   become the normaliser's test cases.
4. **The `telemetry.newPipelineTelemetry` alpha gate ships off**, behind a
   Telecraft flag mirroring upstream's status (it breaks the default
   Prometheus surface and remains StageAlpha at v0.158.0). Flipping it
   later widens the normaliser's alternate key; it changes no claim
   semantics.
5. **Identity stamping extends ADR-0013.** The renderer stamps
   `service::telemetry::resource` with `telecraft.tier` (the Tier's
   `team/name` id) alongside the existing `telecraft.commit`. That pair is
   the whole join: reading → (Tier, SHA) → artefact → claims. Environment
   is not stamped — the Tier declares it (ADR-0025); two sources of one
   fact is drift. Baked at render, both delivery paths carry identical
   identity, so the Foreign path gains stamp-equivalent observability for
   free. ADR-0013's boundary is intact: these attributes ride the
   collector's *self*-telemetry resource only — nothing is stamped onto
   customer data.
6. **Node-stable identity is a substrate pattern, not a platform key**:
   where the substrate offers it (Kubernetes downward API), the rendered
   pattern interpolates node/pod identity into a resource attribute;
   absence is tolerated — no invented host identity (ADR-0035
   discipline). No claim keys on node identity; metering benefits when
   present. **`service.instance.id` is kept as the incarnation id** — its
   per-restart rotation is signal: incarnation churn per Tier is the
   restart-rate reading.
7. **The stamp is a reading, never a back-door Effective.** Effective
   remains OpAMP's `EffectiveConfig` verbatim (ADR-0004). `telecraft.commit`
   tells the Expectation engine what a Foreign collector runs; a Foreign
   collector's delivery status remains whatever the git path can honestly
   say.

## Consequences

- The normaliser is the single place both join-key generations meet; its
  test kit follows the ADR-0036 conformance-kit pattern.
- Adopters pay real ingest for self-telemetry (per-Tier component × signal
  cardinality, not per-collector explosion — instances aggregate).
- If upstream promotes the new pipeline telemetry, only the Telecraft flag
  default and the normaliser's key preference change.
- REQ-050's parenthetical join keys (`otelcol.component.id`,
  `otelcol.pipeline.id`) are corrected by R-4: real, but scope-attributes
  on logs/traces; metrics join on the legacy attributes today.

## Sources

- Session G6; REQ-050/053;
  `docs/research/2026-08-14-r4-self-telemetry-attributes.md` (v0.158.0
  source-verified); ADR-0002, 0004, 0013, 0024, 0025, 0030, 0034, 0035,
  0036, 0038.
