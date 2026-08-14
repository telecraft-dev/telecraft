# Does Amp-Up push config, and how does it reach a collector?

Type: grilling
Status: resolved
Blocked by: 07, 17, 20

## Question

Generating configs graphically is settled. **Delivering** them is not, and the
choice decides whether Amp-Up is side-of-path or a control plane with all the
operational weight that carries.

The spec's section 8 already asks a narrower version of this: "Does the platform
ever push a remediation, or only ever produce the diff and a pull request? Pull
request is safer, keeps it side-of-path, and preserves the audit trail for
free." That was written when the platform only read. It now authors config, so
the question is larger.

Options, not exclusive:

- **Export only.** Amp-Up renders config and writes it to Git, or offers a
  download, and something else delivers it. Keeps every non-goal about the data
  path intact and keeps the audit trail in version control. Also means Amp-Up
  cannot answer "is this collector running what I asked", because it never
  established a channel.
- **Own OpAMP server.** Amp-Up serves config over OpAMP and collectors connect
  to it. This is what Interchange does; the working implementation is
  `testbed/control-plane/main.go`. It makes Amp-Up a runtime dependency of
  config delivery, though not of telemetry flow, and it hands Amp-Up the
  declared reading for free over a connection it already owns.
- **Push through an existing OpAMP server or fleet manager.** Amp-Up renders and
  hands off to whatever is already deployed. Widest compatibility, thinnest
  guarantees, and the most integration surface.

Questions to settle:

1. Which of the three is v1, and are the others seams or futures?
2. If Amp-Up serves OpAMP, does it also need the **Supervisor** deployed beside
   every collector? Ticket 01 establishes that the in-process extension cannot
   apply config, which makes the Supervisor part of the unit of adoption rather
   than an optional extra. That is a real adoption cost and it must be stated,
   not discovered.
3. Does Amp-Up ever **write without a human approving**? Auto-remediation is
   what makes a floor hold by construction, and it is also how a platform
   breaks an estate at 3am.
4. What is the **rollback** story, and does it come from OpAMP's revert-on-
   failure behaviour or from Amp-Up's own state?
5. Does the **audit trail** live in Git, in Amp-Up, or both? Ties to ticket 08.
6. Does a config Amp-Up did not author, found on a collector it manages, get
   **overwritten, flagged, or adopted**?

Question 6 is the drift question and it is the seam between orchestration and
conformance. Answer it here, because ticket 11 builds on the answer.

## Sharpened by ticket 01

Ticket 01 is resolved and it tilts the trade-off, without settling it.

- **Serving OpAMP buys the declared reading for nothing.** The Supervisor reports
  effective config and health to its own server by default, and forwards the
  collector's own effective config upward. So the own-OpAMP-server option
  delivers `Declared` over a connection the platform already owns, with no fleet
  provider, no Fleet dependency and no preview feature. Export-only cannot do
  this at all: with no channel, the platform can never answer "is this collector
  running what I asked", which removes one of the three readings in ticket 11 and
  with it the intended-versus-declared drift check. That is a large functional
  difference and it should be weighed explicitly rather than treated as a
  deployment-convenience choice.
- **Question 2 has a firm answer: yes, the Supervisor is required.** The
  extension's `toAgentCapabilities()` cannot advertise `AcceptsRemoteConfig` and
  has no config key to add it, so config delivery needs the Supervisor beside
  every collector. State it as an adoption cost in whatever this ticket outputs.
  One nuance worth not overclaiming: `accepts_restart_command` plus the
  `RemoteRestarts` gate, both off by default, let a server SIGHUP a collector
  into re-reading its own `--config`. That is not remote config delivery, but it
  is not nothing either.
- **If the platform serves OpAMP, reserve the extension name.** A config block
  named `opamp` overrides the Supervisor's injected endpoint, because remote
  config merges after it, and no code prevents it. Any generated config must
  name additional extensions `opamp/<something>`. This is a renderer rule, and it
  belongs wherever the write path is decided.

## Reframed, 4 August 2026

**This ticket's options were wrong and the recommendation it fed into has been
revoked.** The three options above omitted GitOps entirely, and export-only was
described as unable to answer "is this collector running what I asked". That is
false when a GitOps controller applies: sync status is precisely that answer.
Premise 7 on the map was chosen from that faulty menu and has been revoked.

