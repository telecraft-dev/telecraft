# Intended, declared, observed: settle the reading model

Type: grilling
Status: resolved
Blocked by: 02, 09

## Question

The core of the destination.

Amp-Up today reads each application **twice**. `Declared` is what a collector
reports it is configured to do. `Observed` is what telemetry landed. Crossed,
they give four findings: `compliant`, `broken_pipeline`, `ungoverned`,
`not_configured`, degrading to `not_delivered` when only one reading exists.

Once Amp-Up generates config, a **third** reading exists: what Amp-Up
**intended**, meaning what it rendered from the tier and policy model. Neither
codebase has all three. Amp-Up has no `Intended` because it never wrote config.
Interchange has no `Declared` or `Observed` because it never read anything back.

Two crosses, two different owners:

| Cross | Question | Whose problem |
|---|---|---|
| Intended vs Declared | Did the config I pushed actually land? | the platform, or the collector |
| Declared vs Observed | Did the config that landed actually work? | the application, or the pipeline |

Questions to settle:

1. Is `Intended` a **stored artefact** or **re-rendered on demand**? Stored gives
   a real audit trail and survives a library change; re-rendered cannot drift
   from the model but cannot tell you what was pushed last Tuesday either.
2. Is a reading held as **bytes**, as a **semantic structure**, or both?
   Byte comparison is the only honest drift check. Requirement assertions need
   structure. Probably both, and the cost should be named.
3. **The pipeline-structure defect.** `model.Declared` at
   `internal/model/model.go:142` carries `Receivers`, `Processors`, `Exporters`
   as three flat `[]string` with no pipeline wiring. So `has_receiver:
   [filelog, otlp]` proves a `filelog` receiver exists somewhere, not that it
   feeds the **logs** pipeline. A collector with `filelog` wired only into
   traces passes the assertion and delivers no logs, which is exactly the
   `broken_pipeline` case the design exists to catch. Fix the type, or decide
   deliberately to live with it and record why.
4. What is the **finding taxonomy** across three readings? The current four
   cells become more. Not every cell needs a name, but the ones that get one
   should have a distinct owner and a distinct remediation, which is the test
   the existing four pass.
5. What does the model do for a collector Amp-Up does **not** manage, where
   `Intended` is absent by definition? Absent must stay distinguishable from
   empty. `Declared.Known` already makes that distinction and the comment at
   `model.go:138` explains why it matters; extend the same discipline.
6. Do the **seams change**? `FleetProvider` at `provider.go:51` is named for a
   world where the fleet was someone else's. If Amp-Up serves OpAMP it supplies
   its own declared reading, and the seam narrows to foreign collectors.

## Sharpened by tickets 01 and 02

Both resolved. They bound question 3 and largely answer question 6.

- **Question 3's fix is affordable, so the flat `[]string` should go.** Ticket 02
  established that Fleet returns `service.pipelines.<name>.{receivers,processors,
  exporters}` verbatim, with **processor order preserved**, and that the list
  endpoint's `pipeline_config` fingerprint returns full wiring for every
  collector in one call. The pipeline-structure defect was worth living with only
  if no source could populate the fix. One can. `Declared` should carry pipelines.
- **Processor order is semantically significant** and is available. Decide
  whether the type preserves it. A redaction processor before an exporter and
  after it are different systems, and an unordered set cannot express that.
- **Declared state is available without any fleet provider at all**, if the
  platform serves OpAMP. Ticket 01 established the Supervisor reports effective
  config and health by default and forwards the collector's own effective config
  upward. So question 6's answer depends entirely on ticket 09's write path: with
  an own-OpAMP-server choice, the fleet seam narrows to foreign collectors, and
  `FleetProvider` is misnamed for what it would then be.
- **Two known losses to encode honestly.** Fleet drops **named** OpAMP config
  entries and ingests only the unnamed one, and it replaces scalars with
  `"REDACTED"` where the key matches `auth|certificate|passphrase|password|token|
  key|secret`. A redacted value is not an absent one, and the type must not let a
  consumer confuse them. This is the same discipline as `Declared.Known`.
- **Health needs the tree, not the roll-up.** Fleet's `status` does not traverse
  the recursive `ComponentHealth` map, so a collector with a dead receiver reads
  as `online`. `Declared.Health` is a bare `string` today, which cannot carry
  that. If a broken pipeline should be detectable from health alone, the type has
  to change.
