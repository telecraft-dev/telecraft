# Amp-Up: scoped requirements

Everything decided across twenty-three shaping tickets, the product spec, the
built code and the blueprint prototype, gathered for a dedicated repository.

Compiled 11 August 2026. Sources are named per requirement so anything here can
be traced back and argued with.

---

## 0. The scope decision this document is waiting on

The brief for the new repository is **"the UI for managing OpenTelemetry
configurations via blueprints, cloud native, GitHub for source control,
supporting Kubernetes."**

Read literally that is **one rung of a three-rung ladder** which ticket 10 laid
out and which most of the prior work sits on:

| Rung | What it is | Adoption cost | Status under the literal brief |
|---|---|---|---|
| 1 · Conformance | Reads a telemetry backend, judges applications against a tiered library, produces verdicts | A connection string | **Dropped.** This is what the existing `ampup` repo builds today |
| 2 · Authoring | Blueprints render otelcol YAML into git, opened as pull requests | No agent change | **This is the brief** |
| 3 · Serving | An OpAMP server delivers that YAML from git | A Supervisor per collector | **Dropped**, though ticket 21 decided it in |

Sections 1 to 7 are the requirements for rung 2, which the brief unambiguously
includes. Section 8 holds rungs 1 and 3, written up so they can be adopted,
deferred or dropped as a deliberate act rather than by omission.

**The cost of dropping them, stated once so it is visible.** Rung 1 is where the
differentiator lives. Ticket 04 examined eight competing tools and found that
authoring is a crowded market where Amp-Up would be the ninth entrant, while
**no tool derives, from the configuration it manages, an expectation of what
telemetry should arrive and then checks it.** A blueprint authoring UI without
the check is a product with no claim ticket 04 did not already refute.

---

## 1. Product definition

**R1.1** Amp-Up is a **fleet and policy management platform for OpenTelemetry**
that models collection topologies graphically and generates the OTel
configurations. _(premise 1)_

**R1.2** It is **optional for Interchange and never a dependency**. Interchange's
delivery is unchanged whether Amp-Up exists or not. _(premise 2)_

**R1.3** **No component sits in the telemetry path.** If Amp-Up is down, no
telemetry stops flowing. This is the last surviving non-goal of the original
four and it now carries the entire adoption argument. _(spec §3, ticket 10)_

**R1.4** **Amp-Up ships configurations, never binaries.** No collector
distribution, no gateway, no container image, no Helm chart, no rendered
DaemonSet. How a binary reaches a host is documented, never owned. _(premise 3,
decided in ticket 21 where a Supervisor-plus-collector image was proposed and
rejected)_

**R1.5** The core is **backend-neutral**. Elastic is first-party, never
privileged. Any Elastic-specific behaviour in the core is a design error.
_(premises 5 and 12)_

---

## 2. Vocabulary (binding on code and UI)

Fixed in ticket 07, extended by 11, 13 and 21. These are not synonyms to be
varied for readability.

| Term | Meaning |
|---|---|
| **Stage** | A position in the pipeline: edge or gateway |
| **Hop** | The directed edge between two Stages, a **first-class object** |
| **Path** | One application's route through the Stage graph. An application may have several |
| **Criticality Tier** | A service's central importance classification. **"Tier" means this and nothing else** |
| **Estate** | The population of collectors |
| **Fleet** (capital F) | The **Elastic product**: Fleet Server plus the Fleet UI in Kibana. Never the estate |
| **Intended** | The config in git, pinned to a SHA |
| **Declared** | The collector's own reported effective config |
| **Observed** | Telemetry that landed in a backend over a window |

**R2.1** **Adopt upstream vocabulary rather than inventing a dialect.** Where a
concept exists upstream, use its name and semantics. `RemoteConfigStatus` is
adopted from OpAMP verbatim. Where something is genuinely absent upstream,
propose it there rather than shipping a local synonym. _(premise 13)_

