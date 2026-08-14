# Run the Fleet coexistence spike on the live PoC

Type: task
Status: resolved
Blocked by: 01

## Question

Nothing to decide here. This is the manual work that produces the evidence
tickets 12 and 13 both wait on, and it is on the map only for that reason.

Add an `opamp` extension pointed at Fleet Server to a rendered Managed Edge
Collector config on the running Kubernetes PoC, and establish by observation:

1. Does the collector **appear in Kibana Fleet**, with health and a reported
   effective config?
2. Does the **Interchange Supervisor still drive its configuration**? Push a
   Baseline change and confirm it lands.
3. Does the Supervisor **overwrite the hand-added extension block** on the next
   config push? Ticket 01 should predict this. Confirm the prediction.
4. What is the **exact shape of the effective config Fleet reports**? Capture it
   verbatim. Is it byte-identical to what Interchange rendered, semantically
   equivalent, or reduced?
5. Do the two connections interfere: reconnect loops, duplicate instance UIDs,
   conflicting remote config offers?

**Environment.** The PoC runs on Docker Desktop Kubernetes and ingests into
Elastic Serverless project `playground-d57725`, region `europe-west2`. Fleet is
already enabled there with a hosted Fleet Server at
`d57725ac16664d11bb0151b4f230fe42.fleet.europe-west2.gcp.elastic.cloud` and zero
agents enrolled, so nothing needs provisioning. Credentials are in `poc/.env`,
which is gitignored. Start from `poc/README.md`.

**Gotchas already known.** A rebuilt image has no effect without `make images`
or `make load`, because Docker Desktop's kubelet cannot see `docker build`
output. NodePorts do not resolve on `localhost`; use `kubectl port-forward`.
Fleet's generated bootstrap config sets `tls.insecure_skip_verify: true`.

**Warnings from ticket 03, read before starting.** Two of these could make the
spike fail for reasons that have nothing to do with the architecture, so do not
misread either as a verdict on ADR-0003.

- The PoC runs **`otelcol-contrib`, not EDOT**. Upstream contrib enrols and
  reports, but it fails Fleet's "Elastic Agent 7.11+" version gate and loses UI
  features. An Elastic engineer acknowledged this unfixed on 22 June 2026. If
  the collector appears but the UI is degraded, that is expected and is not
  evidence against coexistence.
- **The OpAMP Supervisor path is unsupported by Fleet**, and Fleet Server
  operates in monitoring-only mode. This spike is the in-process extension
  pointing at Fleet, which is a different thing, but do not be surprised by
  Supervisor-shaped errors.
- Fleet uses **HTTP transport only**. WebSocket is unsupported, and Interchange's
  own OpAMP endpoint is a plain WebSocket. Configure the extension accordingly.
- Authentication is a **shared enrolment token**.
- Watch for `fleet-server#6820`: agent identity is frozen at enrolment and
  `AgentDescription` updates are silently ignored afterwards. If you push a
  config change and Fleet keeps showing the old identity, that is this bug, not
  a coexistence failure. Question 4 is still answerable, because effective
  config is reported separately from identity, but **check whether the reported
  effective config also goes stale after a Supervisor push**. Nothing in the
  research answers that, it is exactly what this spike can settle, and it
  decides whether a drift check is possible at all.

## Sharpened by tickets 01 and 02

Both are resolved. They change how to run this and add the one question that
matters most.

**Do this, or the spike fails for the wrong reason.**

- **Name the second extension `opamp/fleet`, never `opamp`.** The Supervisor
  injects its own `extensions.opamp` block, and `$REMOTE_CONFIG` merges *after*
  it, so a block named `opamp` silently overrides the Supervisor's endpoint and
  breaks the Supervisor link. The Supervisor design doc claims this cannot
  happen. No code enforces the claim.
- **Question 3 is already answered: the Supervisor will not overwrite it.**
  `service::extensions` merges by concatenation and de-duplication, not
  replacement, so `[opamp]` plus `opamp/fleet` yields both. Verified across
  releases back to v0.120.0. Confirm it live, but treat a different result as
  surprising rather than expected.