The direction now, pending ticket 20:

- **Amp-Up does not apply configuration.** A GitOps controller does, most likely
  Argo CD. Applying may later be offered as an option, never as a requirement.
- **The graphical surface opens pull requests.** Git stays the source of truth.
- **Question 2 is moot** unless an OpAMP path returns: no Supervisor is needed
  beside anything, because nothing is pushed.
- **Question 3, writing without human approval, is answered by construction**: a
  pull request is the approval.
- **Questions 4, 5 and 6 change owner.** Rollback is `git revert`, the audit
  trail is git history, and drift is the applier's job to detect and optionally
  self-heal. Ticket 20 question 5 verifies how much of that Argo genuinely does,
  including drift introduced out of band.

What is left for this ticket is narrow: whether the lens targets one GitOps
controller or an abstraction over several, and what it does for an estate that
does not use GitOps at all. Ticket 20 supplies the evidence for both.

## Answer

Resolved 4 August 2026, under the strengthened premise 8. Two research sweeps
ran first, and one of them returns a **build** answer, which under the reuse
rule has to defend itself. It did not survive at v1 scope.

### Amp-Up renders one artefact: plain otelcol YAML at a stable repo path

**The unifying finding, reached independently by both sweeps: every delivery
target that works accepts an opaque file, and none of them want to understand
the YAML.** Splunk's `*_config_source` variables, AWS SSM `aws:downloadContent`,
Grafana's `contents = file(...)`, Nomad Pack's `config_yaml`, GCP OS Policy's
`file` resource, NixOS `configFile`, Alloy's `--config.format=otelcol`.

So the applier-agnostic property costs nothing. It follows from emitting the
one format everything already consumes. There is no abstraction over GitOps
controllers to build, and none exists to adopt.

Target ranking:

1. **Plain otelcol YAML plus whatever already applies config**, or AWS SSM
   `aws:downloadContent` with `sourceType: Git` and **commit-ID pinning**, which
   gives exactly the git-as-source-of-truth semantics the design wants. Zero
   coupling.
2. **Grafana Fleet Management** via the Terraform
   `grafana_fleet_management_pipeline` resource with `config_type = "OTEL"`.
   One-line consumption, Cloud dependency is the price.
3. **Alloy standalone, NixOS `configFile`, GCP OS Policy `file`, Nomad Pack
   `config_yaml`.** All take the artefact unchanged. Narrower audiences, and
   Nomad is BUSL 1.1 since v1.7.0, licensor now IBM.
4. **Bindplane: skip.** Right domain, wrong interface. The Terraform provider
   has **no raw-YAML resource**, so targeting it means decomposing a rendered
   pipeline back into their source/processor/destination vocabulary. Lossy
   round-trip and a standing maintenance liability.
5. **Azure DCRs and the Google Ops Agent: ruled out.** Neither can express an
   OTel pipeline at all.

**Elastic Fleet is not a delivery channel**, confirmed from
`fleet-server/docs/opamp.md`: "No remote configuration. No package management.
No server-initiated commands." Second independent source for ADR-0003's
permanence claim. Its only raw-YAML route into a policy is authoring a custom
input package with a Handlebars template.

### Helm and Kustomize rendering is dropped from v1

Premise 11 had Amp-Up rendering Helm and Kustomize itself, on the grounds that
rendering is the hard part. **Priced under the reuse rule, it does not pay.**

- **Nothing exists to reuse.** Argo CD's repo-server is gRPC-only with **no
  authentication of any kind**: no auth interceptor exists, and the entire
  access-control model is a NetworkPolicy. Building against v3.5.0 needs **37
  `k8s.io` replace directives**, pinning the whole dependency tree to Argo's
  Kubernetes version. Library adoption is roughly three importing packages.
- **The strongest evidence is other people's revealed preference.** Akuity, who
  employ Argo CD maintainers, refused this dependency in Kargo and wrote the
  reason into the source: it "can sometimes hold us back from upgrading
  important Kubernetes packages". Their predecessor `kargo-render` did import it
  and dropped it. Kargo, kluctl, holos, helmfile, gitops-promoter and
  argocd-lovely-plugin all have **no `argoproj/argo-cd` dependency**.
