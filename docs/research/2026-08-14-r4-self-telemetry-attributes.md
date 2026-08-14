# R-4: Collector self-telemetry attribute spelling vs current release

- Date: 2026-08-14 (feeding session G6, REQ-050/053)
- Method: shallow clone of `opentelemetry-collector` (core) at tag
  **v0.158.0** (commit `0378e9a`, 2026-08-04); grep of the checked-out source;
  the opentelemetry.io internal-telemetry docs source at `main` (fetched
  2026-08-14); GitHub API for releases; code search of
  `open-telemetry/semantic-conventions`. Where docs and code disagree, the
  **code at the release tag** was trusted.

## TL;DR verdict

**The drafted spellings are correct — `otelcol.component.id`,
`otelcol.component.kind`, `otelcol.pipeline.id` are exactly what the current
release emits — but they are *instrumentation scope attributes*, they are
default-on only for internal **logs and traces**, and on **metrics** they sit
behind an alpha feature gate that is still off at v0.158.0.** The full set is
five keys, defined in `internal/telemetry/attribute.go:12-16`:

```go
ComponentKindKey = "otelcol.component.kind"
ComponentIDKey   = "otelcol.component.id"
PipelineIDKey    = "otelcol.pipeline.id"
SignalKey        = "otelcol.signal"
SignalOutputKey  = "otelcol.signal.output"
```

What G6 can rely on **today, with a default config**:

- **Logs/traces → component join**: the scope attributes above, on by default
  since v0.125.0 (no gate). `otelcol.component.id` carries the component ID
  **exactly as written in the rendered YAML** (`type` or `type/name`, via
  `component.ID.String()`).
- **Metrics → component join**: the *legacy* datapoint attributes on the
  helper metrics — `receiver`, `scraper`, `processor`, `exporter` (each
  holding the full `type/name` id), plus `transport`, `format`, `otel.signal`
  (processors), `data_type` (exporters). These are default-on and what every
  dashboard in the wild joins on.
- **Not reliable yet**: `otelcol.component.*` on metrics
  (`telemetry.newPipelineTelemetry`, **alpha, default off**, and known to
  break the default Prometheus surface), and the new dotted metrics
  (`otelcol.receiver.produced.items` etc., **stability: development**, only
  emitted when that gate is on).
- **`otelcol.pipeline.id` is NOT on receivers or exporters** — only
  processors carry it (receivers/exporters are shared across same-signal
  pipelines). Pipeline membership for receivers/exporters must be derived
  from the rendered config topology, not from telemetry. The G6 draft must
  not assume a pipeline id on every point.

**Nothing here is semconv.** No `otelcol.*` attribute or metric exists in the
semantic-conventions registry; all of it is collector-owned, at collector
stability (alpha for the helper metrics, development for the new ones,
"no guarantees" for logs, "experimental" for traces).

**Docs/code divergence flagged (§7): the official internal-telemetry page
documents none of the `otelcol.component.*` attributes, the gate, or the new
dotted metrics.** The code is the authority for all spellings below.

---

## 1. Current release

**v1.64.0 / v0.158.0, published 2026-08-04** (GitHub releases API; dual tag —
stable modules at 1.64.0, the rest at 0.158.0). `opentelemetry-collector-contrib`
released the matching **v0.158.0** the same day. All source citations below are
against tag `v0.158.0`, commit `0378e9a`.

Internal-telemetry stability is per-signal (docs, `internal-telemetry.md`):

