# Can one collector report to two OpAMP servers?

Type: research
Status: resolved
Blocked by: none

## Question

ADR-0003 rests on an unverified reading of two mechanisms: that the in-process
`opamp` **extension** only reports (health, status, effective config) and cannot
apply config, while the **Supervisor** is a separate process that receives
config, writes it, restarts the collector and reverts on failure. The ADR
concludes that pointing the extension at Elastic Fleet costs nothing
architecturally because the Supervisor keeps pointing at Interchange.

Establish from upstream sources whether that reading holds. Specifically:

1. Does the OpAMP **specification** forbid, permit or simply not address one
   agent maintaining connections to two OpAMP servers?
2. Does the Collector's `opamp` **extension** implementation support more than
   one server endpoint in its configuration, or exactly one?
3. When the **Supervisor** manages a collector, does it inject or manage the
   collector's own `opamp` extension block? If it does, a hand-added extension
   pointing elsewhere may be overwritten on the next config push, which would
   defeat ADR-0003 quietly rather than loudly.
4. Does the Supervisor itself report `EffectiveConfig` and health to *its* OpAMP
   server? If so, a control plane gets declared state over the connection it
   already owns, and the extension is free to point at Fleet.
5. What `AgentCapabilities` and `ServerCapabilities` flags govern effective
   config reporting, and which side must advertise what?

This is the cheapest possible test of ADR-0003's central claim, and question 4
also bears directly on where Amp-Up's declared reading comes from.

**Context worth carrying in.** Interchange's control plane is already an OpAMP
server at `testbed/control-plane/main.go`. At line 139 it advertises only
`AcceptsStatus | OffersRemoteConfig`, and `onMessage` never reads
`msg.EffectiveConfig`. If question 4 resolves in the affirmative, that is a
small change with large consequences for both halves of the map.

## Answer

Resolved 4 August 2026. Full findings with citations:
[research/01-findings.md](../research/01-findings.md).

**ADR-0003's central claim holds, with two qualifications and one dependency
that only the testbed can settle.**

1. **The spec does not address two servers per agent.** No prohibition, no
   explicit permission. The only normative cardinality statement is that a
   Client "SHOULD connect no more than once to *the* Server", which is scoped to
   one Server and is about duplicate connections and `instance_uid` collision.
   The spec contemplates *redirecting* the single connection, never two
   concurrent ones, and no text defines what happens when two Servers disagree.
   So this is unspecified territory rather than sanctioned behaviour.

2. **The extension takes exactly one endpoint.** `Server *OpAMPServer` is
   singular, and `ws` and `http` are mutually exclusive and validated. Two
   *named* instances, `opamp` and `opamp/fleet`, fit the collector's `type/name`
   component model, but **no upstream doc, test, example or issue establishes
   that as supported**. That is the weakest link in the whole chain.
   Also confirmed, and this is the ADR's key premise: `toAgentCapabilities()`
   cannot advertise `AcceptsRemoteConfig` and has no config key to add it, so the
   extension genuinely cannot be offered remote config.

3. **The Supervisor does inject the extension block, but the feared silent
   overwrite does not happen.** It injects `extensions.opamp` pointed at
   `ws://127.0.0.1:<port>/v1/opamp` plus `service.extensions: [opamp]`.
   Critically, `service::extensions` is merged by **concatenation and
   de-duplication, not replacement**, so a remote config adding `opamp/fleet`
   yields `[opamp, opamp/fleet]`. Verified in `main`, in released v0.157.0, and
   back to v0.120.0.

4. **The Supervisor reports EffectiveConfig and health to its own server, both
   on by default**, and it **forwards the collector's own reported effective
   config upward**. Defaults are `ReportsEffectiveConfig: true`,
   `ReportsHealth: true`, `AcceptsRemoteConfig: false`.

5. **Capability flags, with a trap.** The Agent sets `ReportsEffectiveConfig`
   and the Server sets `AcceptsEffectiveConfig`, separate enums, same bit value.
   The spec MANDATES that an Agent stop using capabilities the Server does not
   advertise, but **opamp-go's client never inspects `ServerCapabilities`**, so
   the data flows regardless. Consequence for Interchange: the Control Plane at
   `testbed/control-plane/main.go:139` **is already receiving effective config
   and discarding it**. That is stronger than the charting-session reading, which
   said it was one capability away. Advertise the bit anyway rather than depend
   on a client not implementing a MUST.

**Qualification A, and it is load-bearing: the second extension must not be
named `opamp`.** `$REMOTE_CONFIG` merges *after* `$OPAMP_EXTENSION_CONFIG`, so
any values under `extensions.opamp` override the Supervisor's injected endpoint
and break the Supervisor link. The Supervisor design doc claims "the extension's
configuration cannot be overridden by the remote configuration". **No code
enforces that claim.** Name it `opamp/fleet`.

**Qualification B: "only reports" is imprecise.** `accepts_restart_command` plus
the `RemoteRestarts` feature gate lets the extension's server SIGHUP the
collector into re-reading `--config`. Both are off by default. The exact phrasing
is "cannot accept remote config", and ADR-0003 should use it.

**Not established: two concurrent `opamp` extension instances in one collector.**
Permitted by the component model, no shared state in the factory, zero upstream
evidence either way. Settle it in the testbed, not by reading more code. This is
ticket 06's single most important question.

**The finding that reshapes the map.** Since the Supervisor already reports
effective config and health on the connection Interchange owns, the argument
that the extension must be spent on Fleet in order to get visibility is weaker
than ADR-0003 assumes. Carried into ticket 13.
