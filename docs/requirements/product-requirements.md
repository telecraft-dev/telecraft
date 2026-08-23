# Product requirements

**Telecraft**: an open-source fleet and policy management platform for
OpenTelemetry (named 2026-08-12, `docs/branding/naming.md`). Carried from the
prior shaping effort (`docs/research/2026-08-11-compiled-requirements-original.md`)
under neutral terminology, extended with four new requirement areas, and
renumbered. Vocabulary per ADR-0015 (Tier = topology position; Service Class;
Sensitivity; Intended/Effective/Observed; Service). Each requirement cites its
ADR or the grill session that will produce one; the traceability matrix lives
in `traceability.md`.

## 1. Product definition

- **REQ-001** The platform models collection topologies graphically, generates
  otelcol configurations, judges the estate's conformance, and optionally
  delivers config, in three separately-adoptable rungs: Conformance, Authoring,
  Serving. Adopting a higher rung is never required to use a lower one.
- **REQ-002** No component sits in the telemetry path. If the platform is
  down, no telemetry stops flowing. (ADR-0002)
- **REQ-003** The platform ships configurations, never binaries. (ADR-0002)
- **REQ-004** The core is vendor-neutral; Elastic is the first plugin, never
  privileged. Vendor words in core interfaces are lint errors. (ADR-0001)
- **REQ-005** The project is uniquely branded: a name that survives collision
  checks and a visual identity, decided before anything is published. (G0)
- **REQ-006** Deployable air-gapped: no hard dependency on any SaaS or on a
  specific git host. GitHub is the first-party forge integration, never an
  assumption. (ADR-0019)

## 2. Governance

- **REQ-010** A machine-generated component Catalogue, sourced from
  collector-contrib `metadata.yaml`, inventories every receiver, processor,
  exporter, connector and extension type with stability and supported
  signals. Hand-curation of the component list is prohibited: it is the
  maintenance burden that kills config libraries. (G2)
- **REQ-011** Allow-lists select from the Catalogue per Team: only permitted
  component types are offered in the composer palette and accepted by the
  renderer. Scoping, inheritance down the team tree, and default posture are
  G2 decisions. (G2)
- **REQ-012** Owners and Teams are hierarchical: an Owner is the lowest unit
  of management and belongs to a Team; Teams nest. Compliance rolls up the
  tree: "a team running N servers at class C1 must run these modules" is
  expressible and reportable. (ADR-0017, ADR-0035)
- **REQ-013** Service Class and Sensitivity are two orthogonal first-class
  axes with the adopter's own names and values. Class floors are cumulative
  and non-negotiable; elective additions sit above the floor and the surface
  makes the two visually distinct. A Service Class is never rendered as
  "Tier N". (ADR-0007, ADR-0015, G3)
- **REQ-014** Exemptions carry a mandatory owner and expiry; grace periods
  shrink as Service Class rises; waivers never replace the diagnosis.
  (ADR-0004, ADR-0037)
- **REQ-015** Every authored object carries an owner (Component, Blueprint,
  Tier, Hop, Path, Service, Requirement, Exemption). Findings route to the
  owner of the object the finding is about. Collectors inherit ownership
  from their Tier; exceptions are expressed by splitting the Tier.
  (ADR-0016)
- **REQ-016** Components are first-class: configured instances of catalogue
  types, named, versioned, ownable, and inherited by reference, never by
  copy. A change by the owning team re-renders every consumer. (ADR-0016)
- **REQ-017** Authentication is pluggable: OIDC, SAML and basic auth
  first-party; forge OAuth as a convenience. Authorization is derived from
  ownership with one source of truth: generated forge code-ownership where
  supported, a platform merge gate otherwise. Team roll-up is
  ratio-plus-worst per finding kind, waivers always visible, never a single
  blended number. (ADR-0017, ADR-0019)

## 3. Conformance (rung 1)

- **REQ-020** Services are read twice, Effective and Observed, and the
  cross produces the seven outcomes with a severity ordering; delivery
  status (Intended × Effective) sits beside the verdict, per collector.
  (ADR-0004)
- **REQ-021** The requirements library is a directory of files, one concern
  per file, strictly validated at load, failing closed. (prior built code)
- **REQ-022** Conformance vocabulary adopts semconv/Weaver: four-level
  `requirement_level`, Weaver's finding taxonomy and severity split, custom
  registries for adopter attributes, `registry infer` for onboarding.
  (ADR-0009, ADR-0034)
- **REQ-023** Requirements never embed a backend query language. The
  `AttributeNames` primitive is the sanctioned extension. (ADR-0009, ADR-0034)