**The single most important question, unestablished anywhere upstream: can one
collector run two concurrent `opamp` extension instances at all?** The component
model permits it, the factory holds no shared state, and there is zero upstream
doc, test, example or issue either way. Nothing more can be learned by reading
code. This spike is the only way to settle it, and if it fails, ADR-0003 fails
with it.

**While you are there, confirm ticket 02's reads against a real collector.**
Ticket 02 established Fleet's API shape from schema and source, because the
project has zero agents enrolled. This spike produces the first real record, so:

- `GET /api/fleet/agents?kuery=type:OPAMP` and check the `pipeline_config`
  fingerprint decodes to the pipelines you rendered.
- `GET /api/fleet/agents/{agentId}/effective_config` and check
  `service.pipelines.<name>.{receivers,processors,exporters}` is present with
  processor **order** intact.
- Check what `"REDACTED"` swallowed. Fleet redacts scalars whose key matches
  `auth|certificate|passphrase|password|token|key|secret`, which will hit
  exporter credentials in a rendered Interchange config.
- Check whether the collector's `opamp` config file entry is **unnamed**. Named
  entries are silently dropped and the effective config would arrive empty.
- Read the recursive `health` tree, not the roll-up `status`. The roll-up does
  not traverse the tree, so a dead receiver reads as `online`.

**And the question the research could not answer.** ADR-0003's drift check needs
Fleet's reported effective config to stay current. `fleet-server#6820` freezes
agent *identity* at enrolment. **Does the reported effective config go stale the
same way after a Supervisor push?** Push a Baseline change and re-read. If it
goes stale, the drift check is impossible and ADR-0003's self-verifying claim
fails. Nothing else on the map can settle this.

**Record in the answer**: what was seen for each of the five questions, the
captured effective config verbatim, and anything that had to change on the PoC
to make it work. Leave the PoC in a working state, and say so if you cannot.

## Answer

Run 4 August 2026 against the live Kubernetes PoC and Serverless project
`playground-d57725`. **ADR-0003's central claim holds, proven by observation.**
One material defect was found that would have broken it in production, plus a
fix, verified.

### The question nothing else could settle: two concurrent opamp extensions

**Yes.** One `otelcol-contrib` 0.156.0 process ran both the Supervisor's
injected `opamp` extension and the rendered `opamp/fleet` extension
concurrently. Fleet's reported effective config shows
`service.extensions: ['opamp', 'opamp/fleet']`, both logged "Extension started"
with no errors, and the reported health tree carries
`extension:opamp` and `extension:opamp/fleet` each at `StatusOK`.

Ticket 01's predictions all held:

1. The Supervisor **did not overwrite** the rendered extension. Concatenation
   and de-duplication confirmed live.
2. Naming it `opamp/fleet` was load-bearing and worked. The Supervisor's own
   `opamp` kept its local endpoint `ws://127.0.0.1:35217/v1/opamp`.
3. No reconnect loops, no conflicting config offers, no interference.

### The Supervisor still drives config

**Yes**, verified across four Baseline pushes, each confirmed landing in the
collector's `/var/lib/otelcol/supervisor/effective.yaml`. Config authority never
moved.

### The defect: a new Fleet agent per config push

**Every Supervisor config push restarted the collector, and on restart the
`opamp/fleet` extension minted a fresh instance uid, so Fleet enrolled it as a
brand new agent.** Two Baseline changes produced **three agent records for one
collector**. The orphaned records persist permanently and report
`status: online` for several minutes before flipping to `offline`.

This was fatal to the ADR as written. Its entire value proposition is that
Customer C sees its estate in Kibana, and Baseline changes are not rare, they
are the whole point of an authoritative control plane. An estate view that grows
a phantom collector on every push, each briefly claiming to be healthy, is worse
than no view.

**The cause was visible in the reported config**: `opamp/fleet` carried
`instance_uid: ""` while the Supervisor's `opamp` carried a populated one from
its own persistent state.

**The fix, verified.** Setting `instance_uid` explicitly in the extension makes
Fleet adopt that value as the agent id directly. Two further config pushes after
pinning produced **no new agents**; the pinned record was reused and its checkin
advanced. **Consequence for the ADR**: if the extension becomes a mandatory
stamp in `render()`, the Control Plane must derive a **stable, estate-unique**
uid from the agent's registered identity. A literal cannot work for a DaemonSet,
where every node would claim the same uid.

