# How do people actually manage and apply OTel collector configuration?

Type: research
Status: resolved
Blocked by: none

## Question

Amp-Up's scope has narrowed to **a lens over the repository where collector
configurations live**. It does not govern the fleet, it is not a dependency, and
applying the configuration is someone else's job, possibly offered as an option.

That makes one question decisive: **what is that someone else, in practice?**
Argo CD is the assumption. Test it rather than adopt it.

1. **What is the actual distribution of application mechanisms** for OTel
   collector configuration in production? Cover Argo CD, Flux, Helm directly,
   Kustomize with `kubectl apply`, the OpenTelemetry Operator, Ansible or Chef or
   Puppet, cloud-vendor agents, and hand-managed config files. Use real evidence:
   CNCF and OpenTelemetry community surveys, the CNCF GitOps microsurvey, Argo
   and Flux adoption data, OTel Operator download or star trends, and what the
   OTel documentation itself recommends as the default install path.

2. **Is GitOps actually the majority position**, or the loud minority? Be honest
   about the gap between what conference talks describe and what surveys show.
   If Helm-directly or plain manifests dominate, say so, because a lens that
   assumes Argo would then be a lens over a minority.

3. **Argo CD versus Flux.** Relative adoption, and whether a tool integrating
   with "GitOps" can realistically target one or must target both. What is the
   common abstraction, if any?

4. **What is the repository actually shaped like?** For a fleet of collectors,
   what do people put in git: raw collector YAML in ConfigMaps, Helm values,
   `OpenTelemetryCollector` custom resources, Kustomize overlays? A lens has to
   read whatever is really there, so the input format matters more than the tool.

5. **What does the application tool already report** that a conformance lens
   would otherwise build? Argo CD's sync status and health, Flux's Kustomization
   conditions. Specifically: does Argo's sync status genuinely answer "is the
   cluster running what git says", including drift detected out of band? That
   claim is load-bearing, since it would supply the intended-versus-declared
   cross for free.

6. **What does none of them answer?** The expected finding is that all of them
   report whether config was *applied* and none report whether the telemetry it
   asked for *arrived*. Confirm or refute, because it is the product's whole
   remaining claim.

7. **Is there prior art for a read-only lens over a GitOps repo** that adds
   semantic checks? Anything in the policy space, Kyverno or OPA Gatekeeper or
   Argo CD plugins, that establishes the pattern or occupies the space.

Return the distribution with numbers where they exist, and a plain answer to
whether targeting Argo CD alone is sufficient, insufficient, or premature.

## Answer

Resolved 4 August 2026. Full findings with citations:
[research/20-findings.md](../research/20-findings.md).

**Direct answer: targeting Argo CD alone is insufficient AND premature.**
Insufficient because it reaches a minority of collector fleets and is partial
even within them. Premature because the reason being relied on, drift detection,
is the weaker of the two arguments.

1. **No measured distribution exists. Nobody has asked the question.** Neither
   OTel collector survey nor CNCF's annual survey asks how collector config is
   applied. Everything below is proxy evidence and is labelled as such in the
   findings. Best available: **Helm 77% in production, the whole Argo project
   family 43%, Flux 17%** (CNCF 2024, n=689). **OTel's own documentation sends
   adopters to the Helm chart**, explicitly deprioritises the operator, and
   mentions GitOps, Argo CD, Flux and Kustomize on **no install page at all**.
   For VMs the project's guidance is Ansible.

2. **GitOps is not the majority in the sense this product needs.** The
   circulated 77% is `some 30% + much 25% + nearly all 22%`. Only **22% say
   nearly all**, and **53% say some, just beginning, or not started**. CNCF's
   2025 survey splits it further: **58% of "innovators" against 23% of
   "adopters"**. That is the conference-versus-practice gap, stated by CNCF
   itself. Every sample is cloud-native-biased, so these are upper bounds.

