# What does the Supervisor cost to run, and does a git-backed OpAMP server already exist?

Type: research
Status: resolved
Blocked by: none
Blocks: 21

## Question

Split out of ticket 21 on 4 August 2026. Ticket 21 mixed enumeration with a
decision. This is the enumeration half, and it has to land before the grilling in
21 is worth holding, because three of 21's six questions were facts nobody had
gone and got.

**Why this matters.** Ticket 11 restored premise 7 in a narrower form: **git
stores, OpAMP serves.** The price was deferred rather than paid. The in-process
`opamp` extension **cannot accept remote config** (ticket 01:
`toAgentCapabilities()` cannot advertise `AcceptsRemoteConfig` and has no config
key to add it), so **every collector Amp-Up serves needs the Supervisor beside
it**. That is a second process in the unit of deployment, and it is unpriced.

If the answers come back expensive enough, the honest outcome is that Amp-Up
renders to git and **ships no server at all**, which is where ticket 09 landed
before the user reopened it. So do not write toward a conclusion. Ticket 11's
design was deliberately built to survive either verdict.

Read tickets 01, 09, 11, 13, 17 and 20 first, plus the map's premises 7, 8, 9
and 11. Premise 8 is binding on question 4 and premise 14 on all naming.

### 1. What is the actual deployment delta of mandating the Supervisor?

Price it per substrate, concretely: what an operator has to add, install, run and
keep running that they do not today.

- **Kubernetes.** Sidecar container, or the Supervisor as the container entry
  point with the collector as a child process? Which does upstream actually do,
  and what does the OTel Operator support, if anything?
- **VMs.** Second binary, second systemd unit, or a supervisor that owns the
  collector process. What does the packaging story look like: is there a distro
  package, a container, or is it "go build it"?
- **Bare metal**, and anything without a package manager.
- **Windows**, if it is supported at all.

**Ticket 20's numbers are the sharp point here: 51% of collector deployments
include VMs and 18% include bare metal, and reaching exactly those is the whole
reason to serve at all.** If the Supervisor is hardest precisely on the substrate
that justifies serving, the argument for serving weakens sharply. State plainly
whether that is the case.

### 2. What is the maturity risk of `opampsupervisor`?

Premise 8 prefers CNCF Graduated or stable upstream components over alpha ones,
and this is the component that would own the config file on every served host.

- Current stability level as declared upstream, and the release cadence. Ticket
  09 corrected an earlier "abandoned" reading: the OpAMP spec is actively
  released, v0.19.0 published 2026-08-03. Establish the same for the Supervisor
  itself, which is a separate artefact from the spec.
- What is the open defect profile? Look for the classes that would bite a
  product mandating it: config corruption, restart loops, state loss, identity
  churn, memory growth.
- **Does anyone run it in production, and does any vendor ship it supported?**
  A vendor-supported build changes the adoption calculus more than the upstream
  stability label does.
- What is the declared path to beta or stable, if one exists.

### 3. What does the Supervisor persist, where, and what does it do when the server is unreachable?

Premise 9 says Amp-Up is **not a dependency of telemetry flow**. That claim is
currently unverified for the served population, and it is the load-bearing claim
of the whole serve half.

- **What exactly does the Supervisor write to disk**, at what paths, and which of
  it is identity, which is last-known config, which is transient?
- **Across a server outage**: does the collector keep running the last-good
  config, and for how long? Is there a TTL, a retry ceiling, or a fallback to a
  bootstrap config?
- **On restart with no server reachable**: does it come up with the last-good
  config, an empty config, or not at all? This is the case that decides whether
  premise 9 is true or aspirational.
- **What is the identity behaviour across restart?** Ticket 06 found the
  Supervisor's own state on an `emptyDir` makes Interchange accumulate duplicate
  agents on every DaemonSet rollout. Establish the mechanism from the
  Supervisor's side: what file holds the instance uid, and what is upstream's
  documented guidance on persisting it.

Return the facts here. The decision about what storage Amp-Up specifies stays in
ticket 21.

### 4. Does an OpAMP server that serves config from a git repo already exist?

**Premise 8 binds hardest here: a decision to build must defend itself in
writing.** Shipping a server is a build. Enumerate what exists, price adoption
against building, and record the alternatives even if building wins.