### Effective config shape, and why the drift check needs rewriting

Structurally rich, exactly as ticket 02 predicted, and confirmed against a real
collector. Full pipeline wiring with **processor order preserved**. The
`pipeline_config` fingerprint on the list endpoint, verbatim:

```
pipe:logs[filelog/containers,otlp|memory_limiter,batch,k8sattributes,resource/platform-owner,transform/interchange|otlp/gateway];pipe:metrics[kubeletstats,otlp,prometheus/self,prometheus/workloads|memory_limiter,k8sattributes,batch,resource/platform-owner,transform/interchange|otlp/gateway];pipe:traces[otlp|memory_limiter,batch,k8sattributes,transform/interchange|otlp/gateway];ext:opamp,opamp/fleet;receivers:...;processors:...;exporters:otlp/gateway;connectors:
```

**But it is not byte-identical to what Interchange rendered, and cannot be.**

- **Redaction damages non-secret config.** Fleet redacted
  `k8sattributes.extract.labels[0].key` and `.key_regex`,
  `resource/platform-owner.attributes[0].key`, and two `auth_type:
  serviceAccount` values. It matches on the field name `key`, which in OTel
  config almost always means an attribute key rather than a credential. The
  first of those is `app.kubernetes.io/name`, which is how `service.name` is
  derived, so **governance-critical configuration is destroyed in the reported
  copy**.
- **The extension's own server block is absent entirely**, not marked redacted.
  Only `polling_interval` survives. Fleet cannot see where the extension points.

So ADR-0003's claim that reported and rendered config "should be byte-identical,
and an alert on divergence makes ADR-0001 self-verifying" is **false as
written**. A drift check is still possible but must be **semantic**, comparing
component sets and pipeline wiring while excluding a known redaction list. That
is a different and weaker claim than the ADR makes, and it should be rewritten.

### Health

The recursive `ComponentHealth` tree is fully populated: per extension, per
pipeline, per receiver and per processor, each with `healthy` and a status.
Confirms ticket 02: read the tree, never the roll-up `status`.

### Security finding, fixed

Fleet's documented bootstrap config sets `tls.insecure_skip_verify: true`,
which sends the shared enrolment token over an unverified connection to a public
Elastic Cloud endpoint. **This is unnecessary.** Verified working with
`insecure_skip_verify: false` and `ca_file:
/etc/ssl/certs/ca-certificates.crt`: the extension started cleanly, no x509
errors, and the agent continued reporting. Elastic Cloud presents a publicly
trusted certificate. **ADR-0003 should say the insecure default is not required
rather than "replace before production".**

### Other observations

- Fleet has **no discovery**. Enrolment is opt-in with an extension plus a valid
  token, confirming ticket 02 question 5 and bounding the `ungoverned` quadrant.
- Fleet uses the collector's own `service.name` as `agent.type`, here
  `k8s-node-telemetry`, and `identifying_attributes` carries `service.name`,
  `service.instance.id` and `service.version`.
- Fleet is **HTTP only** for OpAMP, while Interchange's endpoint is a plain
  WebSocket. Both worked simultaneously from one collector.
- The PoC runs `otelcol-contrib`, which ticket 03 warned would fail Fleet's
  agent version gate. It enrolled and reported fine over the API. UI degradation
  was not assessed, since Kibana is behind SSO.

### State left behind

- **PoC running and healthy**, all pods up, `collection_interval` restored to
  its original 30s. The `opamp/fleet` extension, the pinned uid and the TLS fix
  remain in the Baseline, since they are the spike's product.
- Changes on branch **`spike/fleet-coexistence`**. The enrolment token is in a
  Kubernetes Secret created out of band, never in git.
- **In Fleet**: agent policy `interchange-otel-collectors`
  (`a8795656-5189-499e-8f6f-f17f9d4c3466`), one enrolment key, and **three agent
  records**, two of them orphans from before the fix. Left in place as evidence.
  Worth unenrolling before any demo.
