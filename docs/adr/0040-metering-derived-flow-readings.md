# ADR-0040: Metering — derived flow readings, computed on read, stored nowhere

- Status: accepted
- Date: 2026-08-17 (session G6)

## Context

REQ-050 wants per-Tier/per-Hop throughput, volume, and freshness on the
canvas and cards. R-4 constrains what is honestly derivable: helper
metrics are per-component totals — per-`service.name` flow through a Tier
does not exist in self-telemetry. The stateless posture (ADR-0013,
ADR-0032: git-the-tool, loseable caches only) constrains where readings
may live. OQ-12 (evaluator cost at scale) is carried and watching.

## Decision

1. **Two sources, two grains, named as such.** **Pipeline-grain** metering
   comes from self-telemetry: per (Tier, signal) in/out item rates — in =
   receiver-accepted, out = per-exporter sent, summed across instances; a
   **Hop's throughput is its feeding exporter's out-rate**.
   **Service-grain** metering comes from the Observed data itself via
   `TelemetryProvider` (`service.name` splits). The two are never blended
   and per-service flow through a Tier is never faked by division.
2. **Items are the unit** (spans, datapoints, log records — what helper
   metrics emit at level `normal`); bytes appear only where a real reading
   exists, never estimated.
3. **In-minus-out is "reduction", never "loss".** A filter processor
   dropping 90% is doing its job; the meter presents the delta and passes
   no judgement — judgement belongs to claims and Requirements. The only
   reds the meter itself sources are error-rate readings
   (`refused`/`send_failed`/`enqueue_failed`), which feed ADR-0038's
   pipeline claims.
4. **Freshness at both grains**: service-grain = age of newest landed
   record per (Service, Environment, signal); pipeline-grain = age of
   newest self-telemetry reading per Tier, plus **incarnation churn**
   (ADR-0039 §6) as the restart-rate reading.
5. **Computed on read through `TelemetryProvider`; loseable cache at most;
   no metering store.** The scaling argument that makes this tenable:
   query cardinality follows *authored* objects — (Tiers × components ×
   signals), a few hundred aggregations — not collector count; 22,000
   collectors collapse into server-side sums exactly as P3's counts did.
   History and sparklines are range queries against the adopter's backend
   at the adopter's retention — the platform holds no time series. The
   named cost: console latency for cards and canvas is a function of the
   adopter's backend query speed; accepted rather than building a shadow
   metrics store — git-the-tool discipline applied to telemetry.
6. **Metering never invents.** A `TelemetryProvider` that cannot answer a
   metering query declares the incapability (ADR-0036 pattern); readings
   are `Known: false`; surfaces show last-known-plus-age.

## Consequences

- P3's Tier-card counts and P4's per-signal matrix rows are served by
  these readings through the card contract (ADR-0041).
- OQ-12 gains a note: metering inherits the on-read cost ceiling;
  cardinality follows authored objects, not collectors.
- Backends differ in aggregation power; the `TelemetryProvider` contract
  test kit (ADR-0036 pattern) grows metering fixtures.

## Sources

- Session G6; REQ-050; ADR-0013, 0032, 0035, 0036, 0038, 0039;
  `docs/research/2026-08-14-r4-self-telemetry-attributes.md`; P3/P4
  verdicts; OQ-12.
