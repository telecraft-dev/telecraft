# What does mandating the Supervisor cost, and does the serve half survive it?

Type: grilling
Status: resolved
Blocked by: none (23 resolved 4 August 2026)

## Question

Raised by ticket 11, which restored premise 7 in a narrower form: **git stores,
OpAMP serves.** That decision has one price and it was deferred rather than paid.

**The in-process `opamp` extension cannot accept remote config.** Ticket 01
established this firmly: `toAgentCapabilities()` cannot advertise
`AcceptsRemoteConfig` and has no config key to add it. So **every collector
Amp-Up serves needs the Supervisor beside it**. That is not an optional extra, it
is a second process in the unit of deployment.

Ticket 09 closed this as moot, because nothing was pushed. It is live again.

**Split on 4 August 2026, and the research half is now resolved.** This ticket
originally carried six questions, three of which were enumeration with nothing to
decide. Those became [ticket 23](23-supervisor-cost-enumeration.md), resolved the
same day. **Read `research/23-findings.md`, or at minimum ticket 23's `## Answer`,
before opening this ticket.** What remains here is the decision, and it needs the
user.

**What the research changed, in one paragraph.** The fear that framed this ticket
was wrong: the Supervisor is **best packaged exactly where serving is justified**,
on VMs and bare metal, with signed distro packages, a Windows MSI and
OpenTelemetry's own blueprint calling it "the most capable integration". **The
gap is Kubernetes**, which has nothing upstream at all. **Premise 9 is confirmed
true rather than aspirational**: no TTL, no retry ceiling, no path from connection
state to collector state, so a served collector rides out an outage indefinitely
and restarts on cached config. **Premise 8 is not satisfied and has no path to
being satisfied**: alpha since creation, no beta issue, no graduation criteria,
and six vendors chose the embedded client instead, though two ship or require the
Supervisor and one took it to GA. **No prior art exists**, so building the server
is defensible under premise 8, with the caveat that nobody has built a stateless
read-from-an-external-store OpAMP server and the reason may be that everybody who
serves config concluded they also want to store it.

Questions to settle:

1. **Does the serve half survive the price?** This is the ticket. Ticket 23
   returned the bill; this decides whether to pay it. **The "adopt an existing
   server" outcome is now dead**: ticket 23 found no prior art in any state, and
   no pluggable config source to contribute one to. Two outcomes remain: serve as
   decided in ticket 11, or **render to git and ship no server at all**, which is
   where ticket 09 landed before the user reopened it.

   **A third has appeared, and it is the one to put to the user first.** Ticket 23
   observed, without deciding, that **serving is opt-in per collector**, so the
   Kubernetes gap argues against serving *Kubernetes* collectors, where GitOps
   already works, rather than against serving at all. That would mean Amp-Up
   serves the VM and bare-metal half GitOps cannot reach and renders to git for
   everything else. **Whether that is a coherent product or an awkward one is the
   judgement this ticket exists to make**, and ticket 23 was explicit that it
   could not settle it.

   **Weigh the largest unknown honestly.** There is no adoption data for the
   Supervisor at all, and no long-running-stability defect in the tracker in
   either direction, which is equally compatible with a clean component and with
   nobody running it long enough at scale to find out. Given six vendors chose the
   other shape, ticket 23 judged the second at least as likely.

2. **What storage does Amp-Up specify for the Supervisor's own state?** Ticket 06
   found Interchange accumulates duplicate agents on every DaemonSet rollout
   because that state sits on an `emptyDir`, and the collector mints a fresh
   instance uid per restart. If Amp-Up mandates the Supervisor it inherits the
   defect, so it has to specify the storage rather than leave it to the adopter.
   Ticket 23 confirmed the mechanism from the Supervisor's side: **identity is one
   UUID in one file, so `agent::instance_id` is a seed rather than a pin.** The
   decision is what Amp-Up requires, and whether it is a documented requirement, a
   rendered default, or something the platform refuses to run without.

   **Ticket 23 surfaced a second storage case this ticket should settle with it.**
   First boot with no cache and no local config yields a **healthy** collector
   running a `nop` pipeline. That is silent nothing rather than a visible failure,
   and it is a conformance-reporting problem as much as a storage one: decide what
   Amp-Up shows for a collector in that state.

3. **Can an adopter mix?** Some collectors served over OpAMP, some delivered by
   Argo or SSM, one estate, one Amp-Up. Ticket 11's design assumes yes, since the
   commit stamp works identically for both populations. Confirm nothing else
   breaks: the `EstateProvider` keying, the conformance verdict, the delivery
   status composition, and whether the two populations are distinguishable to a
   user who needs to know why one is drifting.

