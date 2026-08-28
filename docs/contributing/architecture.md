---
title: Architecture
description: The package map, the neutral core boundary the vendor-word lint enforces, the CLI binaries, and how a verdict flows from authored files to output.
order: 5
---

# Architecture

The repository holds a Go core, a set of vendor implementations behind its
seams, a TypeScript console, and one lint that keeps the first two apart.

| Path | What lives there |
|---|---|
| `cmd/` | The four binaries. |
| `internal/` | The neutral core. No vendor word appears here. |
| `internal/provider/` | Vendor implementations behind the core's seams, product-qualified. |
| `console/` | The console, which consumes only the documented platform API. |
| `tools/vendorlint/` | The lint that draws the core and provider boundary. |
| `docs/` | The decision corpus and this documentation. |

## The neutral core boundary

The core is vendor-neutral, and ADR-0001 makes that mechanical rather than
cultural. `vendorlint.yaml` at the repository root declares four scopes, and
its glob patterns **are** the boundary. Changing them changes the
architecture, so change them only with ADR-0001 in hand.

**`core`** covers `cmd/**`, `internal/**` and `pkg/**`, excluding
`internal/provider/**`. No telemetry vendor's name appears there at all, in
identifiers, strings, or comments, and no forge's name appears there either.
Matching is case-insensitive and by substring, which is deliberate: it catches
compounds like the historical `FleetProvider` that gave the rule its reason to
exist. Two narrow exceptions are allowlisted, because they are module paths
and generated output rather than vendor behaviour: `github.com/` as the Go
module namespace, and the phrase naming the dialect a generated
code-ownership projection is written in.

**`provider`** covers `internal/provider/**`. Vendor words are expected here,
but only fully qualified with the vendor's *product*, never the company:
`Elasticsearch` and `ElasticFleet`, never `Elastic`; `GrafanaFleetManagement`,
never `Grafana`. A bare `Fleet` appears nowhere.

**`console`** covers `console/**`, excluding build output. The console is
neutral core too, so the same vendor-word rule applies to its sources,
fixtures and tooling.

**`docs`** covers `README.md` and `docs/**`, excluding the research,
prototype and branding trees, which record external reality verbatim. In the
docs the rule narrows to one word: a bare `Fleet`, with product-qualified
forms and mention-not-use constructions allowed. That is why this section
writes Elastic Fleet in full.

What the boundary means in practice: an interface is named in domain terms
only (`EstateProvider`, `TelemetryProvider`, `InventoryProvider`), and the
package that defines it holds no knowledge of who implements it. Each
provider tree exposes a neutral `New` constructor taking neutral connection
settings, and that constructor is the only door a binary goes through. Which
backend answers is wiring inside `internal/provider/`, never knowledge in the
caller.

## The core packages

### Seams

Each seam package defines one interface and its vocabulary, and nothing else.

| Package | Owns |
|---|---|
| `internal/telemetry` | The TelemetryProvider seam: did signal X arrive for Service Y in window W. No query language, index name or product concept crosses it. `AttributeNames` is the sanctioned extension primitive; `ObserveSelf` reads collector self-telemetry; `Meter` reads pipeline-grain flow counters. |
| `internal/estate` | The EstateProvider seam: the collector estate in one call, keyed on the collector. Also the static capability declaration, the minimum populated set, and the staleness arithmetic that demotes an old reading before it can feed a verdict. |
| `internal/inventory` | The InventoryProvider seam: given a Tier's selector, how many instances should match. Also the population findings the answer produces. |
| `internal/forge` | The forge-adapter seam: a change is a branch, a message, an acting human and a set of file contents; a proposal is an opaque identifier and a URL. Implementations declare a static capability ladder. |
| `internal/auth` | The authentication seam and the ownership-derived authorisation it feeds. Two flow shapes, password and redirect, cover every first-party provider. The redirect flow hands its provider two values per attempt, the CSRF state and a secret the browser never sees, and both ride the caller's signed cookie rather than a server-side store (ADR-0019, ADR-0013). |

### The authored model

These packages load and validate what humans write in the estate repository.
All of them load strictly and fail closed: an unknown field, a malformed
document, or a missing mandatory field is a load error naming the file, never
a silently lenient verdict.

| Package | Owns |
|---|---|
| `internal/requirements` | The requirements library: the versioned assertions a Service is judged against, and the signal vocabulary the rest of the core adopts. |
| `internal/blueprint` | Blueprints and Components: per-signal lanes of ordered Component references, shared or local, with the ordering rules and the authoring findings. |
| `internal/catalogue` | The Catalogue: the versioned inventory of collector component types, machine-generated from upstream metadata. Hand-curation of the component list is prohibited. |
| `internal/ownership` | The Team tree and the owner every authored object carries. Routes each finding to the owner of the object it is about, and rolls compliance up as ratio-plus-worst per finding kind. Owns the one grade vocabulary every producer of findings grades in: pass, neutral, advisory, violation. |
| `internal/allowlist` | The Allow-list policy: which subset of the Catalogue each Team may use, composed down the tree by narrowing-only inheritance with owned Grants as the one widening mechanism. |