Ground already covered, do not redo it: ticket 17 established the OTel Operator's
OpAMP bridge is an OpAMP **client** that requires a server, and the server it
embeds is deliberately read-only, with `standalone` mode unreachable through the
`OpAMPBridge` CRD. Start from there.

Enumerate at least:

- The two personal repos of a maintainer that the bridge README points at as
  server examples. What are they, what state are they in, what licence?
- `opamp-go`'s own `server` package and any reference server in that repo.
- **Bindplane's server.** Ticket 09 skipped Bindplane as a delivery target for
  lacking a raw-YAML resource. That is a different question from whether its
  OpAMP server is adoptable. Licence, and whether config can come from git.
- **Grafana Fleet Management.** Ticket 09 has it as a delivery target with
  `config_type = "OTEL"`. Is it also an OpAMP server, is it self-hostable, and
  what is the licence?
- Anything in the CNCF landscape, the OpAMP spec's own implementations list, or
  vendor products that reads a git repo and serves it over OpAMP. Include
  proprietary ones and say so.
- Adjacent shape: any OpAMP server that serves from **any** external store, since
  a pluggable-source server plus a git source is a smaller build than a whole
  server.

For each: does it serve **arbitrary rendered YAML**, is it self-hostable, what is
its licence and governance, and what would adoption actually cost against the
build?

**The build baseline for comparison**: Interchange's
`testbed/control-plane/main.go` is an existence proof that a serving OpAMP server
is small, and `opamp-go`'s `server` package means the work is policy and state
rather than wire protocol. That is an argument for building only once the
alternatives are on the table.

## Return

Findings to `.scratch/ampup-product-shape/research/23-findings.md`, matching the
convention of the existing files there. Every claim cited to a primary source:
upstream docs, source code, the OpAMP spec, first-party APIs. Follow claims back
to the source that owns them rather than a write-up of them.

Open each of the four sections with a verdict line, then the evidence. Say where
the evidence is thin or absent, and do not fill a gap with inference presented as
fact. Ticket 20's findings file is the model.

Close with a plain statement of **whether the serve half of "git stores, OpAMP
serves" survives the price**, framed as evidence for the decision in ticket 21
rather than as the decision itself.

**No em-dashes.** Periods, colons or commas.

## Answer

Resolved 4 August 2026. Full findings with citations:
[research/23-findings.md](../research/23-findings.md). Four repos cloned and read
at named commits, all dated 2026-08-04.

**The ticket's own sharp point does not land, and that is the headline.**

1. **The deployment delta is smallest exactly where serving is justified, and
   largest where it is not.** The fear was that the Supervisor would be hardest
   on the VM and bare-metal population that is the whole reason to serve. The
   opposite is true. VMs and bare metal get signed deb and rpm packages with a
   systemd unit, a Windows MSI that registers a service, six platform binaries,
   cosign signatures and SBOMs, on a 53-release biweekly train. **OpenTelemetry's
   own non-Kubernetes blueprint recommends OpAMP for that population and calls
   the Supervisor "the most capable integration".** Kubernetes has **nothing**:
   zero grep hits for "supervisor" across the OpenTelemetry Operator and the
   OpenTelemetry Helm charts, requested since 2022-12-12 and unbuilt, and zero
   mentions of Kubernetes, sidecar, container or DaemonSet in the Supervisor's own
   980-line design spec or its 4,115-line e2e suite. **The upstream pattern is
   entry-point-with-child in one container, not a sidecar**, and a sidecar is
   structurally awkward rather than merely unbuilt: the Supervisor forks the
   collector, signals it directly, and injects `ppid` so the collector exits when
   its parent dies. Interchange already runs the combined-image shape in about ten
   lines (`testbed/edge/Dockerfile`), which is a live existence proof. **The
   universal cost is that the Supervisor takes over the collector process.** That
   is a migration, not an addition, and nobody has automated it.