4. **If the answer is "serve", what is the unit of adoption?** Ticket 11 changed
   it from "point Amp-Up at your collectors" to "run a second process beside every
   collector you want served". That is a materially different sell, and ticket 10
   is blocked on this ticket precisely because the positioning changes with it.
   Decide how it is stated honestly.

**What could overturn what.** The honest outcome may still be that Amp-Up
**renders to git and ships no server at all**. That would leave the foreign
population as the only population, and `Declared` would come solely from
ElasticFleet or nothing. Ticket 11's design survives that, because the commit
stamp was deliberately built to work without a server. **So this ticket can
safely conclude against serving without invalidating the reading model.**

Ticket 23 came back genuinely split rather than decisive, and stated both cases
in full. **Do not read it as pointing one way.** Its four strongest arguments for
serving are the packaging fit, premise 9 holding by construction,
GrafanaFleetManagement having taken the same path to GA, and the serve half being
99 lines. Its four strongest against are premise 8 being unsatisfiable, the market
having split toward the embedded client, Kubernetes having nothing at all, and the
absence of prior art cutting both ways.

This ticket gates 10, 12 and 22.

Read tickets 01, 09, 11, 13 and 17, and `research/23-findings.md`, before
starting.

## Answer

Resolved 5 August 2026, by grilling. Evidence: ticket 23 and
[research/23-findings.md](../research/23-findings.md).

**The serve half survives, and it survives wider than recommended.** The
recommendation put was the hybrid: serve the VM and bare-metal population,
render to git for Kubernetes on the grounds that GitOps already works there.
**The user rejected the framing and was right.** Ticket 20's own numbers refute
it: only **22% say nearly all** of their deployment is GitOps and **53% say some,
just beginning, or not started**, measured in a cloud-native-engaged population,
so that is an upper bound. "Kubernetes, where GitOps already works" was doing
work the evidence does not support. Not every Kubernetes cluster runs GitOps, and
flexibility is the point.

1. **Amp-Up serves everything, and GitOps is an alternative rather than a
   fallback.** One artefact, plain otelcol YAML at a stable repo path, and two
   ways to move it, chosen per collector by the adopter. This sits cleanly on
   premise 11: the lens was already applier-agnostic, so the OpAMP server becomes
   one delivery option among several rather than a second product. **The "adopt
   an existing server" outcome is dead** on ticket 23's evidence: no prior art in
   any state, and no pluggable config source to contribute one to.

2. **Amp-Up publishes configurations and no images.** Serving Kubernetes puts
   ticket 23's Kubernetes gap on the critical path, since upstream ships nothing
   for running the Supervisor there. A published Supervisor-plus-collector image
   was proposed and **rejected under premise 3**: that is a binary in the data
   path, distributed by Amp-Up, carrying a registry, CVE patching and a version
   matrix against a biweekly upstream train. Rendering a DaemonSet manifest was
   also rejected, as still over-reach.

   **Amp-Up publishes two configurations and stops there.** The rendered otelcol
   YAML, and **`supervisor.yaml`**, which is small: server endpoint, capability
   flags, identifying attributes, executable path, storage directory. Interchange's
   is 25 lines (`poc/supervisor/node.yaml`). How the Supervisor binary reaches the
   host is the adopter's concern, documented and not owned. The combined-image
   shape is the only one upstream ships, and Interchange proves it in about ten
   lines (`testbed/edge/Dockerfile`), so it is cheap to document.

   **Known cost, accepted:** a Kubernetes adopter must supply an image before they
   can be served, which is a higher barrier than `helm install`, and it is exactly
   where such an adopter is most likely to choose GitOps instead. That is now a
   legitimate path rather than a failure, which is what makes the cost bearable.

3. **`CollectorID` is derived from reported identifying attributes, not from
   `instance_uid`.** Ticket 11 left `CollectorID string` undefined and this fills
   it. The user delegated the choice; the reasoning is recorded here rather than
   asserted. Identity comes from what the collector reports about itself, matched
   against selectors held in git, which is the mechanism ticket 07 already chose
   for Stage matching. **`instance_uid` stays OpAMP's connection key and is never
   surfaced as identity.**

   Three consequences. The loop closes with **no stored state**: a collector
   connects and reports attributes, Amp-Up matches them against git, reads the
   config at that path and serves it, remembering nothing. **Both `EstateProvider`
   implementations land on one key**, which matters because a foreign collector
   seen through ElasticFleet has attributes but no Amp-Up-controlled uid. And
   **ticket 06's `emptyDir` defect stops being load-bearing**: a churning uid costs
   duplicate rows in a view, never a wrong config or a wrong verdict.

   **The renderer must emit a node-unique attribute**, via the Downward API
   reference on Kubernetes, rather than leaving adopters to rediscover ticket 13's
   defect. A DaemonSet renders one manifest for all nodes, so without it every pod
   reports identical attributes. This is a renderer requirement, not documentation.

