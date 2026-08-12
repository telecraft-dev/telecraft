# ADR-0003: accept, reject or supersede

Type: grilling
Status: resolved
Blocked by: 01, 03, 06

## Question

The one Interchange decision on this map. Decided on Interchange's own terms.
Amp-Up has no bearing on it: premise 2 holds that Interchange's delivery is
unchanged and Amp-Up is optional.

`docs/adr/0003-fleet-visibility-split-channel.md` is `status: proposed`. It
argues Managed Edge Collectors should report state to Elastic Fleet while
continuing to take config from the Interchange Control Plane. Two channels, two
roles: Fleet answers what the fleet is doing, Interchange answers what it is
allowed to collect. The ADR is explicit that the coexistence is unverified and
"a reading of the two mechanisms, not something proven".

**A finding from charting that the ADR does not account for.** The Control Plane
is already an OpAMP server. At `testbed/control-plane/main.go:139` it advertises
only `AcceptsStatus | OffersRemoteConfig`, and `onMessage` never reads
`msg.EffectiveConfig`. Advertising `AcceptsEffectiveConfig` and reading the
field would give Interchange the reported effective config directly, over the
connection it already owns.

If that holds, and ticket 01 should establish whether it does, then Fleet is not
a **source** of anything Interchange lacks. It is a **console**. That does not
kill the ADR: an Elastic-native view over the whole collector estate is real
value for Customer C, and Fleet sees collectors Interchange does not manage. But
it demotes the decision from architectural to presentational, and the ADR should
say so rather than resting on an argument that no longer stands.

Questions to settle:

1. Does the coexistence hold, per tickets 01 and 06? If not, the ADR is dead as
   written and the alternative has to be chosen from the three it already lists.
2. Does the `AcceptsEffectiveConfig` route work, and does it change the verdict?
3. Is the **drift check** worth building? The ADR claims reported and rendered
   config "should be byte-identical, and an alert on divergence makes ADR-0001
   self-verifying rather than merely asserted". Ticket 06 question 4 captures
   whether they actually are. If they are not byte-identical, the drift check
   needs a semantic comparison and the claim needs rewriting.
4. Does Preview status, per ticket 03, make this a v1 commitment or a v2 one?
5. If accepted, does the `extensions` block become a **mandatory stamp** in
   `render()`, putting fleet visibility in the Observability Baseline rather
   than leaving it an elective a workload owner can decline? That is what the
   ADR proposes and it is a real change to the renderer.
6. `tls.insecure_skip_verify: true` in Fleet's generated bootstrap config. What
   replaces it, and does it land with or before the mTLS work already planned
   for the OpAMP endpoint?

## Sharpened by ticket 01

Ticket 01 is resolved. **The central claim holds**, so question 1 is provisionally
yes, pending ticket 06 settling the one thing code reading cannot.

- **The ADR's premise is confirmed**: the extension's `toAgentCapabilities()`
  cannot advertise `AcceptsRemoteConfig` and has no config key to add it, so it
  genuinely cannot be offered remote config.
- **Two corrections the ADR must absorb.** First, the second extension must be
  named `opamp/fleet`; a block named `opamp` overrides the Supervisor's injected
  endpoint, because remote config merges after it, and no code prevents this
  despite the Supervisor design doc claiming otherwise. If the ADR is accepted
  and the extension becomes a mandatory stamp in `render()`, the renderer must
  enforce the name. Second, "only reports" is imprecise: with
  `accepts_restart_command` and the `RemoteRestarts` gate, both off by default,
  the extension's server can SIGHUP the collector. Write "cannot accept remote
  config", which is exact.
- **The visibility argument is weaker than the ADR states, and this is the
  substantive point.** The Supervisor already reports effective config and health
  to its own server by default, and forwards the collector's own effective config
  upward. Interchange's control plane is therefore **already receiving effective
  config and discarding it**, which is stronger than the charting note that it was
  one capability flag away. So Fleet supplies no declared state Interchange
  cannot already have. The honest case for ADR-0003 is a console and an
  Elastic-native estate view, not access to data. Rewrite the "Where each console
  owns the answer" table accordingly: the row claiming Fleet answers "what config
  is it actually running right now" is no longer a reason to adopt Fleet.
- **The spec is silent** on two concurrent servers, and two concurrent extension
  instances are unestablished upstream. Whatever the verdict, the ADR should
  record that it rests on unspecified behaviour rather than a sanctioned pattern.

## Sharpened by ticket 03

Ticket 03 is resolved. It answers question 4 and adds a question the ADR does
not currently ask.

- **Question 4 is answered, and the answer is harder than "Preview".** No public
  GA commitment exists, the GA work is unmilestoned, and the 9.4 launch post does
  not mention the feature. Do not read the Observability Labs "production-ready
  today" line across: it refers to Elastic Agent in OTel mode under Fleet's own
  protocol, a different capability.