- **Argo has not solved offline rendering for itself.** Issue #11129 is open
  with 81 upvotes; #28942 states the trade-off directly.
- The fallback, a thin layer over the Helm Go SDK and Kustomize's `krusty` API,
  was sized at ~200 lines for rendering and extraction but **1,500 to 3,000 for
  a production service**, mostly git, auth and values plumbing.

**Decision, by the user, 4 August 2026: not now. Focus on generating the OTel
configs.** Amp-Up reads and writes plain otelcol YAML. Helm and Kustomize repos
are out of scope for v1.

**What this costs, stated plainly so it is not rediscovered later.** Helm is at
77% production use, so for any adopter whose collector config lives inside a
chart, Amp-Up cannot see the rendered result and the intended-versus-declared
cross is unavailable for them. That is a real and known limitation, not an
oversight. Revisit it when there is a user who needs it, and reconsider the
optional fidelity mode below at the same time.

**Kept on the table, deliberately not built:** talking gRPC to a user's
*existing* repo-server via `GenerateManifestWithFiles`, which client-streams a
tarball so the repo-server never touches git. It is the only way to pick up
their config management plugins and cluster capabilities. Additive, Argo-users
only, and it inherits an unauthenticated internal API. Not first.

### The earlier questions, closed

- **Q1, which of the three options is v1**: none as originally framed. The menu
  omitted GitOps and was revoked. Amp-Up exports; something else applies.
- **Q2, is the Supervisor required**: moot. Nothing is pushed.
- **Q3, does it write without a human**: no. A pull request is the approval.
- **Q4, rollback**: `git revert`.
- **Q5, audit trail**: git history.
- **Q6, drift**: the applier's job. Note ticket 20's correction, Argo treats
  collector config as an opaque string, so its check is whole-blob equality and
  not the semantic comparison this product needs.

### Amended by ticket 11, 4 August 2026

**Q1 and Q2 are reopened. The user restored OpAMP, narrowly: git stores, OpAMP
serves.**

This ticket's answer treated export-to-git as the whole write path. The user
challenged that the same day, on the grounds that distributing configuration is
what OpAMP is *for*, and that Interchange already has a working server which
pushed config across four Baseline pushes in ticket 06.

What survives here, unchanged: **the artefact.** Every delivery target that
works accepts an opaque file, so Amp-Up still renders exactly one thing, plain
otelcol YAML at a stable repo path, and Helm and Kustomize rendering stays
dropped. Git stays the source of truth, the history, the rollback and the
approval.

What does not survive: **the claim that export-only loses nothing.** Ticket 20,
resolved after this ticket, found Argo treats collector config as an opaque
string so its check is whole-blob equality, and that **51% of collector
deployments include VMs no GitOps controller can reach at all.** OpAMP is the
only channel indifferent to substrate. So an OpAMP server is added as **stateless
transport that reads git and stores nothing**, not as a store and not as the only
egress.

- **Q1** is now: export to git *and* optionally serve from it. The two are not
  rivals.
- **Q2 is no longer moot.** The extension cannot accept remote config, so **the
  Supervisor is mandatory beside every collector Amp-Up serves.** Ticket 21
  prices that, and it may still conclude against serving, in which case this
  ticket's original answer stands as written.
- **Q3 through Q6 are unchanged.** A pull request is still the approval, rollback
  is still `git revert`, the audit trail is still git history. Drift stays the
  applier's job for the foreign population, and becomes computable for the served
  one via ticket 11's normalised semantic comparison.

Ticket 11 also added the renderer rule this ticket's "Sharpened by ticket 01"
section anticipated: any additional extension must be named `opamp/<something>`,
never `opamp`, or it overrides the Supervisor's injected endpoint.

### Correction to propagate

One sweep reported the OpAMP spec as "v0.19.0, released 3 Aug 2024, with no
release since". **Wrong.** v0.19.0 published **2026-08-03**, v0.18.0 2026-05-20,
v0.17.0 2026-05-07. The spec is actively released and still `0.x`. The maturity
conclusion is unchanged; the "abandoned" framing is not true.