- **Fleet is blind to unconnected collectors**, having no discovery. So the
  `ungoverned` quadrant is only partly reachable through it, which matters for
  question 5.

Read tickets 01 and 02 in full before starting. Output the revised Go types,
unimplemented.

## Answer

Resolved 4 August 2026. The write path moved underneath this ticket during the
session, so read that part first: everything else follows from it.

### 0. Git stores, OpAMP serves

**Decided by the user, and it amends ticket 09.** That ticket revoked OpAMP as
the egress and left export-to-git as the only route. The user challenged this
directly: distributing configuration is what OpAMP is *for*, and Interchange
already has a working server that pushed config across four Baseline pushes in
ticket 06.

The two are not rivals, and the split is clean:

- **Git is the single source of truth.** It holds the rendered otelcol YAML, the
  history, the rollback and the approval, as a pull request. None of that is
  built.
- **The OpAMP server is stateless transport.** It reads git and serves. It
  stores nothing, so removing it loses delivery but never loses the record.

Ticket 09's evidence survives this, with one row weakened. Its finding that
every delivery target accepts an opaque file still holds, so the artefact is
unchanged. Its third reason for rejecting OpAMP, that applier sync status
answers "is it running what I asked", was already undermined by ticket 20:
Argo's check is whole-blob equality on an opaque string, and **51% of collector
deployments include VMs no GitOps controller can reach**. OpAMP is the only
channel indifferent to substrate.

**The cost, and ticket 09's question 2 is no longer moot.** The in-process
extension cannot accept remote config (ticket 01), so **the Supervisor is
mandatory beside every collector Amp-Up serves**. That changes the unit of
adoption and it is priced in a new ticket rather than waved through here.

### 1. Three readings, and the third is not a third axis

- **`Intended`** is read from git, pinned to a commit SHA, never a branch tip.
  Amp-Up is a lens over the repo (premise 9), so a config a human hand-commits is
  `Intended` too, indistinguishable from a rendered one. That is correct rather
  than a gap: premise 10 makes the pull request the authoring route, so editing
  the repo directly is legitimate. **No model-versus-git cross is built**, because
  going "around" the UI is not a thing to catch.
- **`Declared`** is the collector's own effective config and nothing else. Never
  what an applier holds, never what a ConfigMap contains. One definition across
  both populations, so a served collector and a foreign one are judged alike.
- **`Observed`** is unchanged.

The applied-versus-effective gap the GitOps route opens (ConfigMap updated,
collector never restarted) closes by itself for the served population: the
Supervisor writes the file, restarts the collector and reverts on failure. For
the foreign population it stays delegated to the applier, and the consequence is
stated rather than hidden: **Amp-Up cannot say which half of a foreign delivery
broke, only that it broke.**

### 2. The commit SHA travels inside the artefact

Pinning `Intended` to a SHA appeared to require server-side state, "which commit
did I serve to which collector", which would break the single-source-of-truth
rule and collide with ticket 19. It does not. **Stamp the SHA into the rendered
config:**

```yaml
service:
  telemetry:
    resource:
      ampup.commit: 9f3c1ad
```

The collector reports its effective config back, so the SHA returns *from* the
collector rather than being remembered *about* it. Three consequences:

1. **It works identically for the foreign population.** The stamp rides inside
   the file, so a collector delivered by Argo, SSM or a person reports its SHA
   exactly as a served one does. The settling window a HEAD comparison would
   have needed disappears for everybody.
2. **The OpAMP server holds nothing.** It deletes the `cp.offered[uid]` fallback
   that Interchange's control plane keeps at `main.go:188` for exactly the case
   where `RemoteConfigStatus` is absent.
3. **It survives Elastic Fleet.** `ampup.commit` matches none of
   `auth|certificate|passphrase|password|token|key|secret`, unlike
   `app.kubernetes.io/name`, which ticket 06 lost to that exact rule.

`service.telemetry.resource` scopes it to collector self-telemetry, so no
customer data is touched. **Deliberately not recommended now:** stamping the SHA
onto the telemetry itself via a `resource` processor would give the observed
reading config identity and a direct intended-to-observed link. Low cardinality,
so cheap, but it writes into customer data. Opt-in if ever.

### 3. Byte-exact drift detection is permanently unavailable

This ticket's question 2 asserted that "byte comparison is the only honest drift
check". **It is not available on either path, and neither is fixable.**