**R2.2** **Names are unambiguous about vendor and product.** Seam names are
domain terms only, with no vendor word in any interface definition.
Implementations are fully qualified with the vendor's product: `ElasticFleet`
never `Fleet`, `Elasticsearch` never `Elastic`, `GrafanaFleetManagement` never
`Grafana`. A bare `Fleet` appears nowhere. This makes backend-agnosticism a lint
rather than a habit. _(premise 14)_

---

## 3. The blueprint library

The object the new repository is built around. Nothing below exists yet.

**R3.1** A **blueprint** is a named, versioned bundle of otelcol components with
a human-readable summary, the signals it produces, and the requirement ids it
claims to satisfy.

**R3.2** **Blueprints are versioned individually**, so raising the bar is a
dated, visible event rather than a silent overnight change. Mirrors
`Requirement.Version` in the built code. _(spec §4)_

**R3.3** **Composition is phase-ordered, not a set union.** Discovered by the
prototype: merging blueprints by union produces the right components in an
invalid order (`k8sattributes` after `batch`, `memory_limiter` not first), which
renders and deploys while being wrong. Each blueprint declares a phase (detect,
enrich, classify, protect, batch, export) and the renderer sorts by phase. This
is a constraint on the object model, not a rendering detail.

**R3.4** **Component collisions are resolved and surfaced.** Two blueprints may
legitimately claim the same component name (two sources both wanting `otlp`).
The renderer shares it; the UI says so.

**R3.5** A blueprint's `satisfies` list is a claim of **intent, never of fact**.
"This blueprint intends to satisfy X" is renderable from config alone; "does
satisfy X" requires observed telemetry and belongs to rung 1. The UI must not
blur the two. _(prototype finding)_

**R3.6** The library is a **directory of files, one concern per file**, so a
change to one blueprint is a one-file diff in review. Mirrors the existing
`library.Load` layout. _(built code)_

**R3.7** **Load-time validation is strict and fails closed.** A blueprint
referencing an unknown component, or a tier referencing an unknown requirement,
is a hard error at load. A silently lenient library scores everything 100%
against a floor nobody checked, which is worse than a crash. _(built code,
`library.Validate`)_

**R3.8** The **tier floor is not a choice.** Governed blueprints pushed by the
central team render whatever the owner selects; elective blueprints sit above
the floor. The surface must make the two visually distinct and the floor
non-negotiable. Tiers are **cumulative**: Tier 1 is Tier 2 plus more.
_(CONTEXT.md, `tiers.yaml`)_

---

## 4. The authoring surface

**R4.1** A **small purpose-built console**. Backstage was evaluated and rejected
as the primary surface: its catalogue is read-and-browse by design with no POST
or PUT that creates an entity, relations are output-only so a first-class Hop
has no home, and its Kubernetes write path attributes every write to the
Backstage service account, which is an audit-trail regression. _(ticket 18)_

**R4.2** Keep the **Backstage door open at zero cost** by holding the three
constraints ticket 18 identified, which are good design regardless.

**R4.3** **Generated config is visible.** A read-only escape hatch to the
rendered YAML is required for trust. _(ticket 14 Q5)_

**R4.4** The topology view must **survive scale**. Gateway-plus-many-collectors
must not become a hairball at 50 collectors or 500. Collectors are **never
drawn**; they are matched into a Stage by selector. _(tickets 07 and 14)_

**R4.5** **Gateway On-ramp emitters must be representable**: a workload that
runs no collector at all. _(ticket 14 Q4, CONTEXT.md)_

**R4.6** **Delivery path is a visible property of a collector**, because
served and git-delivered collectors have different remedies. _(ticket 21)_

**R4.7** **Two orthogonal axes, both first-class, both with the adopter's own
names and values**: Criticality and Classification. Never conflated. _(ticket
07, CONTEXT.md)_

---

## 5. Rendering and the artefact