2. **Premise 8 is not satisfied, and there is no path to satisfying it.** Alpha
   since creation. The alpha tracking issue closed 2025-04-04 with "not
   necessarily something that we intend for production usage", **no successor beta
   issue**, no milestone on any of the 19 open issues, no graduation criteria, and
   the Supervisor-to-collector compatibility policy still undefined (#48739).
   Breaking config changes still landing, one on 2026-08-04. Five-person
   contributor bench. **Against that: GrafanaFleetManagement took an OTel
   collector path to GA around July 2026 that requires the upstream Supervisor,
   documented with no stability caveat, and Coralogix ships its own build as a
   systemd service.** But **six vendors built the embedded in-collector client
   instead**, including Bindplane, who maintain `opamp-go` and steer production
   users toward their own agent. That is a market split, not a consensus.

3. **Premise 9 holds by construction, not by luck, and upstream tests it.** No
   TTL, no retry ceiling (`MaxElapsedTime = 0`, so it retries forever), and **no
   code path at all from connection state to collector state**. A Supervisor whose
   server has been gone a week is still running the last-good config. On restart
   with the server unreachable it comes up on the cached config, covered by
   upstream e2e tests. **Two teeth.** First boot with no cache and no local config
   yields a *healthy* collector running a `nop` pipeline, which is silent
   nothing rather than a visible failure. And **identity is one UUID in one file**,
   so `agent::instance_id` is a seed rather than a pin. That is ticket 06's
   duplicate-agent defect seen from the Supervisor's side, and it confirms Amp-Up
   must specify durable storage rather than leave it to the adopter. The decision
   about what it specifies is ticket 21's.

4. **No prior art exists, in any state, and there is nothing to contribute to
   either.** The OpAMP spec's own implementations list names exactly two Agent
   Management Platforms, both proprietary: Bindplane and GrafanaFleetManagement.
   The CNCF landscape has zero matches for "opamp". Nineteen implementations were
   enumerated and priced. The two personal repos the OpAMP bridge README points at
   are one-person spikes with no release, no CI and no consumers, one dead since
   January 2023. `opamp-go`'s example server is unimportable (`internal/`) and
   loses agent state on disconnect; reading config from a file is an **open,
   unimplemented request** upstream (#456). **No OpAMP server anywhere has a
   pluggable config source**, so "contribute a git source" is not available.
   **Under premise 8, building is defensible because the alternatives are
   genuinely absent.** The defence has one weakness worth stating: three projects
   have shipped an OpAMP server that deliberately refuses to serve config, nobody
   has shipped one that serves from an external store, and the reason may be that
   everybody who serves config concluded they also want to store it.

   Two corrections to earlier tickets fall out. **Ticket 09's reason for skipping
   Bindplane needs qualifying**: `spec.raw` does pass otelcol YAML through
   byte-for-byte and the live OpenAPI schema still exposes it, so the accurate
   statement is "no raw-YAML resource **in the Terraform provider**". The decision
   stands; only the reason was too broad. And **Bindplane's server was never
   relicensed**: `observIQ/bindplane-op` returns a hard 404 with no redirect, and
   the open-source line simply stopped at v1.35.0 under genuine Apache-2.0.

**The build baseline, measured.** 99 lines of serving inside Interchange's
499-line `testbed/control-plane/main.go`, over a 755-line Apache-2.0 `opamp-go`
`server` package, four runtime modules. With no auth, no TLS and no durable state,
none of which is counted.

### What this does not settle

The findings are explicit that they are evidence for ticket 21, not its decision.
Both cases are stated in full there. Two non-signals are called out and should not
be read as evidence: **there is no adoption data for the Supervisor at all**, so
production use is inferred from vendor behaviour; and **there is no
memory-growth or long-running-stability defect in the tracker in either
direction**, which is equally compatible with a clean component and with nobody
running it long enough at scale to find out. Given the vendor split, the findings
judge the second at least as likely, and name it the single largest unknown.

**One reframing is offered and deliberately not decided.** Serving is opt-in per
collector. The population that justifies serving is the VM and bare-metal half
GitOps cannot reach, and that is precisely where the Supervisor is best packaged.
**So the Kubernetes gap is an argument against serving Kubernetes collectors,
where GitOps already works, rather than against serving at all.** Whether that
split is a coherent product or an awkward one is a judgement about Amp-Up's
shape. It belongs to ticket 21.