- **Served.** The Supervisor injects `extensions.opamp` at
  `ws://127.0.0.1:<port>/v1/opamp` and appends `opamp` to `service.extensions`
  (ticket 01). A config Amp-Up rendered and delivered verbatim comes back changed.
- **Foreign via Elastic Fleet.** Redaction on seven key substrings, plus silent
  dropping of *named* OpAMP config entries (tickets 02 and 06).

Hashing does not rescue it. A digest is byte equality compressed, with strictly
less information: it says "different" but never "different how", and it fires
every time on mutations that are expected. **The useful axis is what you hash.**

| Layer | What | Answers | Cost |
|---|---|---|---|
| 1 | digest of **raw bytes** | has this collector changed since last poll? | one hash, no parse |
| 2 | digest of the **normalised** form | is there drift? | one parse per *changed* collector |
| 3 | **structural diff** | drift where, and what do I fix? | only when layer 2 disagrees |

Layer 1 never compares across sources. It compares a collector against its own
previous self, so redaction is harmless: `REDACTED` is deterministic, so the hash
is stable, it simply never equals the rendered config's hash. Layer 2 is the
verdict, and it is semantic because it is the only layer that can be *equal when
the config is right*. Normalisation carries an explicit allow-list of expected
mutations, two entries today.

**Free reuse:** ticket 02 found Elastic Fleet's list endpoint carries a
`pipeline_config` fingerprint encoding `pipe:<name>[recv|proc|exp]`, so
normalised wiring for the whole estate arrives in **one call**. Filter on it,
then fetch per-agent `effective_config` only for collectors whose wiring moved.

**The cost, named so it is chosen:** normalisation is the one genuinely new
component and it is where the bugs will live. A bug there shows as permanent
false drift or, worse, silent no-drift on a real change. It must be the single
place mutations are allow-listed, and it needs tests against the known mutations
from tickets 01, 02 and 06.

### 4. The pipeline-structure defect is fixed

`model.Declared` at `model.go:142` holds `Receivers`, `Processors`, `Exporters`
as unordered `[]string`, so `has_receiver: [filelog]` passes for a collector
whose `filelog` is wired only into traces and delivers no logs. **That is the
exact `broken_pipeline` case the product exists to catch, and it currently
passes.** Ticket 02 proved the fix affordable and it is taken. `Declared` and
`Intended` both carry pipelines, and **processor order is preserved**: a
redaction processor before an exporter and after it are different systems.

Two prices, chosen rather than discovered:

1. `ConfigAssertion` grows a `Pipeline` scope. Requirements written as a bare
   `has_receiver` default to "any pipeline", which is the migration path.
2. Amp-Up now parses otelcol YAML properly, so the `type/name` convention,
   unmarshalling and normalisation become its problem rather than Elastic
   Fleet's.

### 5. The vocabulary is OpAMP's, unextended

A first draft invented five delivery statuses. **The user rejected the premise:
does this not already exist in OpAMP?** It does, and Interchange's own control
plane already uses it, comparing `msg.RemoteConfigStatus.LastRemoteConfigHash`
against its render hash at `main.go:185`.

`RemoteConfigStatus` supplies `UNSET`, `APPLYING`, `APPLIED` and `FAILED`, plus
the hash and an error string. **`FAILED` is the one the invented table missed**,
and it matters: a config that was delivered and then *rejected by the collector*
has a distinct owner (whoever wrote it) and a distinct remediation (fix it).
Without it, a config fault would be reported as `stale` and blamed on the
applier.

The two states the draft claimed OpAMP lacked both dissolve, and **nothing needs
upstreaming**:

- **`drifted` needs no protocol change.** `EffectiveConfig` is already reported,
  and ticket 01 established the Supervisor forwards the collector's own effective
  config upward, on by default. A server comparing that against what it offered
  *is* the drift check. It is a server-side derivation, not a missing state.
  Upstreaming it would not have helped anyway: drift persists in the **foreign**
  population, which is not speaking OpAMP to Amp-Up at all, while for the served
  population the Supervisor owns the file and rewrites it on restart.
- **`unmanaged` and `unknown` are categorically not protocol states.** They
  describe collectors not speaking the protocol. This is a fleet manager's
  concern one layer above OpAMP, and it is the same distinction `Declared.Known`
  already makes at `model.go:138`.

