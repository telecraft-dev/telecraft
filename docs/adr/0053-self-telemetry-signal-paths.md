# ADR-0053: The renderer completes the self-telemetry endpoint per signal

- Status: accepted
- Date: 2026-08-22

## Context

ADR-0039 §2 has the adopter declare one self-telemetry destination,
estate-level, in `telemetry.yaml`, resolved per Tier at render on the Tier's
Environment. The renderer wrote that string verbatim into both blocks of
`service::telemetry`: the metrics reader's exporter and the logs processor's
exporter.

A rendered artefact carries two kinds of OTLP exporter, and they read
`endpoint` differently. The data pipelines' `otlp_http` exporter is the
collector's own, and it treats `endpoint` as a base: it appends `/v1/metrics`,
`/v1/logs` or `/v1/traces` per signal. The exporters under
`service::telemetry` are not the collector's: they are the OTel SDK's
declarative-configuration exporters, and there `endpoint` is the complete URL.
They append nothing.

So one authored endpoint could be correct for at most one signal, and against
any backend with signal-specific intake paths it was correct for neither.
Issue #109 caught it in the devenv (ADR-0052), against Elasticsearch's OTLP
intake, from a collector 0.159.0's own log:

```
failed to send logs to http://elasticsearch:9200/_otlp: 405 Method Not Allowed
failed to upload metrics: failed to send metrics to http://elasticsearch:9200/_otlp: 405
```

Both signals, every collector, continuously, since ADR-0039 shipped. Nothing
self-telemetry-shaped had ever reached a backend, so the ingestion half of
ADR-0039 had never run: the reading → (Tier, SHA) → artefact → claims join of
§5 had no readings to join, and the expectation band sat at `pending_settle`
permanently.

The failure mode is what makes this worth a decision rather than a fix. The
same endpoint string works in one place in the artefact and silently fails in
another, and the only symptom of a wrong self-telemetry destination is an
absence. The estate author has no way to notice, and nothing in the product
was ever going to tell them.

Three answers were live. Per-signal endpoints in `telemetry.yaml` is honest
and widens an authored file for a reason the author cannot be expected to
know. Pointing self-telemetry at a collector rather than a backend is
arguably the intended topology anyway and fixes nothing by itself: an OTLP
receiver serves `/v1/logs`, not `/`, so the same 405 becomes a 404. And the
renderer completing the endpoint, which encodes an SDK convention in the
renderer and makes the rendered endpoint differ from what the author typed.

## Decision

### 1. The renderer appends the signal path

`endpoint` in `telemetry.yaml` is the **base** endpoint, the same string the
data pipelines' `otlp_http` exporter takes. Over `http/protobuf` the renderer
appends `/v1/metrics` in the metrics reader's exporter and `/v1/logs` in the
logs processor's exporter, per artefact, per Environment-resolved
destination. Any trailing slashes on the authored endpoint are dropped first,
so `https://x/otlp/` and `https://x/otlp` render identically.

Encoding the convention in the renderer is what the renderer is for. Its
entire job is turning a domain document into otelcol conventions the author
should not have to hold (pipeline wiring, component ids, the identity
stamps of ADR-0013 and ADR-0039 §5), and the SDK's path semantics are one
more of those. The alternative asks every adopter to know which of two
exporters in one file completes a URL and which does not.

The cost is named and accepted: the rendered endpoint is no longer character
for character what the author typed. That is already true of most of the
artefact, and the rendered YAML is read directly, so the appended path is
visible where the config is.

`telemetry.yaml` does not change. No estate is re-authored, and the field
means what its documentation always claimed it meant.

### 2. An endpoint that already carries a signal path is a load error

An `endpoint`, or any `environments` override, that ends in `/v1/metrics`,
`/v1/logs` or `/v1/traces` fails `telemetry.yaml`'s load, and therefore the
render, naming the fix: declare the base endpoint.

Leaving it alone would push metrics at `…/v1/metrics/v1/metrics`, which fails
the same silent way this decision exists to end. Stripping it would guess at
which signal the author meant on a field that has to serve two. The traces
path is refused with the other two although v1 renders no internal traces
(ADR-0039 §1): an endpoint pointing at it is not a base endpoint either.

Failing loudly at render is the whole point. Self-telemetry is mandatory in
every artefact (ADR-0039 §1), so this refusal blocks the render rather than
degrading it, in the one place the adopter is looking.

### 3. Over grpc, nothing is appended

gRPC OTLP addresses a method on a service. The endpoint is a host and port,
there is no request path to carry a signal, and `/v1/metrics` glued onto it
would name a host that does not exist. A `grpc` destination renders into both
blocks exactly as declared, and §2's refusal still applies to it: a grpc
endpoint carrying a signal path is authored wrong whichever way it is read.

## Consequences

- ADR-0039 §1 and §2 stand unamended in substance. What changes is that the
  destination they declare is now reachable, and the renderer owns one more
  otelcol convention.
- Every rendered artefact in the repository changes on this commit: two
  endpoint lines per artefact, in `devenv/estate/rendered/`, the renderer's
  golden tree and the console's fixture estate. The recompute invariant
  (ADR-0028 §2) makes that mechanical.
- The devenv (ADR-0052) becomes the standing check. It is the only place
  self-telemetry crosses a live collector into a live backend, so a
  regression here shows up as an expectation band that stops settling rather
  than as a silence nobody reads.
- An adopter whose destination is a collector rather than a backend is served
  by the same rule: an OTLP receiver's `/v1/metrics` is what the renderer now
  writes.
- If the SDK's declarative configuration ever gains base-endpoint semantics,
  or the collector switches `service::telemetry` to its own exporters, this
  decision inverts and the renderer stops appending. The signal paths live in
  one map in `internal/renderer/selftelemetry.go` for that reason.

## Sources

- Issue #109, and the collector 0.159.0 log it quotes.
- ADR-0039 §1 and §2, §5 (self-telemetry ingestion, the destination declaration
  and the reading join); ADR-0052 (the devenv, where this was reproducible in
  one command); ADR-0028 §2 (the recompute invariant); ADR-0013 (the artefact
  carries its own identity); ADR-0038 §4 (the settle window that never
  settled).
- `docs/reference/estate-layout.md`, whose `telemetry.yaml` entry described a
  field that could not work as described.
