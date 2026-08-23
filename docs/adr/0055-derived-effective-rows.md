# ADR-0055: The Effective reading per row is derived from the collectors that report it

- Status: accepted
- Date: 2026-08-22

## Context

`rows.yaml` is the last place the product can be told something untrue and
believe it.

Every other reading in the product is read from something. The collector
estate arrives through the EstateProvider seam (ADR-0008, ADR-0036), the
arrivals and the flow through the TelemetryProvider seam (ADR-0039,
ADR-0040), delivery status off the OpAMP wire (ADR-0004). The Effective leg
of the verdict cross — the pipelines the collector serving a Service is
running — is typed into a file and kept in step with `rendered/` by hand. The
demo estate authors it (ADR-0049), the devenv authors it (ADR-0052 §2), and
`telecraft check` takes it as `-estate`.

So `telecraft check` will confirm a pipeline that no collector runs, because
the file says so. That is the exact failure the product exists to catch, one
level up: a claim about collection that nothing collected was consulted for.

The parts to close it are already built. A Service's Paths name its Tiers
(ADR-0007), a Tier declares exactly one Environment and one selector
(ADR-0025 §2, ADR-0007), and the serving matcher already resolves reported
identifying attributes to a Tier. What was missing is not machinery. It is
three judgements the product had not made, which is why #108 stopped short
and said so rather than inventing them in a development environment.

They are: which collector represents a row; what a disagreement between
replicas of one Tier means; and what a Path through several Tiers implies for
a requirement asked of one pipeline set.

## Decision

### 1. A row's Effective reading is the collector on the first Tier of the Service's Path

The row is answered by the collector nearest the Service: the first Tier of
each Path the Service takes. The thing collecting *for* a Service is what
decides whether that Service is instrumented, and a requirement like
`has_receiver: [filelog]` is a question about that thing.

The Environment falls out of the same rule. A Tier declares exactly one
Environment, so a Service's Paths partition by their first Tier's
Environment, and each partition is one row (ADR-0033: the same Service in two
Environments is two rows, judged independently). Where several Paths enter
one Environment through different first Tiers, their collectors form one
population and §3 governs what happens if that population disagrees.

This is what the authored rows already encode. Deriving it therefore changes
no verdict on any existing estate, which is the property that makes it
adoptable: the derivation can be turned on and compared against the file it
replaces, and a difference is a finding about the estate rather than about
this decision.

### 2. Its weakness is accepted, and this is where it is written down

A failure further down the Path does not appear in the Service's row.

`checkout` traverses edge then gateway. If the gateway drops traces — a
receiver missing, an exporter wired into the wrong pipeline — the Effective
leg of `checkout`'s row still reads exactly as before, because the gateway is
not the collector the row is derived from. The row is silent about a real
failure that affects the Service.

This is accepted rather than solved, for two reasons. The failure is not
invisible: the arrivals stop, so the cross lands on `broken_pipeline` on the
Observed leg, which is the diagnosis this product exists to produce
(ADR-0004). And the gateway's own misconfiguration is the gateway team's
finding, routed to its owner (ADR-0016, ADR-0022) — relocating it into every
traversing Service's row would report one fault many times, to people who
cannot fix it.

The residual gap is real and narrow: an aggregation Tier that is
misconfigured while arrivals continue by some other route. Nothing in this
decision catches that, and §7's alternative would not have caught it either.

### 3. Replicas that disagree read as unknown, with the disagreement as the cause

A Tier has many collectors. The row is one reading. Two collectors matched
into one Tier can report different configs, mid-rollout or because one failed
to apply, and the derivation must say something.

Every candidate that produces a value produces a guess. Newest lets a
rollout's leading edge speak for the whole population. Majority hides one
failed applier until half the fleet has failed. Worst has no definition for a
config: there is no ordering on pipeline sets that means anything. Each one
converts "we do not have a single answer" into an answer, which is precisely
the fabrication `internal/estate` was shaped to make impossible — `Known
bool` plus `Cause string` exists so degradation can be data (ADR-0008).

So: the row is Known only when every matched collector's Effective reading is
Known and they all agree. Otherwise the row is Known false, and the cause
states the Tier, how many collectors were matched, and how many distinct
configs they reported.

Two details follow from that sentence:

- **Agreement is compared with component order preserved** (ADR-0004): a
  receiver wired into the wrong pipeline is a difference, not noise. Pipeline
  ordering between collectors is canonicalised by pipeline name before
  comparison, because pipeline order in a report is not a property of the
  config.
- **One unreadable replica makes the row unknown**, even when the readable
  ones agree. Agreement that cannot be established has not been established,
  and the strict direction is the one that refuses to answer.

This is deliberately noisy during a rollout. That is correct: mid-rollout the
population genuinely holds two answers, and a row claiming one of them is
claiming something no collector can support. Unknown never rounds to a pass
(ADR-0008): with an arrival reading in hand the cross still lands on
`compliant` or `not_delivered`, and never on `not_configured`, because
`not_configured` is an accusation the Effective reading has to earn.

### 4. No collector matched is unknown, never "nothing configured"

A Service whose first Tier has zero reporting collectors has no Effective
reading. The row is Known false with a cause naming the Tier and its
selector.

That is a different statement from a collector that reports an empty config,
which is a known reading of nothing and is judged as such. Collapsing the two
would turn a blind spot into an accusation against a team whose collectors
the platform simply never saw.

A first Tier that declares no selector is the same answer for a different
reason: a Tier delivered by git alone has no platform-known population
boundary (ADR-0030, ADR-0035 §2), so no collector can be attributed to it,
and the cause says that rather than pretending the population is empty.