**Generalised at the user's instruction into a standing preference**: where a
concept exists upstream, use its name and its semantics rather than a synonym.
Where Amp-Up genuinely needs something upstream lacks, propose it there rather
than shipping a dialect, and hold a local definition only as a bridge that gets
deleted when the contribution lands. Premise 8 said "reuse over build" and said
nothing about contributing back.

### 6. Delivery status composes, it does not multiply

The obvious move is 2x2x2 and eight cells. It is wrong, because the readings are
not the same kind of thing. **`Declared` x `Observed` is per requirement.**
**`Intended` x `Declared` is per collector**: "is this collector running what I
asked for" does not vary by requirement, so multiplying it across N requirements
manufactures impossible cells and repeats one fact N times.

So the delivery status sits **beside** the conformance verdict and qualifies it.
The existing four-cell cross is untouched. **This is what the third reading
actually buys:**

| `broken_pipeline` with | Meaning | Owner |
|---|---|---|
| `applied` | config right, loaded, running, telemetry drained downstream | network, exporter auth, backend |
| `stale` or `drifted` | a **delivery** fault wearing a pipeline fault's clothes | delivery |
| `failed` | the collector rejected the config | whoever wrote it |

Today all of these are the same finding, sent to the wrong person.

**One genuinely new per-requirement outcome: `library_drift`.** Run the existing
`ConfigAssertion` checker a second time against the intended YAML from git.
Same checker, different input, no new machinery. When it fails, the config in the
repo no longer satisfies the tier, usually because the bar was raised and nothing
was re-rendered. Owner is the repo, remediation is re-render and open a pull
request, and no tool has this today.

**Absent stays distinguishable from empty, one layer up.** `Intended` gets a
`Known` of its own, and `unmanaged` (we see the collector, we do not author it)
never collapses into `unknown` (we cannot see it). This closes question 5, and it
is load-bearing: ticket 02 established Elastic Fleet has no discovery, so an
unconnected collector is **invisible**, not absent.

### 7. Health carries the tree, and the roll-up is never read

Ticket 02, finding 4, is a trap rather than a limitation. Elastic Fleet stores
and returns the full recursive OpAMP `ComponentHealth` tree, arbitrarily deep,
per pipeline and per receiver. But the roll-up `status` is flattened from
top-level health only, and fleet-server's own docs say the nested map "is not
traversed". **A collector with a dead receiver reads as `online`.** Interchange
has the same defect from the other end: `main.go:207` logs only
`msg.Health.Healthy`.

`Declared.Health string` at `model.go:145` cannot represent that. It becomes the
`ComponentHealth` tree, using OpAMP's shape and names, and **the roll-up is never
read**. The argument is the third leg of the same idea as delivery status: it
changes attribution, not the verdict. A healthy collector with no telemetry
drained downstream; an unhealthy receiver **names the failing component**. It
also fires faster, since a dead receiver is visible immediately while the
observed reading must wait out a window before absence means anything.

### 8. The seams change

`FleetProvider` at `provider.go:51` is wrong twice. **The name** says "Fleet",
which ticket 13 fixed to mean the Elastic product, inside the one package whose
doc comment promises the core imports nothing vendor-specific. That is premise 12
violated in a type name. **The shape** is keyed per application and returns only
`Declared`, while delivery status is per collector and ticket 02 found Elastic
Fleet returns the whole estate in one call.

- Rename to **`EstateProvider`**, ticket 13's word for the population.
- Key it on the **collector** and return the estate in one call. Application to
  collector matching is the platform's job, and ticket 07 already decided
  collectors are matched into a Stage by selector rather than drawn.
- It returns `Declared` **plus** delivery status, because for the served
  population both arrive over one connection and splitting them would be an
  artefact of the old design.
- **Two implementations from day one**, which is the real test: Amp-Up's own
  OpAMP server (rich, includes `RemoteConfigStatus`) and Elastic Fleet (effective
  config only, delivery permanently `UNSET`, because ticket 02 finding 5 shows
  `remote_config` is unimplemented and enrolment pins `PolicyRevisionIdx: 1`).
- `NoFleet` becomes `NoEstate`, unchanged in behaviour.