- **REQ-024** A CI check mode evaluates once and exits non-zero on counting
  failures. (prior built code)
- **REQ-025** `library_drift` is detected: config in git that no longer
  satisfies a raised Service Class floor is a finding owned by the repo.
  (ADR-0004)

## 4. Authoring (rung 2)

- **REQ-030** Blueprints are named, versioned compositions of Components
  with phase-ordered assembly (detect → enrich → classify → protect → batch
  → export); the renderer sorts by phase, because union-merge is known to produce
  invalid orderings. Component collisions are resolved and surfaced. (G3)
- **REQ-031** A blueprint's `satisfies` list is intent, never fact; the UI
  must not blur the two. (ADR-0004, G3)
- **REQ-032** The renderer exports exactly one artefact: plain otelcol YAML
  at a stable repo path (+ `supervisor.yaml` where served), stamped with the
  commit SHA, applier-agnostic. (ADR-0002, ADR-0013)
- **REQ-033** The surface opens pull requests via the forge integration
  (GitHub App first-party); it never writes to a cluster; hand-committed
  config is legitimate; commits are attributable to the authenticated human
  even without a forge account. (ADR-0003, ADR-0014, ADR-0019)
- **REQ-034** Renderer hard rules: `opamp/<x>` extension naming, node-unique
  attribute via Downward API, untrusted-Hop attribute stripping generated
  automatically. (ADR-0007, ADR-0010)
- **REQ-035** Generated YAML is visible read-only in the UI: the escape
  hatch is required for trust. (G7)
- **REQ-036** The topology view survives scale: collectors are never drawn;
  gateway on-ramp emitters (no collector at all) are representable as a Path
  straight to a gateway Tier. (ADR-0007)

## 5. Serving (rung 3)

- **REQ-040** A stateless OpAMP server reads git, matches reported
  attributes against selectors, serves the config at that path, and stores
  nothing. (ADR-0013)
- **REQ-041** GitOps is a co-equal delivery path chosen per collector;
  delivery path is a visible collector property. (ADR-0010)
- **REQ-042** The server never serves an empty config map; first-boot
  behaviour must not read as silent success. (ADR-0010, ADR-0030)
- **REQ-043** Staged rollout works across both delivery paths. (ADR-0029)
- **REQ-044** `EstateProvider` ships two implementations day one (OpAMP
  direct, ElasticFleet); a provider that cannot report a reading never looks
  like a failure. (ADR-0008, ADR-0036)

## 6. Pipeline observability

- **REQ-050** The platform visualises telemetry flow over the authored
  topology: per-Tier/per-Hop throughput, volume, and freshness, joined from
  collector self-telemetry back to the config that produced it. Join keys
  per R-4: legacy datapoint attributes primary for metrics,
  `otelcol.component.*` scope attributes for logs/traces.
  (ADR-0039, ADR-0040, ADR-0041)
- **REQ-051** The differentiator: from the Intended config at a SHA, derive
  an Expectation of what telemetry should arrive, and check it. Green means
  "the config worked", never merely "the config applied". (ADR-0033, ADR-0034, ADR-0038)
- **REQ-052** "Expected but never seen" is surfaced even though there is no
  collector to attach it to; ungoverned (observed, never authored) is
  surfaced without reading as failure. (ADR-0030, ADR-0031, ADR-0035)
- **REQ-053** Self-telemetry rides the existing `TelemetryProvider` seam,
  no privileged side channel. (ADR-0039)

## 7. Non-goals

- **NG-1** Not a collector distribution, not an agent, nothing in the
  telemetry path. (ADR-0002)
- **NG-2** No enforcement through Elastic Fleet: permanently unavailable,
  not deferred. Elastic Fleet is a console, not a source. (ADR-0008)
- **NG-3** Profiles are out; logs, metrics, traces only. (ADR-0009)
- **NG-4** No Helm/Kustomize rendering in v1, known cost accepted.
  (ADR-0011)
- **NG-5** Drift detection is table stakes, never the pitch; the refuted
  claims ("nobody alerts on missing data", "nobody checks a stream against a
  declaration") are not reinstated. (ADR-0005, research 04)

## 8. The binding engineering rule

- **REQ-060** Reuse over build: a decision to build must defend itself in
  writing by enumerating what the OTel, Kubernetes, GitOps and vendor
  ecosystems already ship, pricing adoption against building. Prefer stable
  upstream components; alpha dependencies are accepted only explicitly
  (ADR-0010 is the one such acceptance).