4. **Storage: Amp-Up specifies the path and documents durability, and enforces
   nothing.** `supervisor.yaml` carries `storage.directory`. Whether that path
   survives a restart is a volume decision in a manifest Amp-Up no longer writes,
   so it **can specify but cannot enforce**. Given point 3, that is acceptable.
   Detection of identity churn is possible server-side and is deferred as a
   cosmetic finding, to be revisited only if the noise proves to matter.
   Interchange's own fix is on record as the documented pattern:
   `poc/edge-node.yaml` moved the state from `emptyDir` to a `hostPath`, making
   identity node-lifetime.

5. **Delivery path is a visible property of a collector, not an implementation
   detail.** This extends ticket 11's own reasoning, that delivery status sits
   beside the conformance verdict because "today a delivery fault and a pipeline
   fault are the same finding sent to the wrong person". Served and git-delivered
   collectors have **different remedies**: one is actionable inside Amp-Up, the
   other belongs to Argo, Flux, SSM or a person. A drift finding without that
   attached is not actionable. **Accepted cost:** the two-path design is visible
   on day one and the product cannot present itself as simpler than it is.

6. **Mixing is the default mode, settled by construction.** Serve-everything plus
   GitOps-as-alternative makes mixed estates normal rather than tolerated, and
   ticket 20 supports it directly: **50% of fleets over 100 collectors span both
   Kubernetes and VMs.** Attribute-derived keying puts both populations on one key,
   and the commit stamp already worked identically for both.

### The positioning, which ticket 10 inherits

Stated by the user: **Amp-Up is the library and the front door to the policies
that exist, and you should not need Amp-Up for observability orchestration to
work.** Scoped, because the unscoped version is refutable and ticket 04 already
forced one such claim to be retired:

- **Not a dependency of telemetry flow.** Premise 3, nothing in the data path.
- **Not a dependency of the record.** Every rendered config is in git regardless
  of delivery path, with the history, the rollback and the approval trail. Delete
  Amp-Up and all of it is still there, deliverable by any other means.
- **A dependency of delivery only while it is the chosen channel.** Ticket 23
  proved the qualification is survivable: no TTL, no retry ceiling, no code path
  from connection state to collector state, so collectors keep running last-good
  config indefinitely and restart onto the cached one.

**Recorded as decided, softly.** The user's confirmation was "sure". The scoping
is defensible line by line and each clause is evidenced, but ticket 10 should feel
free to reword it rather than treat it as fixed.

### What this pushed onto the rest of the map

- **Premise 8's two halves are reconciled**, and the reconciliation is: build,
  knowingly, on an alpha dependency. The enumeration half is satisfied and points
  at building, because no OpAMP server anywhere reads from git and none has a
  pluggable config source. The maturity half is not satisfied and has no path to
  being satisfied. **The trade is accepted with eyes open**, and ticket 23's
  largest unknown stands unresolved: there is no long-running-stability evidence
  for `opampsupervisor` in either direction.
- **Premise 3 is load-bearing rather than decorative.** It decided the image
  question. "Ships no gateway" generalises to "ships configurations, never
  binaries".
- **Ticket 08 is partly decided**, see below.
- **Tickets 10, 12 and 22 unblock.** Ticket 10 inherits the positioning above.
  Ticket 12 keeps two `EstateProvider` implementations, since serve-everything
  does not remove the foreign population. Ticket 22 now has a harder job, because
  staged rollout must work across two delivery paths rather than one.

### Handed to ticket 08

The user decided, in this session, that **Amp-Up does not own the register**, and
the reasoning is recorded on ticket 08 rather than here. It arose from a case this
ticket could not close: a brand new collector that has never reached Amp-Up runs a
`nop` pipeline while reporting **healthy**, and Amp-Up cannot report a collector it
does not know exists. Checked for an OpenTelemetry-native answer first, at the
user's prompting, and there is none: OTel has no register concept, and ticket 04
already verified that absence alerting everywhere is hand-configured and "derived
from nothing".