**Naming rule, added by the user and generalised to the whole project.** Seam
names are domain terms only, with no vendor word anywhere in `provider.go`.
Implementation names are fully qualified with the vendor's *product*:
`ElasticFleet` not `Fleet`, `Elasticsearch` not `Elastic`,
`GrafanaFleetManagement` not `Grafana`. A bare `Fleet` appears nowhere. This is
greppable, so premise 12 becomes a lint rather than a habit. It catches something
already shipped: `internal/provider/telemetry/elastic.go` names the **company**,
and the thing it talks to is **Elasticsearch**.

### The revised types, unimplemented

```go
// ---- readings ----

type ConfigDigest struct {
	Raw        string // digest of bytes as received. Change gate only, never compared across sources.
	Normalised string // digest of the normalised form. The drift check.
}

type Pipeline struct {
	Name       string
	Receivers  []string
	Processors []string // ORDER IS SIGNIFICANT and is preserved by both sources.
	Exporters  []string
}

// ConfigShape is the parsed, normalised form. Redacted is not cosmetic: a value
// Elastic Fleet replaced is not an absent one, and nothing may confuse them.
type ConfigShape struct {
	Pipelines  []Pipeline
	Extensions []string
	Redacted   []string
}

type Intended struct {
	Known  bool // absent means Amp-Up does not author this collector. Never "empty".
	Commit string
	Path   string
	Raw    []byte
	Digest ConfigDigest
	Shape  ConfigShape
}

type Declared struct {
	Known       bool
	CollectorID string
	Commit      string // from the ampup.commit stamp. Empty means unmanaged.
	Raw         []byte
	Digest      ConfigDigest
	Shape       ConfigShape
	Health      ComponentHealth
	Delivery    DeliveryStatus
}

// ComponentHealth mirrors OpAMP's recursive tree. The roll-up is never read:
// a collector with a dead receiver reports a healthy top level.
type ComponentHealth struct {
	Healthy    bool
	Status     string
	StatusTime time.Time
	LastError  string
	Components map[string]ComponentHealth
}

type DeliveryState string

const (
	// From OpAMP RemoteConfigStatus, verbatim.
	DeliveryUnset    DeliveryState = "unset"
	DeliveryApplying DeliveryState = "applying"
	DeliveryApplied  DeliveryState = "applied"
	DeliveryFailed   DeliveryState = "failed"

	// Derived server-side from EffectiveConfig. Not a protocol state, and does
	// not need to be: the protocol already carries the evidence.
	DeliveryDrifted DeliveryState = "drifted"

	// Above the protocol. A collector not speaking OpAMP to us cannot have a
	// protocol state, and unmanaged must never collapse into unknown.
	DeliveryUnmanaged DeliveryState = "unmanaged"
)

type DeliveryStatus struct {
	Known bool
	State DeliveryState
	Hash  []byte // LastRemoteConfigHash
	Error string // ErrorMessage, set only when DeliveryFailed
}

// ---- assertions ----

// Pipeline scopes the assertion. Empty means any pipeline, which is the
// migration default for requirements written before wiring was available.
type ConfigAssertion struct {
	Pipeline     string
	HasReceiver  []string
	HasProcessor []string
	HasExporter  []string
}

// ---- evidence and outcomes ----

type Evidence struct {
	Intended Intended
	Declared Declared
	Observed map[time.Duration]Observed
}

// OutcomeLibraryDrift: the config in git no longer satisfies the tier. Usually
// the bar was raised and nothing was re-rendered. Owner is the repo, remedy is
// re-render and open a pull request.
const OutcomeLibraryDrift Outcome = "library_drift"

// ---- seams ----

// EstateProvider answers: what collectors exist, what is each running, and did
// what we asked for arrive? Keyed on the collector, returning the estate in one
// call, because Elastic Fleet's list endpoint already does exactly that.
type EstateProvider interface {
	Collectors(ctx context.Context) ([]model.Collector, error)
	Name() string
}

type NoEstate struct{}
```

### What this pushed onto the rest of the map

- Premises 7, 9 and 11 amended for git-stores / OpAMP-serves.
- Ticket 09's question 2 un-mooted: the Supervisor is mandatory for the served
  population. Priced in **ticket 21**, raised by this ticket.
- Two standing preferences added to the map Notes: upstream vocabulary and
  contributing back; seam and implementation naming.
- **Ticket 19 shrinks.** The stamp means the OpAMP server holds no per-collector
  state, so what is left there is cache, not record.
- **Ticket 12 shifts.** `EstateProvider` needs two implementations from day one
  and Elastic Fleet is one of them, so the question is no longer whether to build
  the connector but what the abstraction must survive.