### The import pipeline

Two substrates are imported rather than authored: content somebody else
maintains as ordinary git files, which the platform imports at a pinned ref
into an atomic, versioned artefact and keeps beside the versions already
installed.

| Package | Owns |
|---|---|
| `internal/substrate` | The one import pipeline: the provenance record every artefact carries, the sparse fetch parameterised by the files a substrate needs, the deterministic ref-named atomic write, and the strict load. A substrate supplies four facts (its name, its files, its artefact name, and how to build one from a materialised tree) and gets the pipeline. There is one so that a second one cannot drift from it. |
| `internal/schemaregistry` | The Schema Registry: the versioned declaration of what the estate's telemetry is supposed to look like, imported from a custom Weaver registry held as ordinary git content. Groups, their attributes, enum members, requirement levels and deprecation notices, recorded as declared. The import reads content out of git and runs no registry toolchain, which an architecture test holds the whole repository to. |

| `internal/activation` | The designation half of the pipeline: which imported version of each substrate the estate is judged against, recorded in `activations.yaml` rather than inferred from a file name or a flag, and refusing to call a version active without the impact report the activation was decided on. It also owns the two reports, and the rule that a collector is judged against the Catalogue for the version it runs rather than the active one. |

`internal/catalogue`, in the table above, is the other substrate on this
pipeline.

### Rendering and delivery

| Package | Owns |
|---|---|
| `internal/renderer` | Compiles bound Blueprints to rendered artefacts, one plain collector YAML per Tier at `rendered/<team>/<tier>.yaml`, plus a supervisor config where the Tier is served. Deterministic: identical inputs produce byte-identical artefacts, which is what lets CI recompute `rendered/` and fail on mismatch. |
| `internal/serving` | The stateless OpAMP serving path: a collector reports identifying attributes, the server matches them against the selectors held in git and serves the artefact at that path, remembering nothing. Holds a closed list of three rebuildable things and no durable state. |
| `internal/readings` | One reading of the two live seams, composed into the readings document the console documents are computed over. It decides nothing: every field is something a seam returned, plus the clocks that let a judgement date a gap. |
| `internal/rollout` | Staged rollouts: cohort membership as a pure function, the advisory reading of the foreign population, the halt and advance evaluation, and the platform-proposed advance and abort changes. Nothing is persisted. |
| `internal/normalise` | The three-layer drift normaliser. Layer 1 digests raw bytes, layer 2 digests the normalised form under one delivery path's Mutation profile, and layer 3 computes the structural diff only when layer 2 disagrees. |
| `internal/delivery` | Intended against Effective, judged per collector, producing the delivery status that sits beside the conformance verdict and never blends into it. |

### Judgement

| Package | Owns |
|---|---|
| `internal/conformance` | The verdict cross, Effective against Observed, judged per requirement for one Service in one Environment, producing the eight outcomes with their severity ordering. Schema conformance is judged here too, against the Schema Registry rather than the cross, and maps the registry's four requirement levels onto those same eight outcomes and the one grade vocabulary, adding neither. Nothing here blends across environments. |
| `internal/expectation` | The Expectation engine: derives checkable Claims from the Intended config at a commit SHA, literal-only, and judges arrivals against them. This is what makes green mean "the config worked" rather than "the config applied". |
| `internal/drift` | `library_drift`: config in git that passes the version it claims while failing the current one, in its Requirement and Component facets. |
| `internal/metering` | The derived flow readings on cards and the canvas, computed on read from readings taken through the TelemetryProvider seam. Two grains, pipeline and Service, never blended. Nothing is stored. |
| `internal/selftelemetry` | The one normalisation layer for collector self-telemetry join keys, mapping the attributes a reading carries back to the component in the rendered YAML. |

### Output

| Package | Owns |
|---|---|
| `internal/card` | The card data contract: the face payload, cheap and bulk-fetchable for a whole shelf, and the drawer payload, fetched per card on demand. Integer-versioned, and the only thing a card surface may consume. |
| `internal/console` | Assembles the console API snapshot: the JSON documents the platform API serves, computed by the real evaluators over a real estate checkout. Nothing here fabricates a verdict. |
| `internal/instance` | The Instance server: one process serving the console, the platform API behind the authentication gate, the two probes, and the OpAMP endpoint, over one estate. Holds the head, one loseable memo of the documents, and the authentication wiring at that head. |
| `internal/consoleassets` | The built console, embedded in the binary. A build with nothing staged answers the console route with a page saying so, so the tree compiles where npm has never run. |
| `internal/secrets` | Resolving the material an estate names against the directory the deployment filled. A name is letters, digits and hyphens, so it can never describe a path, and a value is read at the point of use rather than held. |
| `internal/register` | The register of Organisations: one authored record per Organisation, holding its name, the address its Instance answers on, where its estate is read from, and its lifecycle state. The schema has no field that takes secret material, and a remote carrying a password is a load error. |
| `internal/provisioner` | Reconciling the register against the Instances a deployment runs: what to create, what to bring in line, and what to retire, behind a substrate seam that carries names and addresses in both directions. It holds nothing of any Organisation's estate, which a test over every type that crosses it holds it to. |