3. **Argo leads Flux by 2.3 to 3x, but the gap narrowed and there is no common
   abstraction.** 2025 data: argo 52%, flux 23%, with **Flux adoption rising**
   from 17% despite its commit volume falling around 60% in the Weaveworks year.
   Treating Flux as a dead end would be wrong. Three specifics that block a
   naive abstraction: Argo CD does **not** use Kubernetes-standard conditions
   while Flux does, Flux ships two drift models with opposite defaults, and
   **Flux has no rendered-manifest API at all** where Argo exposes
   `GET /api/v1/applications/{name}/manifests`.

4. **Five repo shapes, only one analysable for free.** Helm presets are
   **one-way** and cannot remove preset configuration, lists do not merge,
   `values.schema.json` types `config` as a bare `{"type":"object"}` so schema
   tooling is blind, and the OTel project maintains **23 checked-in golden
   renders** because there is no other way to see what a values file produces.
   Worst case is a Go-templated block scalar, which **is not valid YAML at
   all**. The operator applies four mutations including wholesale rewriting of
   the Prometheus receiver.

5. **The load-bearing claim is partly true and heavily qualified.** Argo does
   detect out-of-band edits and marks `OutOfSync` without self-heal. But it is
   scoped to fields git owns, on tracked resources, minus `ignoreDifferences`,
   on a varying diff strategy, with up to a three-minute window, and **it treats
   collector config as an opaque string**. So "Argo gives you the
   intended-versus-declared cross for free" is too strong: it gives a
   whole-blob equality check, not a semantic one. One correction the researcher
   made to its own earlier draft: the Helm chart does default
   `enableConfigChecksumAnnotation: true`, so the mainstream path does restart
   pods on config change.

6. **Confirmed, not refuted: nobody checks arrival.** Nearest misses are
   Cribl's "No Data Received", which measures arrival at Cribl and is
   hand-configured, and Honeycomb Pipeline Health, which is collector
   self-reporting. Datadog's config-gap inference is the most sophisticated and
   reasons over inventory, not data.

7. **The lens pattern is established, the differentiator is open.** All static
   tools do structural predicates and none semantic, cross-repo analysis is
   universally unsupported, and **no official JSON schema for collector config
   exists** (issue #9769, open since March 2024). **Do not ship as an Argo CD
   plugin**: a Config Management Plugin cannot be read-only, and Argo's own docs
   warn that a plugin rendering blank manifests with `prune: true` deletes
   resources.

### Three things that change the plan

**The estate is not in Kubernetes.** **51% of collector deployments now include
VMs and 18% bare metal, and 50% of fleets over 100 collectors span both.** Argo
CD cannot reach any of that at any adoption level. This is the same boundary
`CONTEXT.md` describes for Customer C, showing up as an industry-wide number.

**The strongest argument for Argo is not the one being made.** Questions 3 and 4
together say the prize is that Argo's repo-server **already renders Helm and
Kustomize and will hand you the output over an API**. Rendering is the hard part
of reading a collector repo. Drift detection is the qualified part. That
reframes the integration case without rescuing an Argo-only architecture.

**A widely circulated figure is wrong.** "Argo CD runs in nearly 60% of
Kubernetes clusters" is a press-release framing that could not be reproduced
from the underlying survey. The nearest real statement is "almost 60% report
75%+ of production apps run on Kubernetes", a different claim. Flagged
do-not-use in the findings. It had been cited in an earlier draft before the
researcher caught it.

### Two pushbacks on the framing, recorded rather than resolved

- **OpAMP is a structural threat to the repository-lens premise.** Eight of nine
  commercial products in this space already make git an **input** that is pushed
  into a control plane, rather than the source of truth a lens reads. The market
  is moving toward control planes.
- **The vacancy of the read-only-lens position is not validation.** It is
  equally consistent with the market having concluded that config authority and
  observation belong together. The arrival check is genuinely unoccupied. The
  read-only constraint around it is a product choice the evidence neither
  supports nor refutes.
