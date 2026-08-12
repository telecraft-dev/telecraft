# Open questions register

Every unresolved item, with where it gets decided. Status values:
`scheduled-for-Gn` (a grill session owns it), `carried` (deliberately open,
with reason), `resolved-by-ADR-nnnn`. A question may not be silently dropped.

| ID | Question | Status | Notes |
|---|---|---|---|
| OQ-1 | Staged rollout across two delivery paths. Is a Cohort expressible as git state (directory/branch + selector)? Who computes membership deterministically? What does a rollout look like as PRs? What failure signal halts it, and does the foreign population get any of this? | scheduled-for-G4 | The largest undesigned piece. Serve-everything made it harder (two paths). Library versioning may be the same problem (`library_drift` at estate scale). |
| OQ-2 | First boot, no cache: a healthy collector running a `nop` pipeline. What does the platform show? Is "expected but never seen" a conformance outcome or a separate finding class (it has no collector to attach to)? | scheduled-for-G4/G5 | Selector-as-expectation (ADR-0007) supplies the *shape*; the finding class is undecided. |
| OQ-3 | A discovered collector matching no selector is the ungoverned quadrant. Does it appear in the estate view, and how, without reading as failure? | scheduled-for-G4 | Feeds prototype P2. |
| OQ-4 | Schema-conformance as a requirement kind: how Weaver vocabulary lands in the requirement model; four-level `requirement_level` adoption; `AttributeNames` fidelity on index-scoped backends; and the real decision — is a collection-time `live-check` tap in-product, recommended-but-external, or refused? It collides with REQ-002. | scheduled-for-G5 | Also: a schema violation's fix is an instrumentation change — routed to someone who cannot edit collector config. Remediation routing must not send work to the wrong person. |
| OQ-5 | Cardinality source per substrate: a selector says what shape should exist, not how many. Kubernetes node count answers; VM inventory maybe; bare metal possibly nothing. | scheduled-for-G5 (deferred from G1) | "Team running N servers" (REQ-012) needs a count. Shape noted in ADR-0017: expected-count is a per-substrate provider concern, not hierarchy state. |
| OQ-6 | `EstateProvider` minimum-populated-set rule, stated so it can be checked: which fields must a conforming implementation populate, covering freshness not only presence? | scheduled-for-G5 | Ticket 12's surviving question. Contract tests required. |
| OQ-7 | Multi-tenancy and the auth model. | resolved-by-ADR-0019 | Pluggable authn (OIDC/SAML/basic); ownership-derived authz; air-gap first-class; read isolation = instance per domain. |
| OQ-8 | Tenancy-to-git mapping: repo-per-team vs path-per-team for rendered artefacts. | resolved-by-ADR-0018 | One estate monorepo, path-per-team, generated code-ownership. G3 designs the layout inside it. |
| OQ-9 | Component Catalogue mechanics: `metadata.yaml` coverage/stability, versioning against collector releases, whether the Catalogue is a seam. | scheduled-for-G2 | Research task R-1 blocks this. New constraint from ADR-0019: the Catalogue must be vendorable for air-gapped instances. |
| OQ-10 | Blueprint versioning and upgrade story; `satisfies` linkage to `library_drift`; phase-collision rules. | scheduled-for-G3 | |
| OQ-11 | Scoring beyond binary: per-requirement binary aggregated to a per-service ratio — and a per-team roll-up. | resolved-by-ADR-0017 | Ratio-plus-worst per finding kind, waivers always visible, no blended number at any level. |
| OQ-12 | Evaluator cardinality and cost: attribute coverage across a large estate; correct semconv checks are per span/metric name, multiplying aggregations. | carried | Revisit when scale data exists; note in Phase 1 design. |
| OQ-13 | Project name and visual identity. | scheduled-for-G0 | Blocks publishing; must land before G3. |
| OQ-14 | UI surface inventory, navigation, canvas interaction model, tech stack. | scheduled-for-G7 (after P1–P4) | Existing console explicitly judged not good enough; fresh prototyping pass required. |
| OQ-15 | Per-collector cache semantics in the server path: is anything cached, where, for how long — cache, not record. Layer-1 digest must be confirmed loseable. | scheduled-for-G4 | Shrunken remainder of shaping ticket 19. |
