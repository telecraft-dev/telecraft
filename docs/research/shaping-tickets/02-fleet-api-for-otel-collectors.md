# What does Elastic Fleet expose over its API for OTel collectors?

Type: research
Status: resolved
Blocked by: none

## Question

Amp-Up's fleet seam will, pragmatically, be an Elastic connector. What can that
connector actually read?

1. Which Fleet API endpoints return OTel collectors, as distinct from Elastic
   Agents? What does an OTel collector's record contain?
2. What is the shape of the **reported effective config**? Is it a raw YAML or
   JSON blob, a structured object, or absent entirely? Is it the full config or
   a redacted subset?
3. Can it be parsed into **receivers, processors, exporters and the pipelines
   wiring them together**? The pipeline structure matters more than the
   component lists: knowing a `filelog` receiver exists proves nothing if it is
   wired only into a traces pipeline.
4. What **health** and status fields are exposed, and at what granularity?
5. What does Fleet expose about collectors it did **not** configure, if
   anything? That population is exactly the `ungoverned` quadrant.
6. What authentication does the API need, and can a read-only API key reach all
   of the above?

Answer against a live target where possible: the PoC's Serverless project has
Fleet enabled with a hosted Fleet Server at
`d57725ac16664d11bb0151b4f230fe42.fleet.europe-west2.gcp.elastic.cloud`, and
zero agents enrolled. Credentials are in `poc/.env`, gitignored. Read-only calls
only. Do not enrol anything: that is ticket 06's job.

This decides whether Amp-Up's declared reading can be populated richly, thinly,
or not at all from Fleet, and it is the evidence ticket 11 and ticket 12 both
depend on.

## Answer

Resolved 4 August 2026. Full findings with citations:
[research/02-findings.md](../research/02-findings.md).

**Fleet can populate a genuinely structured declared reading, not a shallow one.
The pipeline graph, which was the load-bearing uncertainty, is available, and
cheaply in bulk.**

1. **No separate collector API.** `/api/fleet/otel_collectors` and
   `/api/fleet/collectors` both hard-404 on the live project. Collectors return
   from the ordinary agent endpoints, discriminated by top-level `type: "OPAMP"`.
   The string `otel-collector` exists nowhere. `kuery=type:OPAMP` is accepted
   live, so filtering costs one parameter. A record is an Elastic Agent record
   plus `effective_config`, `health`, `identifying_attributes`,
   `non_identifying_attributes`, `capabilities`, `sequence_num`, `signals` and
   `pipeline_config`. Watch for a second field, `agent.type`, set from the
   collector's own OpAMP `service.name`, which is not the same thing as `type`.

2. **Effective config is a structured JSON object, not a blob.** fleet-server
   YAML-unmarshals the OpAMP body, redacts, and re-marshals to JSON;
   `GET /api/fleet/agents/{agentId}/effective_config` returns it untyped. Full
   config, with two losses to design around: only the **unnamed** OpAMP
   config-file entry is ingested and named entries are silently dropped, and
   scalar values are replaced with `"REDACTED"` by substring match on key names
   matching `auth|certificate|passphrase|password|token|key|secret`.

3. **Yes, pipeline wiring included.** This was the question that mattered and it
   comes back positive. `effective_config.service.pipelines.<name>.{receivers,
   processors,exporters}` is present verbatim, with **processor order preserved**,
   which Elastic's own code notes is semantically significant. Two Elastic
   Painless runtime fields reach into exactly those paths, so this is
   demonstrated rather than inferred. Redaction recurses into maps rather than
   replacing them, so structure and component names survive intact.
   **There are two routes.** The list endpoint carries a `pipeline_config`
   fingerprint encoding `pipe:<name>[recv|proc|exp]` for every pipeline, so full
   wiring for N collectors arrives in **one call**. The per-agent
   `effective_config` is then needed only for config *values*.

4. **Health has two levels and the gap matters.** The full recursive OpAMP
   `ComponentHealth` tree is stored and returned under `health`, arbitrarily
   deep, per pipeline and per receiver. But the roll-up `status` is flattened
   from top-level health only, and fleet-server's own docs say the nested map
   "is not traversed". **A collector with a dead receiver but a healthy top
   level reads as `online`.** Any consumer must read the tree, never the status.

5. **Every collector Fleet sees is one it did not configure.** Fleet's OTel
   support is monitoring-only, `remote_config` is explicitly unimplemented, and
   enrolment pins `PolicyRevisionIdx: 1` so no policy is ever delivered.
   Self-authored config is therefore reported faithfully. The hard boundary is
   that collectors must **opt in** with an OpAMP extension and a valid enrolment
   key. Fleet has no discovery or scanning, so genuinely unconnected collectors
   are invisible. The `ungoverned` quadrant is only partly served: Fleet covers
   self-configured-but-connected and is blind to unconnected.

6. **A Kibana API key with `fleet-agents-read` reaches everything above**, with
   no write privilege needed. Stated honestly: the PoC key holds
   `feature_fleetv2.all`, so the successful reads do not prove a minimal key
   suffices. Verifying that needs minting a restricted key, which is a write and
   was out of scope. The key gives no direct index access either, since
   `.fleet-agents` reads were refused, so everything must go through the Fleet
   API.

**Evidence caveat, and it is a real one.** The project has zero agents enrolled,
so all record *content* comes from schema and source rather than live records.
The live probe did establish one thing beyond empty envelopes:
`/effective_config` returned `index_not_found_exception` while a bogus route
returned bare `Not Found`, which proves the OpAMP-gated routes are registered
and enabled on this 9.6.0 Serverless project rather than merely present
upstream. One probe technique was discarded after a control test showed the KQL
validator accepts any dotted path; it is documented in the findings so nobody
rebuilds a conclusion on it. **Ticket 06 should re-run the two-step read against
a real collector to confirm.**