### 5. Staleness demotes before derivation, never after

The estate reading passes through `ForEvaluation` before any row is derived
(ADR-0036 §3). A collector past the staleness horizon arrives at the
derivation already Known false with its payload gone, so it counts as an
unreadable replica under §3 rather than voting with a config it stopped
confirming hours ago.

Freshness stays the platform's arithmetic and never the provider's claim.

### 6. `rows.yaml` becomes an override, and an override says why

The authored estate does not stop existing, and it does not stop winning. A
row authored for a (Service, Environment) overrides the derived one, because
quietly discarding an operator's explicit statement is its own kind of lie,
and because the rows file carries two things no topology holds: the Grace
table and each Service's onboarding date (REQ-014).

What changes is that an override is visible and carries a reason. The estate
file gains an optional `reason:` per (Service, Environment), and
`telecraft check` reports every overridden row with its reason, plus an
`overridden_rows` total in the summary. The posture is the one exemptions
already have (ADR-0037): a green built on overrides is visibly built on
overrides, and an override with no stated reason renders as "no reason
stated" rather than as nothing at all.

The reasons that survive this decision are:

- **A Service the platform cannot see.** Git-delivered collectors, a foreign
  population, a Tier with no selector — ungoverned-in-view (ADR-0031). The
  override is how a human supplies what no seam reaches.
- **An estate with no live reading at all.** The demo (ADR-0049) is a
  repository and a browser, with no collectors anywhere. Its rows are
  authored because there is nothing to derive them from, and that is the
  honest shape rather than a defect.
- **The Grace table and onboarding dates**, which are Service facts rather
  than topology facts and stay authored either way.

`telecraft check` with no estate reading in reach behaves exactly as it does
today: `-estate` alone, every row authored, no derivation. Nothing regresses
for anyone who has not opted in.

### 7. `telecraft check` reaches the seam through a recorded reading

CI is not in reach of a live OpAMP server, and the derivation must not grow a
second way of getting collectors. So the estate reading `check` derives from
comes through the EstateProvider seam like every other one, from a third
implementation: `Recorded` in `internal/provider/estate`, reading a file that
holds one estate reading — identity and Effective config per collector, an
`as_of`, and a declared refresh cadence. It is certified by the shipped
conformance kit (ADR-0036 §4) exactly as the other two are.

A recording is a reading, not an assertion, and the difference is load
bearing: it carries a timestamp and a cadence, so §5's arithmetic applies to
it. A recording left behind in a repository goes stale and demotes to
unknown. An authored `rows.yaml` never could, which is the whole complaint
this ADR answers.

## Rejected alternative: judge every Tier on the Path

The obvious alternative is to derive the row from every Tier the Service
traverses, unioning or intersecting their pipelines.

It was rejected on three counts.

It changes the question. `has_receiver: [filelog]` asked of a collector means
"the thing collecting for this Service picks the logs up". Asked of a Path it
means "somewhere along the route, something has a filelog receiver", which is
satisfied by a gateway that never sees the Service's files. The requirement
vocabulary (REQ-021, ADR-0034) is written against a pipeline set that one
collector runs, and a Path is not that.

It changes existing estates' scores, in both directions and neither of them a
bug fix. Unioning weakens every row: a Service whose own collector is
misconfigured passes on a gateway's components. Intersecting fails almost
every row: a gateway does not run the Service's receivers and is not supposed
to.

And the question it answers belongs elsewhere. Whether the whole Path carries
the Service's data is exactly what the Observed leg measures, and whether a
given Tier is configured correctly is that Tier's own finding with that
Tier's owner attached. Neither needs to be smuggled into a Service's row.

## Consequences

- Two readings of the same estate now exist wherever both a recording and a
  `rows.yaml` are present, and they can disagree. That is intended and it is
  the point: the disagreement is a finding about the estate, surfaced as an
  override with a reason rather than resolved silently.
- Rows become less stable run to run. A row that read as a fixed config now
  goes unknown during a rollout, when a collector goes quiet, and when a
  replica fails to apply. Anything treating a row's Effective reading as a
  constant will notice; that instability is the estate's, and it was always
  there.
- The devenv and the demo keep authoring rows until they are wired onto the
  derived path, so both remain examples of the shape this ADR argues against.
  Wiring the devenv is a follow-up (#112), and the demo may never be wired,
  because §6 already says an estate with no collectors has nothing to derive
  from.
- The residual blind spot in §2 is now written down, so the next person to
  find it finds a decision rather than an oversight. If it needs closing, it
  closes as a Tier-owned finding, not by redefining the row.
- A third EstateProvider implementation means the seam is exercised by three
  shapes rather than two, which is what ADR-0008 wanted verification to look
  like. It also means one more implementation to keep passing the kit.

## Sources

- ADR-0004 (the three readings and the cross, pipeline order preserved),
  ADR-0007 (Tier, Path, selector), ADR-0008 (the EstateProvider seam and not
  knowing as a normal state), ADR-0025 §2 (one Environment per Tier),
  ADR-0030 (the Unmatched artefact and the population boundary), ADR-0031
  (ungoverned in view), ADR-0033 (per-Environment rows), ADR-0035 §2
  (declared population floors), ADR-0036 (the provider contract and its kit),
  ADR-0037 (waivers stay visible), ADR-0049 (the demo), ADR-0052 §2 (why the
  devenv left this declared).
- Issue #112, which raised it, and issue #108, which found it.