**R5.1** Amp-Up exports **exactly one artefact: plain otelcol YAML at a stable
repo path**, plus a `supervisor.yaml` of roughly 25 lines where serving is used.
_(ticket 09, ticket 21)_

**R5.2** **No Helm or Kustomize rendering in v1.** Nothing exists to reuse
(Argo's repo-server is unauthenticated, gRPC-only, and needs 37 Kubernetes
replace directives to build against; Kargo refused the same dependency in
writing) and the fallback was priced at 1,500 to 3,000 lines. **Known cost,
accepted:** an adopter whose collector config lives inside a Helm chart loses
the intended-versus-declared comparison. _(ticket 09, premise 11)_

**R5.3** **The artefact carries its own identity.** Every rendered config is
stamped `service.telemetry.resource: {ampup.commit: <sha>}`. The collector
reports it back, so "which commit is this collector running" is **read from the
collector rather than remembered about it**. This keeps the server stateless,
works for foreign collectors delivered by anything, and survives Elastic Fleet's
redaction, which destroys `app.kubernetes.io/name`. _(premise 11, ticket 11)_

**R5.4** **The renderer must emit a node-unique attribute** via the Kubernetes
Downward API. A DaemonSet renders one manifest for all nodes, so without this
every pod reports identically. A renderer requirement, not documentation.
_(ticket 21, proven by ticket 06's duplicate-agent defect)_

**R5.5** **Applier-agnostic by construction.** The renderer reads and writes
plain YAML at a stable path and never knows what applies the result: Argo, Flux,
Helm, Ansible, SSM, a person, or Amp-Up's own server. This is free rather than
hard, because every delivery target that works accepts an opaque file and none
want to understand the YAML. _(premise 11, ticket 09)_

---

## 6. GitHub as the source of truth

**R6.1** **Git stores.** Git holds the rendered YAML, the history, the rollback
and the approval. None of it is built. _(premise 9, ticket 11)_

**R6.2** **The graphical surface opens pull requests.** It never writes to a
cluster. The audit trail is git history rather than something built. _(premise
10)_

**R6.3** **GitHub specifically** is the v1 host, per the new brief. This
narrows premise 9 and adds requirements the prior work did not carry:
authentication as a GitHub App rather than a personal token, so commits and pull
requests are attributable to the human who acted rather than to a shared service
account. Ticket 18 rejected Backstage partly for exactly this failure, so
repeating it would be self-inflicted.

**R6.4** **A hand-committed config is legitimate**, not drift. Premise 10 makes
git authoritative, so a human editing the YAML directly is a supported path and
`Intended` is whatever git says at that SHA. _(ticket 11)_

**R6.5** **Amp-Up holds no per-collector state.** A collector connects and
reports attributes, Amp-Up matches them against selectors held in git and serves
the config at that path, remembering nothing. _(premise 11, ticket 21)_

---

## 7. Kubernetes support

**R7.1** **Kubernetes is the control plane's substrate, not the managed
population.** Governed collectors do not have to live in Kubernetes; only the
governance does. _(premise 6, ticket 07)_

**R7.2** **Custom-resource-per-collector is disqualified** by Kubernetes' own
CRD documentation at estate scale. Not because of etcd size: external objects
cannot be watched so must be polled, and the reconcile arithmetic fails by an
order of magnitude. The documented alternative is an aggregated API server with
its own storage. Authored objects are few and may be CRDs; per-collector state
may not. _(ticket 17)_

**R7.3** **Mixing substrates is the default mode, not an edge case.** 51% of
collector deployments include VMs, 18% bare metal, and 50% of fleets over 100
collectors span both Kubernetes and VMs. A Kubernetes-only product does not
address the measured market. _(ticket 20, ticket 21)_

**R7.4** **GitOps is not assumed.** Only 22% of adopters say nearly all their
deployment is GitOps and 53% say some or less; Helm is at 77% in production
against the Argo family at 43% and Flux at 17%. _(ticket 20)_

**R7.5** **Upstream ships nothing for running the Supervisor in Kubernetes.**
Zero references in the Operator or the Helm charts, requested since 2022-12-12.
The upstream pattern is entry-point-with-child in one container, never a
sidecar. Amp-Up documents this; it does not solve it with an image. _(ticket 23,
constrained by R1.4)_

---

## 8. Held for a scope decision

Decided work that the literal brief excludes. Each is written so it can be
adopted or dropped deliberately.

### 8.1 Conformance (rung 1): built, and where the differentiator lives

Already working code in the existing `ampup` repo: tiers with inheritance,
requirements asserting on config and signal, load-time validation, mandatory
remediation text, exemptions with owner and expiry, grace periods shrinking as
criticality rises, seven outcomes with a severity ordering, a CLI check mode
that exits non-zero for CI.

- **Two readings, crossed.** Declared present plus observed absent is a broken
  pipeline; declared absent plus observed absent is an unmet requirement;
  declared absent plus observed present is ungoverned telemetry. Three findings,
  three owners, and only the cross produces them. _(spec §2)_
- **Adopt semantic conventions YAML and Weaver**, including its finding taxonomy
  and four-level `requirement_level`. OTel schema URLs are a version-migration
  mechanism and cannot express a constraint. _(ticket 05)_
- **The query-language constraint is permanent.** Requirements express signal
  presence, volume, attribute coverage and cardinality, never a backend's query
  language. If a requirement can embed ES|QL, the abstraction is dead. Type,
  unit and instrument conformance are permanently out, because most backends
  destroy the declared type at ingest. _(premise 5, ticket 05)_
- **Drop drift detection from any pitch.** Alloy and Splunk both ship it and
  Splunk markets it by name. Table stakes. _(ticket 04)_

### 8.2 Serving (rung 3): decided in, and the one carried risk

- **Amp-Up serves everything, with GitOps as an alternative rather than a
  fallback.** One artefact, two ways to move it, chosen per collector by the
  adopter. _(ticket 21)_
- **The OpAMP server is stateless transport that reads git** and stores nothing.
  Removing it loses delivery but never loses the record. _(premise 9)_
- **Price: the Supervisor is mandatory beside every served collector**, because
  the in-process `opamp` extension cannot accept remote config. This changes the
  unit of adoption. _(ticket 01, ticket 21)_
- **"Not a dependency of telemetry flow" holds by construction.** The Supervisor
  has no TTL, no retry ceiling and no code path from connection state to
  collector state, so a served collector rides out an indefinite outage on its
  last-good config. **One tooth:** first boot with no cache yields a *healthy*
  collector running a `nop` pipeline, which is silent nothing rather than a
  visible failure. _(ticket 23)_
- **The carried risk.** `opampsupervisor` has been alpha since creation, its
  alpha tracking issue closed 2025-04-04 as "not necessarily something we intend
  for production usage", with no successor and no graduation criteria. Building
  is defensible under premise 8 (no OpAMP server anywhere reads from git or has
  a pluggable config source; the build baseline is 99 lines over `opamp-go`'s
  755-line server package) but it sits on an alpha dependency with no
  long-running-stability evidence in either direction. _(ticket 23)_

### 8.3 The three plugin seams

| Seam | Answers | First-party | Later |
|---|---|---|---|
| `RegistryProvider` | What applications exist, and what tier is each? | YAML file | ServiceNow, Backstage, CMDB |
| `EstateProvider` | What collectors exist, their health and effective config? | `ElasticFleet`, plus OpAMP direct | Bindplane, static |
| `TelemetryProvider` | Did signal X arrive for application Y in window W? | `Elasticsearch` | Prometheus, ClickHouse, Loki |

Needed in full only if rung 1 is in scope. `EstateProvider` needs **two
implementations from day one** even so, because serving everything does not
remove the foreign population. _(ticket 11, ticket 12)_

---

## 9. Non-goals

**N1** Not a collector distribution, not an agent, nothing in the telemetry
path. _(R1.3, R1.4)_

**N2** Not a change to Interchange delivery. Interchange keeps managing
collectors the way it does now, permanently. _(map, out of scope)_

**N3** **Enforcement through Elastic Fleet is permanently unavailable**, not
deferred. Fleet redacts on the field name `key`, freezes agent identity at
enrol time, and has no public GA commitment for OTel collector support. Fleet is
a console, not a source. _(tickets 02, 03, 13)_

**N4** **Profiles are out.** The signal is Alpha, backend support is uneven, and
a requirement written against it could not be evaluated honestly. Logs, metrics
and traces only. _(CONTEXT.md, built code)_

---

## 10. The binding engineering rule

**R10.1 Reuse over build.** Strengthened from a preference into a test: the
default answer to "how do we do X" is "what already does X", and **a decision to
build must defend itself in writing** by enumerating what the OTel, Kubernetes,
GitOps and Elastic ecosystems already ship for that job, pricing adoption
against building, and recording the alternatives even where building wins.
Ecosystem maturity counts: prefer CNCF Graduated or stable upstream components
over alpha ones. _(premise 8)_

Three reuse candidates for the blueprint library specifically, none of which has
been priced yet and all of which should be before any format is invented:

1. **OTel declarative configuration.** JSON Schema, 1.0.0, Stable since February
   2026. A stable machine-readable schema for a whole collector config gives
   validation and form generation without inventing either. Named in ticket 04
   as a competitor's modelling unit, never yet as Amp-Up's own substrate.
2. **Component `metadata.yaml` in collector-contrib.** Every receiver, processor
   and exporter ships one declaring stability and supported signals. A
   machine-generated catalogue of what exists, maintained upstream. Hand-curating
   that list is the maintenance burden that kills config libraries.
3. **OTTL** for the transformation half, and **Weaver plus semconv YAML** for the
   attribute half if rung 1 is in scope.

---

## 11. Still open, and not blocked on this document

- **Multi-tenancy.** Does a workload owner see only their applications? Blocked
  on whether Amp-Up is deployed centrally or per workload owner, which nobody has
  decided. Changes the auth model substantially, so it wants deciding before the
  UI rather than after.
- **The auth model**: who edits the library, who assigns a tier, who only looks.
  Follows multi-tenancy, and now also follows R6.3.
- **Scoring beyond binary.** Probably binary per requirement aggregated to a
  ratio per application, never a single estate-wide number.
- **Staged rollout**, which is harder than when ticket 22 was written because it
  must now work across two delivery paths.
- **Where cardinality comes from** per substrate, and whether "expected but never
  seen" is a conformance outcome or a separate class of finding. _(ticket 08)_
- **Evaluator cardinality and cost.** Presence checks per application per window
  are cheap; attribute coverage across a large estate may not be.
- **The project name.** "Amp-Up" promises OpAMP, which the product does not lead
  with, and it collides with Sourcegraph's Amp. A rename is cheap until the repo
  is published, which is now.

---

## 12. Evidence trail

Resolved tickets behind the above, in
`.scratch/ampup-product-shape/issues/`, with research findings alongside in
`research/`:

01 extension and Supervisor coexistence · 02 Fleet's OTel API surface ·
03 Fleet stability signal · 04 graphical tooling landscape · 05 conformance in
OTel terms · 06 live coexistence spike · 07 what Amp-Up models (root) ·
09 the write path · 11 the three readings · 13 ADR-0003 verdict · 17 Kubernetes
ecosystem reuse · 18 Backstage as surface · 20 how config is actually applied ·
21 the Supervisor's price · 23 the Supervisor cost enumeration.

Open: 08, 10, 12, 14, 15, 16, 19, 22.

Prototype: branch `proto/blueprint-library` in the existing `ampup` repo, three
variants of the blueprint surface, `make proto`.