- **Tighten the ADR's caution into a permanent property.** The ADR says v1 must
  not build enforcement on Fleet, which implies v2 might. It cannot. Fleet
  Server is monitoring-only, and even after Elastic ships more, enforcement stays
  collector-opt-in because the upstream Supervisor does not advertise
  `accepts_remote_config` by default. "Config authority stays with Customer C"
  is not a v1 concession, it is durable. That strengthens the ADR's own argument
  and it should be rewritten to say so.
- **A new question the ADR does not ask: is the visibility itself trustworthy?**
  `fleet-server#6820` freezes agent identity at enrolment and silently ignores
  later `AgentDescription` updates. The ADR's whole value proposition is that
  Customer C sees its estate in Kibana. A console that confidently shows stale
  hostnames and versions, with no API to correct them, is a different and worse
  proposition than the one the ADR argues for. Whether reported **effective
  config** goes stale the same way is unestablished, and ticket 06 can settle it.
  If it does, the drift check in question 3 is impossible and the ADR's
  self-verifying claim fails outright.
- **The PoC runs `otelcol-contrib`, which Fleet supports but degrades**: it fails
  Fleet's Elastic Agent version gate and loses UI features. If the ADR is
  accepted, the EDOT swap already sitting at position 8 in the handoff's next
  steps stops being optional.
- Fleet is **HTTP-only** for OpAMP transport while Interchange's endpoint is a
  plain WebSocket, and Fleet authenticates with a **shared enrolment token** with
  no per-collector revocation, blocked upstream. Both belong in the consequences
  section alongside the existing `insecure_skip_verify` note, and the second one
  interacts with the planned mTLS work in question 6.
- On Serverless there is **no readable changelog since 30 April 2026** and no
  way to pin a build, so the usual mitigation against a churning preview is
  unavailable on the deployment the PoC actually uses.

Output: ADR-0003 updated to `accepted`, `rejected` with the alternative written
in, or `superseded` with the superseding decision recorded. If accepted, the
consequences section needs correcting for whatever question 2 finds.

## Answer

**Accepted, with the ADR substantially rewritten.** Resolved 4 August 2026. The
decision itself survives intact and is now proven, so this is not a supersede.
The **argument** for it changes from architectural to presentational, and the
ADR now says so rather than resting on a claim that no longer stands.

`docs/adr/0003-fleet-visibility-split-channel.md` is `status: accepted`.

### The six questions

1. **Does the coexistence hold? Yes, proven live.** Ticket 06 ran both extensions
   on one collector, both `StatusOK`, Supervisor still driving config across four
   Baseline pushes. The ADR now records that this rests on behaviour the OpAMP
   spec does not sanction and upstream does not test, and should be re-checked on
   collector upgrades.
2. **Does `AcceptsEffectiveConfig` work, and does it change the verdict? Yes to
   both, and this is the substantive finding.** Verified in code:
   `testbed/control-plane/main.go:139` advertises only
   `AcceptsStatus | OffersRemoteConfig`, and `onMessage` never reads
   `msg.EffectiveConfig`. **Correction to ticket 01**, which claimed Interchange
   is *already receiving* effective config and discarding it. It is not. OpAMP
   gates the agent's send on the server advertising `AcceptsEffectiveConfig`, and
   that flag is absent, so this is one capability flag plus a field read away.
   Cheap, but not free, and the distinction matters to ticket 09, which leans on
   "the declared reading for free". Consequence: **Fleet is a console, not a
   source.** It supplies no declared state Interchange cannot already obtain. The
   "what config is it actually running" row moved from Fleet to Interchange in
   the ADR's console table.
3. **Is the drift check worth building? Yes, but not on Fleet.** Ticket 06
   disproved byte-identity: Fleet redacts on the field name `key`, destroying
   `app.kubernetes.io/name`, which is how `service.name` is derived, and the
   extension's `server` block vanishes entirely. **Build the drift check on
   Interchange's own OpAMP channel**, where the same data arrives unredacted over
   a connection Interchange controls with no preview dependency. This is the same
   one flag and one field read as question 2, so the check is nearly free and
   ADR-0001's self-verification stops depending on the Vendor slot. A Fleet-side
   comparison remains possible but must be semantic against a known redaction
   list, and there is now no reason to prefer it.
4. **Preview status: a durable property, not a v1 concession.** Per ticket 03.
   The ADR's "v1 must not build enforcement on Fleet" implied a v2. There is
   none. Fleet Server is monitoring-only and enforcement stays collector-opt-in
   regardless, because the upstream Supervisor does not advertise
   `accepts_remote_config` by default. Rewritten as permanent.
