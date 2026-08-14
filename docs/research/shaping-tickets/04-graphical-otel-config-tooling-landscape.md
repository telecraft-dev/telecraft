# How do existing graphical OTel config tools model topology?

Type: research
Status: resolved
Blocked by: none

## Question

Amp-Up's ambition is a fleet and policy management platform that models
topologies graphically, from a single collector to a gateway with many
collectors, and generates the OpenTelemetry configurations. That is a populated
market. Establish what the incumbents model and where each one stops, so ticket
07 decides Amp-Up's own modelling abstraction from evidence rather than from a
blank page.

Cover at least: **Bindplane** (observIQ), **Cribl** (Stream and Edge),
**Dash0**, **Grafana Alloy** and its UI, **otelbin.io**, **Dynatrace** and
**Splunk** OTel management surfaces, and the OpenTelemetry project's own
emerging config tooling including the declarative configuration effort.

For each, answer:

1. What is the **unit the user manipulates**? A pipeline graph, a per-agent
   config file, a reusable policy, a fleet-wide template, a source and
   destination pair?
2. How is **reuse** expressed? Can one policy target many collectors, and how is
   the per-collector variation handled?
3. Can it model a **multi-tier topology**, where edge collectors feed a gateway
   tier that fans out? Or does each collector get modelled in isolation?
4. What does it do about **drift**, meaning config changed outside the tool?
5. Does anything in this set connect config to **outcomes**, checking whether
   the telemetry the config asked for actually arrived? This is the claimed gap
   and it needs testing rather than asserting.
6. Where is each **open source**, and under what licence?

Return the modelling abstractions as a comparison, and name explicitly which
parts of the gap the conformance-platform spec claims are genuinely unfilled.

## Answer

Resolved 4 August 2026. Full findings with citations:
[research/04-findings.md](../research/04-findings.md).

### Q1 unit manipulated, Q3 multi-tier

| Tool | Unit manipulated | Multi-tier modellable? |
|---|---|---|
| **Bindplane** | Named versioned `Configuration`: canvas graph of Source, Processor, Destination, Connector, Extension, pinned to one OS platform | **Closest, but discovered rather than authored.** `topologyprocessor` infers gateway edges from in-band traffic headers and draws them. Each tier is still a separate Configuration |
| **Cribl** | The `Route`: an ordered filter table binding one Pipeline to one Destination. QuickConnect is a separate, weaker canvas that bypasses Routes | **Deployable, not modelled.** The Data Insights map is limited to a specific Worker Group, so the Edge-to-Stream hop is two cards on two maps |
| **Dash0** | **No collector-config surface at all.** Ships a Bindplane integration for fleet. Has a `Dash0Monitoring` CRD using OTTL, plus backend spam filters | No. DaemonSet plus Deployment is a data-source split; both export direct to the backend |
| **Alloy + UI** | Text component graph. **The UI is read-only**: list, graph, health, debug, with no documented write path. In Fleet Management the unit is a labelled pipeline fragment | No. The graph is intra-process; the Fleet Management data model is flat, collectors to pipelines |
| **otelbin.io** | One otelcol YAML in a text editor. The graph is one-way, verified at source: no `onNodesChange`, no `onConnect`, no graph-to-YAML path | No. One config, one set of swimlanes |
| **Dynatrace** | Three unconnected surfaces: unmanaged collector YAML, `DynaKube` declaring a collector exists, and OpenPipeline which is **server-side ingest**, not collector config | **The tier is real and chainable**, ActiveGate to ActiveGate, bound by network zones with priority fallback. Never composed graphically |
| **Splunk** | Split: otelcol YAML with no UI, versus Edge Processor's SPL2 `$pipeline` linear step editor. Edge Processor cannot author otelcol | Edge Processor is a tier, but the console is per-pipeline. No composed graph |
| **OTel declarative config** | A JSON Schema, 1.0.0 from February 2026, Stable. One document, one process. No UI | None. The Operator's `spec.mode` is a workload-shape enum on independent CRs; connectors are intra-process |

**Two structural findings, and they are the useful output of this ticket.**
**Nobody authors config by manipulating a topology graph.** And **the tier hop is
universally a string, not an object**, in all eight.

### Q5, the claimed gap: partly filled, and the claim must be reworded

Three genuine refutations of "nobody connects config to outcomes":

- **Cribl ships absence alerting.** Verbatim: "'No Data Received': The Source or
  Collector ingests zero data over your configured time window", plus High and
  Low Data Volume. Real triggers, real notification channels.
- **Weaver `registry live-check`** checks "the conformance level of an OTLP
  stream against a semantic convention registry", with Rego policies and CI
  actions. It judges emitted telemetry against a declaration.
- **Collector self-telemetry is already keyed to config identifiers.**
  `otelcol_exporter_sent_*` and siblings carry `otelcol.component.id` and
  `otelcol.pipeline.id`.

None reaches the actual gap. Cribl's triggers cover Sources and Destinations
only, with no Route or Pipeline condition, each hand-configured, and Cribl's own
docs confirm there is no automatic trigger derivation from pipeline or route
configuration. Weaver's declaration is a semconv registry, not a config, and it
has no notion of a collector or a fleet. The counters have no expectation to
compare against.

**Unfilled by all eight**: no tool derives, from the configuration it manages, an
expectation of what telemetry should arrive, and then checks it. Green
universally means "I applied the config", never "the config worked", verified
independently in Dash0, Alloy Fleet Management and Bindplane. And nobody can
make that join across a tier boundary, because nobody holds an object
representing the hop. **That makes questions 3 and 5 the same question.**

### Recommendations

- **Retire "connects config to outcomes"** as the differentiator. Claim instead:
  **the intended multi-tier topology as a first-class object, reconciled against
  both drift and delivery.**
- **Drop drift detection from the pitch entirely.** Alloy and Splunk both ship
  effective-versus-server config views, and Splunk markets drift detection by
  name. It is table stakes, not a differentiator.

### Two corrections for the record

- **Dash0's semconv work is normalisation, not conformance checking.** It
  silently rewrites attributes rather than flagging them. Relevant to tickets 05
  and 16: it is not a competitor to a conformance check, it is the opposite
  posture.
- `otelcol_processor_dropped_*` no longer exists, replaced by `incoming_items`
  and `outgoing_items`.