| Signal | Stability |
|---|---|
| Metrics | Per-metric lifecycle (development → alpha → beta → stable). Helper metrics are **alpha** ("Alpha metrics have no stability guarantees"); the new pipeline metrics are **development**; process metrics alpha. |
| Logs | "Individual log entries and their formatting might change from one release to the next. There are no stability guarantees at this time." |
| Traces | "Internal tracing is an experimental feature, and no guarantees are made as to the stability of the emitted span names and attributes." |
| Config surface | Built on the OTel declarative-config schema v0.3 (`otelconf/v0.3.0`); docs warn it "may undergo **breaking changes** in future releases" (issue #10808). Config structs are typed `...V030` and marked `Experimental` in `service/telemetry/otelconftelemetry/config.go`. |

## 2. Exact attribute spellings

### 2a. Component-identity scope attributes (the new scheme)

Constants in `internal/telemetry/attribute.go:12-16` (quoted in the TL;DR).
Attached per component kind in `service/internal/attribute/attribute.go:50-103`
— this table is the ground truth for which keys exist on which kind:

| Kind | Scope attributes attached |
|---|---|
| receiver | `otelcol.component.kind`=`receiver`, `otelcol.signal`, `otelcol.component.id` |
| processor | `otelcol.component.kind`=`processor`, `otelcol.signal`, **`otelcol.pipeline.id`**, `otelcol.component.id` |
| exporter | `otelcol.component.kind`=`exporter`, `otelcol.signal`, `otelcol.component.id` |
| connector | `otelcol.component.kind`=`connector`, `otelcol.signal` (input side), `otelcol.signal.output` (output side), `otelcol.component.id` |
| extension | `otelcol.component.kind`=`extension`, `otelcol.component.id` |
| (internal) | `otelcol.component.kind`=`capabilities` or `fanout`, with `otelcol.pipeline.id` — synthetic graph nodes, not components in the YAML |

Kind values are **lowercase** since v0.125.0 (CHANGELOG: "Lowercase values for
'otelcol.component.kind' attributes. (#12865)").

**Where they apply, per signal** (`service/internal/componentattribute/telemetry.go:13-27`):
Logger and TracerProvider are wrapped **unconditionally**; MeterProvider only
`if metadata.TelemetryNewPipelineTelemetryFeatureGate.IsEnabled()`. The gate
(`service/internal/metadata/generated_feature_gates.go:41-47`):

```
"telemetry.newPipelineTelemetry", featuregate.StageAlpha, from v0.123.0
"Injects component-identifying scope attributes in internal Collector metrics"
```

They are **instrumentation scope attributes**, not datapoint/log-record
attributes. On OTLP-exported logs they land on the scope (v0.125.0 CHANGELOG:
"internal logs exported through OTLP will now use instrumentation scope
attributes to identify the source component instead of log attributes. This
does not affect the Collector's stderr output" — on stderr they appear as
ordinary zap fields).

Timeline of the rename/mechanism (CHANGELOG):
- v0.120.0 — logs first carried component ids as log-record attributes.
- v0.123.0 — `telemetry.newPipelineTelemetry` gate added; switched to scope attributes (#12217).
- v0.125.0 — gate restricted to metrics; scope attributes for logs+traces turned **on by default**; kind values lowercased.
- v0.131.0 — new metrics gained `otelcol.component.outcome` (`success`/`failure`/`refused`) datapoint attribute (#13234).
- Still StageAlpha at v0.158.0 — no further promotion.

### 2b. Legacy datapoint attributes (what default-on metrics actually carry)

| Where | Key | Value | Source |
|---|---|---|---|
| receiverhelper metrics | `receiver` | `ReceiverID.String()` (full `type/name`) | `receiver/receiverhelper/internal/obsmetrics.go:11`, `obsreport.go:69` |
| receiverhelper metrics | `transport` | e.g. `grpc`, `http` | `obsmetrics.go:13` |
| scraperhelper metrics | `receiver`, `scraper`, `format` | ids via `.String()` | `scraper/scraperhelper/obs_metrics.go:25-37` |
| processorhelper metrics | `processor` | `set.ID.String()` | `processor/internal/obsmetrics.go:10`, `processorhelper/obsreport.go:33` |
| processorhelper metrics | **`otel.signal`** (note: *not* `otelcol.signal`) | `traces`/`metrics`/`logs` | `processorhelper/obsreport.go:19` |
| exporterhelper metrics | `exporter` | `set.ID.String()` | `exporter/exporterhelper/internal/obs_report_sender.go:35,71` |
| exporterhelper metrics | `data_type` | signal name | `obs_report_sender.go:38` |
| queue metrics | `exporter`, `data_type` | as above | `exporterhelper/internal/queue/obs_queue.go:21-24,44-45` |

### 2c. Instance-level resource attributes

`service/telemetry/otelconftelemetry/resource.go:25-35` — defaults:

```go
string(semconv.ServiceNameKey):       buildInfo.Command,   // e.g. "otelcol-contrib"
string(semconv.ServiceVersionKey):    buildInfo.Version,
string(semconv.ServiceInstanceIDKey): instanceUUID.String(), // random UUIDv4 per process start
```

So `service.name`, `service.version`, `service.instance.id` — semconv
schema URL v1.40.0 (import at `resource.go:15`; bumped from v1.37.0 at
v0.141.0 per CHANGELOG #14232). All overridable and suppressible under
`service::telemetry::resource` ("the attribute must be specified with a null
value" to suppress — `config.go:19-26`). Since v0.157.0, experimental
`service::telemetry::resource::detection/development` adds SDK resource
detection. **Caveat for G6: `service.instance.id` is regenerated on every
process restart** — it identifies a process incarnation, not a node; joining
across restarts needs an adopter-set attribute or host identity.

## 3. Metric names for per-component throughput

Two coexisting generations, distinguishable by separator. mdatagen prefixes
un-prefixed names with `otelcol_`; `prefix: otelcol.` yields dotted names.
**The instrument (OTLP) name for the legacy metrics is underscore-form** —
verified in `receiver/receiverhelper/internal/metadata/generated_telemetry.go:74`
(`"otelcol_receiver_accepted_log_records"`). The docs confirm: "Use the
`instrument_name` value `otelcol_process_uptime` (the OTLP name) in views."
So **underscores are not a Prometheus artifact — `otelcol_receiver_accepted_spans`
is the name on the OTLP surface too.** The only Prometheus-vs-OTLP differences
are Prometheus type/unit suffixes (`_total`, `_seconds` — suppressed by the
default reader's `without_type_suffix`/`without_units: true`) and dot→underscore
translation for the new dotted metrics.

### Legacy helper metrics (default-on, stability: alpha)

- Receivers (`receiver/receiverhelper/metadata.yaml`):
  `otelcol_receiver_accepted_{spans,metric_points,log_records,profile_samples}`,
  `otelcol_receiver_refused_*`, `otelcol_receiver_failed_*`; plus
  `otelcol_receiver_requests` (attr `outcome`: `success|refused|failure`) —
  gated behind `receiverhelper.newReceiverMetrics` (alpha, v0.138.0), which
  also **changes the semantics of `refused` vs `failure`** on the existing
  metrics ("This is a breaking change for the semantics of the
  otelcol_receiver_refused_* " — metadata.yaml:127).
- Scrapers (`scraper/scraperhelper/metadata.yaml`):
  `otelcol_scraper_scraped_{metric_points,log_records}`,
  `otelcol_scraper_errored_*`.
- Processors (`processor/processorhelper/metadata.yaml`):
  `otelcol_processor_incoming_items`, `otelcol_processor_outgoing_items`,
  `otelcol_processor_internal_duration`. (Item-count, signal in the
  `otel.signal` attr — no per-signal name variants.)
- Exporters (`exporter/exporterhelper/metadata.yaml`):
  `otelcol_exporter_sent_{spans,metric_points,log_records,profile_samples}`,
  `otelcol_exporter_send_failed_*`, `otelcol_exporter_enqueue_failed_*`.
- Queue: `otelcol_exporter_queue_size`, `otelcol_exporter_queue_capacity`
  (gauges, unit `{batch}`; code description says "retry queue", docs say
  "sending queue" — cosmetic divergence), `otelcol_exporter_queue_batch_send_size{,_bytes}`.

Components not built on these helpers may emit none of them (docs: "some
components not using those packages might not emit them").

### New pipeline metrics (gated, stability: development)

`service/metadata.yaml` + `service/internal/metadata/generated_telemetry.go:191-287`,
emitted by `service/internal/obsconsumer` **only when
`telemetry.newPipelineTelemetry` is enabled** (`obsconsumer/metrics.go:24`
returns the bare consumer otherwise):

`otelcol.receiver.produced.items`, `otelcol.processor.consumed.items`,
`otelcol.processor.produced.items`, `otelcol.exporter.consumed.items`,
`otelcol.connector.consumed.items`, `otelcol.connector.produced.items`
(each with a `.size` sibling, disabled by default). Datapoint attribute
`otelcol.component.outcome` = `success|failure|refused`
(`obsconsumer/option.go:12`); identity comes from the scope attributes (§2a).

### Levels

Docs group the metrics: `basic` covers all receiver/scraper/processor/exporter
throughput + queue + process metrics; `normal` (the default) adds
`otelcol_processor_batch_*`; `detailed` adds `otelcol_processor_batch_batch_send_size_bytes`
plus non-collector-owned `http.*`/`rpc.*` client-server metrics. Note
`rpc.*.request.size` and friends stopped being emitted at v0.148.0.

## 4. Config surface: `service::telemetry`

Implemented by `service/telemetry/otelconftelemetry` wrapping
`go.opentelemetry.io/contrib/otelconf/v0.3.0` (the declarative-config SDK).

- **Levels** (`config/configtelemetry/configtelemetry.go:12-20`): `none`,
  `basic`, `normal` (default), `detailed`. Validation: readers must be
  non-empty when level ≠ none; `views` only allowed at `detailed`
  (`otelconftelemetry/config.go:52-63`).
- **Default** (`otelconftelemetry/factory.go:createDefaultConfig`): one
  **pull Prometheus reader on `localhost:8888`** with
  `WithoutScopeInfo: true`, `WithoutUnits: true`, `WithoutTypeSuffix: true`.
  Logs: console encoding to stderr, level INFO, sampled. No traces exporter
  by default.
- **OTLP-out**: metrics via `service::telemetry::metrics::readers` →
  `periodic::exporter::otlp`; logs via `service::telemetry::logs::processors`
  → `batch::exporter::otlp`; traces via `service::telemetry::traces::processors`
  likewise (docs, "Configure internal {metrics,logs,traces}"). Since
  v0.150.0 headers on the internal OTLP exporter are redacted when config is
  marshaled (#14756).
- **Migration status**: the SDK-config path is finished business —
  `useOtelWithSDKConfigurationForInternalTelemetry` went stable v0.110.0 and
  was **removed** v0.128.0; the old `metrics::address` shorthand is gone from
  the config struct (readers only). `telemetry.disableHighCardinalityMetrics`
  was deprecated v0.132.0 and **removed** v0.144.0 (use views).
  **The pull-based Prometheus default has NOT flipped to OTLP-push** — no
  gate for that exists; OTLP-out remains opt-in per collector.
- **Scope-label wrinkle**: v0.130.0 enabled `otel_scope_*` labels on the
  default Prometheus reader; v0.131.0 **reverted** it (Prometheus-exporter
  downgrade, #13429/#13344), and at v0.158.0 the default still has
  `WithoutScopeInfo: true`. Consequence: on the default Prometheus surface,
  scope attributes are dropped — which is exactly why the metrics half of
  `newPipelineTelemetry` stays gated ("enabling the ... feature gate may break
  the export of Collector metrics ... Having a `batch` processor in multiple
  pipelines is a known trigger" — v0.125.0 CHANGELOG). **The
  `otelcol.component.*` metric attributes are effectively OTLP-path-only.**

## 5. Join keys back to the rendered YAML

**Reliable today (default config), per surface:**

- Instance: `service.instance.id` (per-process UUID) + `service.name`;
  stronger instance identity should be stamped by Telecraft via
  `service::telemetry::resource`.
- Logs/traces → component: scope attrs `otelcol.component.kind` +
  `otelcol.component.id` (+ `otelcol.signal`; + `otelcol.pipeline.id` for
  processors). Values match the config YAML exactly: `component.ID.String()`
  returns `type` or `type/name` (`component/identifiable.go:164-170`), and
  `pipeline.ID.String()` likewise gives `metrics` or `metrics/name`.
- Metrics → component: legacy datapoint attrs `receiver` / `scraper` /
  `processor` / `exporter` — same full `type/name` values. Signal comes from
  the metric name (receivers/exporters) or `otel.signal` / `data_type` attrs.

**Caveats (all verified in source):**

1. **Receivers/exporters carry no pipeline id on any surface.** A receiver in
   three metrics pipelines is one instance with one telemetry stream. Joining
   throughput to a *pipeline* requires the config topology (which G6 has from
   the rendered YAML).
2. **Singleton components deliberately drop identity attributes.** The `otlp`
   receiver drops `otelcol.signal` (`receiver/otlpreceiver/otlp.go:56`);
   `memory_limiter` drops `otelcol.signal`, `otelcol.pipeline.id` **and
   `otelcol.component.id`** (`processor/memorylimiterprocessor/factory.go:138-143`)
   — its logs/metrics under the new scheme identify only as
   `otelcol.component.kind=processor`. The RFC blesses this pattern, so other
   components may do the same.
3. **Connectors** appear once per (input signal, output signal) with
   `otelcol.signal` + `otelcol.signal.output` scope attrs; on the new consumed/
   produced metrics the *destination* pipeline is a **datapoint** attribute
   `otelcol.pipeline.id` (`service/internal/graph/connector.go:27,106`), so a
   connector feeding two pipelines is disambiguated there — but only under
   the gate.
4. **Synthetic nodes**: `otelcol.component.kind` values `capabilities` and
   `fanout` (`service/internal/attribute/attribute.go:17-19,91-103`) do not
   correspond to anything in the YAML; a joiner must tolerate/ignore them.
5. **Scraper metrics** join on `receiver` + `scraper` (two-level identity —
   e.g. `hostmetrics` receiver, `cpu` scraper).
6. **`otel.signal` vs `otelcol.signal`**: the legacy processor datapoint attr
   and the new scope attr differ by prefix. Do not normalise them into one
   key blindly.
7. Everything under §2a on metrics is downstream of an **alpha gate whose
   CHANGELOG history includes one revert and one semantic restriction** —
   treat as unstable for schema purposes; model the legacy attrs as the
   primary key and the scope attrs as an alternate.

## 6. Semconv status

**Not registered.** GitHub code search for `otelcol` in
`open-telemetry/semantic-conventions` returns 5 hits, all incidental uses of
`otelcol` as an example *process name* in `process.*`/`cli` docs
(`model/process/registry.yaml`, `docs/registry/attributes/process.md`, …).
There is no `model/otelcol/` namespace; the registry's `otel` namespace covers
`otel.scope.*`/`otel.status_code`, not the collector. The collector's own docs
say only that its metric stability levels "follow Semantic Conventions
[guidance]" — the *process*, not the names. **Stability of every `otelcol.*`
name is governed solely by the collector repo** (alpha/development as in §3).

## 7. Docs vs code disagreements

1. **The official internal-telemetry page omits the entire new scheme.** The
   `main` docs source (fetched 2026-08-14) contains zero occurrences of
   `otelcol.component`, `otelcol.pipeline`, `newPipelineTelemetry`, or the
   dotted `*.produced.items` metrics, and does not document the legacy
   `receiver`/`exporter`/`processor` datapoint attributes either. Trusted:
   the code (§2). Consequence: expect ecosystem confusion and possible doc
   churn; do not treat doc absence as evidence of removal.
2. `otelcol_exporter_queue_size`/`_capacity`: docs say "sending queue",
   code metadata says "retry queue" (`exporter/exporterhelper/metadata.yaml:126,135`).
   Cosmetic; trusted the code.
3. The docs' claim that resource attributes are "randomly generated" applies
   to `service.instance.id` only; name/version come from build info — code
   (`resource.go:25-35`) is precise.

## Recommendations for G6

1. Join on the **legacy datapoint attrs** (`receiver`/`processor`/`exporter`
   /`scraper`, full `type/name`) for metrics, and on the
   **`otelcol.component.*` scope attrs** for logs/traces. Keep both mappings
   in one normalisation layer keyed by `(kind, id)`.
2. Derive pipeline membership of receivers/exporters from the rendered YAML,
   never from telemetry (§5.1).
3. Have Telecraft-rendered configs stamp a stable instance identity into
   `service::telemetry::resource` — the default `service.instance.id` rotates
   every restart.
4. If Telecraft wants the new `otelcol.*.{consumed,produced}.items` metrics,
   it must render both the `telemetry.newPipelineTelemetry` gate **and** an
   OTLP reader (the default Prometheus surface drops the identifying scope
   attributes). Gate this behind a Telecraft feature flag mirroring upstream's
   alpha status.
5. Model `capabilities`/`fanout` kinds and attribute-dropping singletons
   (`otlp` receiver, `memory_limiter`) as expected, not as join failures.

## Sources

- Releases: https://api.github.com/repos/open-telemetry/opentelemetry-collector/releases (v1.64.0/v0.158.0, 2026-08-04); https://api.github.com/repos/open-telemetry/opentelemetry-collector-contrib/releases (v0.158.0, 2026-08-04)
- Source at tag v0.158.0 (commit 0378e9a), https://github.com/open-telemetry/opentelemetry-collector/tree/v0.158.0 — files cited inline: `internal/telemetry/attribute.go`, `internal/telemetry/telemetry.go`, `service/internal/attribute/attribute.go`, `service/internal/componentattribute/{telemetry.go,logger_zap.go}`, `service/internal/obsconsumer/{option.go,metrics.go}`, `service/internal/graph/connector.go`, `service/internal/metadata/generated_feature_gates.go`, `service/metadata.yaml`, `service/telemetry/otelconftelemetry/{config.go,factory.go,resource.go}`, `config/configtelemetry/configtelemetry.go`, `component/identifiable.go`, `receiver/receiverhelper/{metadata.yaml,obsreport.go,internal/obsmetrics.go}`, `processor/processorhelper/{metadata.yaml,obsreport.go}`, `processor/internal/obsmetrics.go`, `processor/memorylimiterprocessor/factory.go`, `receiver/otlpreceiver/otlp.go`, `exporter/exporterhelper/{metadata.yaml,internal/obs_report_sender.go,internal/queue/obs_queue.go}`, `scraper/scraperhelper/obs_metrics.go`, `CHANGELOG.md`, `docs/rfcs/component-universal-telemetry.md`
- RFC: https://github.com/open-telemetry/opentelemetry-collector/blob/main/docs/rfcs/component-universal-telemetry.md
- Docs: https://opentelemetry.io/docs/collector/internal-telemetry/ (source: https://raw.githubusercontent.com/open-telemetry/opentelemetry.io/main/content/en/docs/collector/internal-telemetry.md, fetched 2026-08-14)
- Semconv search: https://github.com/search?q=repo%3Aopen-telemetry%2Fsemantic-conventions+otelcol&type=code (5 hits, all `process.*` examples); https://github.com/open-telemetry/semantic-conventions/tree/main/model (no `otelcol` namespace)
- CHANGELOG anchors (opentelemetry-collector): v0.123.0 #12217 (gate added), v0.125.0 #12865/#12856/#12933 (lowercase kinds; gate restricted to metrics; logs/traces scope attrs default-on), v0.130.0→v0.131.0 #13344/#13429 (otel_scope labels enabled then reverted), v0.131.0 #13234 (`otelcol.component.outcome`), v0.110.0/v0.128.0 #7532/#13152 (SDK-config gate stable, removed), v0.132.0/v0.144.0 #13537/#14373 (high-cardinality gate deprecated, removed), v0.141.0 #14232 (semconv v1.37.0; v1.40.0 at v0.158.0 per `resource.go:15`), v0.150.0 #14756 (OTLP header redaction), v0.157.0 (resource detection), v0.138.0 #12802 (`receiverhelper.newReceiverMetrics`)