### Provider implementations

`internal/provider/` holds everything vendor-shaped: index patterns, field
layouts, query bodies, API paths, authentication schemes.

| Package | Implementations |
|---|---|
| `internal/provider/telemetry` | `Elasticsearch`. |
| `internal/provider/estate` | `OpAMPDirect`, reading collectors served by the platform's own server, and `ElasticFleet`, reading the foreign population through Elastic Fleet's agent APIs. Also the versioned Elastic Fleet Mutation profile and its redaction handling. |
| `internal/provider/inventory` | `Kubernetes`, answering live from the substrate's own API so the expectation floats with the autoscaler. |
| `internal/provider/forge` | `GitHubApp`, plus the neutral `New` that dispatches on the repository's host. |

See [Providers](providers.md) for how to add one.

## The CLI binaries

| Binary | What it does |
|---|---|
| `cmd/telecraft` | The platform CLI, with eight subcommands: `observe` prints Observed readings for one Service; `check` is the CI mode, evaluating the estate once and writing one machine-readable report; `palette` prints one team's effective palette with provenance; `render` compiles every Tier's bound Blueprint to the rendered tree; `serve` runs the stateless OpAMP server; `snapshot` writes the console API snapshot; `delivery` prints one collector's delivery status; and `passwd` hashes one basic-auth secret for the `users.yaml` seam. |
| `cmd/blueprint-check` | Strict-loads every Blueprint and shared Component in an estate's source roots and prints the findings. Mechanical invalidity refuses the load and exits 1; findings print and exit 0, because they advise owners and never block. |
| `cmd/catalogue-import` | Runs the Catalogue half of the import pipeline: fetches the upstream collector-contrib tree at a pinned release tag, walks every `metadata.yaml`, and writes one atomic versioned Catalogue artefact plus a coverage report. Re-running the same tag is idempotent, and existing versions are retained rather than replaced. |
| `cmd/register-check` | Strict-loads the register of Organisations a deployment serving several of them reads, and prints what it holds. It exits 1 when the register does not load, with every problem in every file at once, because the register is reviewed as one change. |
| `cmd/schema-registry-import` | Runs the Schema Registry half of the same pipeline: fetches an adopter's registry repository at a pinned ref, reads every model file out of it, and writes one atomic versioned Schema Registry artefact plus a coverage report of what entered it, what was left out, and which references come from a dependency registry. Idempotent and version-retaining in the same way. |

Run any of them with `go run ./cmd/<name>`. `cmd/telecraft` with no arguments
prints its usage.

## How a verdict flows

The path from a file a human wrote to a state a card shows, and which package
owns each step:

1. **Authored files.** The estate repository holds `teams.yaml`, the
   per-team trees under `teams/<team>/` (components, blueprints, tiers,
   services), the requirements library, `allow-lists.yaml`, the exemptions
   tree, and the retained Catalogue versions.
2. **Strict load.** `internal/ownership`, `internal/requirements`,
   `internal/blueprint`, `internal/catalogue` and `internal/allowlist` load
   those files. A load error names the file and the field, and judges nothing.
3. **Render.** `internal/renderer` compiles each Tier's bound Blueprint into
   the rendered artefact tree, stamping every artefact with the commit SHA so
   it carries its own identity. This is the **Intended** reading.
4. **Deliver.** `internal/serving` matches a connecting collector's reported
   attributes against Tier selectors and serves the artefact at that path.
   GitOps delivery is co-equal, and the same computation runs for both paths.
5. **Read.** Three seams answer what is actually true. `internal/estate`
   returns the **Effective** config each collector reports, its health tree
   and its delivery status. `internal/telemetry` returns the **Observed**
   arrivals. `internal/inventory` returns the population a Tier's selector
   should match.
6. **Judge.** `internal/delivery` crosses Intended against Effective per
   collector, using `internal/normalise` to decide whether a difference is
   drift, lag, or nothing. `internal/expectation` derives Claims from the
   Intended config and judges the arrivals against them.
   `internal/conformance` crosses Effective against Observed per requirement
   for one Service in one Environment. `internal/drift` finds `library_drift`,
   and `internal/inventory` finds population shortfalls.
7. **Route and roll up.** `internal/ownership` routes each finding to the
   owner of the object it is about, and rolls the results up the team tree as
   ratio-plus-worst per finding kind, with waived findings always visible.
8. **Present.** `internal/card` assembles the face and drawer payloads.
   `cmd/telecraft check` writes a JSON report and exits non-zero exactly when
   counting failures exist. `internal/console` assembles the same documents as
   a static snapshot. The console reads whichever of these it is pointed at,
   and nothing that draws a card consumes anything but the card contract.

Two properties hold across the whole flow. Not knowing is a normal state: a
provider that cannot answer reports the reading as unknown with a cause,
returns no error, and never fabricates a value. And every reading carries the
instant it was taken, so a stale answer cannot masquerade as a fresh one.