5. **Mandatory stamp: no. Optional capability, off by default, estate-wide when
   enabled.** This question was first answered "mandatory in every Baseline",
   and that answer was **wrong**. It was reached by offering a false choice
   between a mandatory stamp and a per-workload-owner elective, when the real
   axis was whether the capability is bound to a vendor at all.

   **The correction, on Interchange's own terms.** `CONTEXT.md:50` holds that
   the vendor-agnostic seam lives at the **Gateway's exporters**, and that
   swapping the Vendor is "a Gateway-exporter change, **not an edge change**".
   A mandatory Elastic Fleet endpoint in every rendered edge config is exactly
   an edge change bound to the Vendor slot: swapping the Vendor would then mean
   re-rendering the estate, which is the outcome the seam exists to prevent.
   Interchange must be as backend-agnostic as Amp-Up is.

   **What survives.** The mechanism is generic: the `opampextension` reports to
   any OpAMP server, and endpoint, transport and auth are rendered from
   configuration. Elastic Fleet is the **first and only demonstrated
   implementation**, and a demonstration rather than a requirement. Two binding
   rules: off by default, so a deployment with it disabled renders no extension
   and depends on no console; and no console-specific behaviour may leak into
   the Control Plane, since that would be evidence the seam has moved.

   The completeness argument survives in a narrower form: **when an operator
   enables it, it applies estate-wide** rather than per workload owner, so
   within a deployment that wants it there are still no holes.

   **Neither remaining obligation needs code.** Checked upstream after the fact,
   because the first draft asserted renderer work that does not exist.

   - The extension must never be named `opamp`, since that overrides the
     Supervisor's endpoint and nothing upstream prevents it. This is a Baseline
     authoring convention. A one-line check in `render()` is cheap insurance,
     not a requirement.
   - **`instance_uid` cannot be a renderer concern at all.** The renderer emits
     one manifest for all nodes, so it has no per-node value to compute. The
     Baseline renders `instance_uid: ${env:OTEL_INSTANCE_UID}`, ordinary
     collector env expansion already proven in this same block by the enrolment
     token, and the value comes from the Downward API as `metadata.uid`. Pod
     UIDs are UUIDs and pass the extension's validation. Zero code.
6. **`insecure_skip_verify` is replaced now, and does not wait on mTLS.** Ticket
   06 verified CA verification working against Elastic Cloud, which presents a
   publicly trusted certificate. The ADR now says the insecure default is **not
   required**, rather than "replace before production". The shared enrolment
   token is a credential, so it lives in a Secret created out of band and is
   referenced by environment variable, never rendered into policy, because policy
   is committed to git.

### Also absorbed into the ADR

- Terminology fixed. **Fleet** capital-F means the Elastic product throughout.
  The collector population is the **estate**, never "the fleet". The map uses the
  word both ways and should follow.
- "Only reports" replaced with "cannot accept remote config", which is exact.
- `fleet-server#6820` recorded: agent identity is frozen at enrolment, so
  hostnames and versions in the Fleet view can be stale with no API to correct
  them. Effective config was observed updating, so this bounds the claim rather
  than voiding it.
- Read the recursive health tree, never the roll-up `status`.
- Fleet is HTTP-only for OpAMP; Interchange's endpoint is a WebSocket. Both ran
  simultaneously from one collector.
- No readable Serverless changelog since 30 April 2026 and no way to pin a build.

### Follow-on work this creates, none of it on this map

0. **Add the capability toggle to the Control Plane**, defaulting to off, with
   console endpoint, transport and auth as configuration. Nothing else on this
   list matters until the toggle exists, and it is what keeps Interchange
   backend-agnostic. This is the only code the capability needs.
1. Replace the literal `instance_uid` with `${env:OTEL_INSTANCE_UID}` and add the
   Downward API `metadata.uid` env var to the DaemonSet. Config only.
2. **Fix `supervisor-state` in `poc/edge-node.yaml`: `emptyDir` to `hostPath`.**
   Found while checking the above and unrelated to the visibility capability.
   `persistent_state.yaml` holds the **Supervisor's own** uid, so on an
   `emptyDir` it is lost on pod replacement and **Interchange accumulates
   duplicate agent records on every DaemonSet rollout**. The spike never caught
   this because it only ever restarted the collector child process, which does
   not touch the pod. One-line config fix, worth doing whatever happens to
   ADR-0003.
3. Advertise `AcceptsEffectiveConfig` in the Control Plane and read the field.
   Unblocks the drift check and is small.
4. **Assess the Kibana UI with `otelcol-contrib`.** Ticket 03 warned contrib
   fails Fleet's agent version gate. Ticket 06 confirmed it enrols and reports
   correctly over the API but could not check the UI, because Kibana is behind
   SSO. If the UI is materially degraded, the EDOT swap becomes a precondition of
   promising Customer C a console, not an optional improvement.
5. Unenrol the two orphan agent records left in Fleet as evidence before any
   demo.
