# ADR-0054: The delivery cross judges the keys the artefact asserts, beside a structural check

- Status: accepted (amends ADR-0004's cross in practice; amends ADR-0005 and ADR-0046)
- Date: 2026-08-23

## Context

Point `telecraft delivery` at a real collector running exactly the artefact
the platform served it, and it reported `drifted` with 77 changes (issue
#110, found while verifying ADR-0052's local environment).

The 77 classify cleanly. Seventy-one are the collector expanding its own
defaults: `timeout: added 30s`, `max_idle_conns: added 100`, and sixty-nine
more like them. Two are re-spellings of a value that did not change —
`max_elapsed_time: 60s -> 1m0s` is the same duration, `metrics.level: normal
-> Normal` the same level. Four are shape rewrites, of which the worst reads
`service.telemetry.resource.telecraft.tier: removed platform/gateway`: the
Tier stamp is still there, re-encoded into the SDK's list of `{name, value}`
entries, but a reader has no way to tell that from a stamp genuinely going
missing, which is the one thing on that line worth an alarm (ADR-0013).

The two sides are not the same document and never can be. The artefact is
sparse because a human wrote the Blueprint it came from; the reported config
is fully defaulted because a collector emitted it. Neither is wrong.

ADR-0046 §2 anticipated part of this and drew the wrong conclusion from the
right premise. It struck "explicit defaults" from the cosmetic list on the
grounds that equating `batch: {}` with its defaults-expanded form needs every
component's default table, which lives in component Go code and the Catalogue
cannot supply — that part holds. It then reasoned that the equation was
unnecessary, because "both delivery paths report the merged *input* config,
never a defaults-expanded form". That is the sentence issue #110 falsifies.
The collectors the environment runs report a resolved, fully defaulted
configuration, and rendered-versus-reported therefore disagrees on defaults
alone, estate-wide, permanently.

Drift is one of the three bands. Red on every served collector from the
moment it connects, it carries no information, and the first thing anyone
does with a band like that is stop looking at it — including at the case it
exists for.

## Decision

### 1. The cross judges the keys the Intended artefact asserts

`Intended × Effective` is judged over the keys the artefact actually states,
against the projection of the collector's report onto those keys. A key the
artefact never mentions is not drift, whatever the collector defaults it to.

```
intended:  batch/batcher: {timeout: 2s}
reported:  batch/batcher: {timeout: 2s, send_batch_max_size: 0,
                           metadata_cardinality_limit: 1000, …}
judged:    timeout only -> no drift
```

Four rules make the projection, and the last three are what keep it from
being a licence:

1. A key the artefact does not mention is not projected, and so cannot read
   as drift.
2. A key the artefact spells with no body — `batch:` or `batch: {}` — asserts
   that the component is there and nothing about its settings. That is what
   an empty component body means in the configuration language, so it is what
   the cross reads it as.
3. A value carrying `${…}` asserts only its literal parts. The collector
   expands references at load, so `k8s.node.name: ${env:TELECRAFT_NODE_NAME}`
   against the node's own name is expansion, not drift; `https://${env:HOST}:4317`
   still holds its scheme and its port.
4. Everything else the artefact spells out is judged, **including list
   length**. Pipeline order is semantic (ADR-0004), so an asserted list is
   judged whole: a pipeline that grew a processor the artefact does not list
   is an asserted value that changed.

So all of these remain drift, and are tested as remaining drift: an asserted
key whose value differs; an asserted key absent from the report; a component
or pipeline the artefact describes that the collector is not running.

This is a property of the **cross**, not of the normalised form. It is not a
Mutation and could not be one — a Mutation sees one document and cannot know
what the other side asserted. Layer-2's per-document digests are unchanged,
ADR-0046 §1's digest identity is untouched, and ADR-0046 §2's ruling that two
*authored* configurations differing in spelled-out defaults are different
configurations stands exactly as written. What changes is which tree the
Effective digest is taken of: the projection, not the raw report.

### 2. A structural check at component and pipeline grain, which is what makes §1 payable

Judging only asserted keys means an addition can no longer read as key-level
drift. That is a real cost, and it is paid, not waved through: the addition
worth catching was never a defaulted setting. It is a whole exporter shipping
somewhere nobody rendered, or a pipeline nobody asked for.

So the cross also reports, separately, every **component and pipeline present
in the Effective reading that the Intended artefact does not describe** —
each entry of `receivers`, `processors`, `exporters`, `connectors` and
`extensions`, and each entry of `service.pipelines`. This check is not
optional; it is the reason §1's trade is acceptable.

It is reported apart from key-level drift because it answers a different
question. "A value you asserted is wrong" and "the collector is running
something nobody rendered" have different owners and different remedies, and
collapsing them into one list is what made the original 77 unreadable. Either
finding alone puts the comparison out of sync.

It runs on post-Mutation trees, so a delivery path's own catalogued
injections — the Supervisor's `extensions.opamp` (ADR-0046 §4) — are already
gone and never read as undescribed.

The check is one-directional by design. The other direction, a component or
pipeline the artefact describes and the collector is not running, is an
asserted key absent from the report, which rule 1 already leaves the layer-3
diff to report. A second mechanism for it would only report it twice.

### 3. Values compare by what they mean, in the layer that owns the rewriting

Three of issue #110's classes are one document expressing one thing two ways.
Each is neutralised where the rewriting happens, and nowhere wider:

**Durations are canonical form.** `60s` and `1m0s` are the same duration.
Every setting that reads a duration reads it through one grammar, so an
author's spelling of one is not a change to it. The narrowing is deliberate:
only a literal with a unit on every component is read as a duration, so a
bare `"0"` or a version string is never quietly equated with something else.
It runs in the canonical encoding rather than on the tree, so a layer-3
finding still prints the value the collector actually reported, and the
digest and the diff cannot disagree about equality.

**Telemetry levels are canonical form too.** The collector parses a level and
re-emits its own spelling, so `normal` comes back `Normal`. The fold is
scoped to `service.telemetry.{metrics,logs,traces}.level` and applies under
every profile, because the Effective reading is the collector's own report on
**both** delivery paths (ADR-0004) — a git-delivered collector title-cases
its levels exactly as a served one does. A case-blind comparer at large would
digest two genuinely different configurations equal, which is the silent
no-drift ADR-0005 fears most; scoping it to the enums is the price of not
doing that.

**The re-encoded resource is a supervisor Mutation.** The Supervisor rewrites
`service.telemetry.resource` from the authored map into the SDK's
list-of-`{name, value}` form, so it belongs in the `supervisor` profile,
where reading-path mutations live (ADR-0046 §3). It is matched by shape and
never by literal (ADR-0046 §4): a list whose every entry is a map carrying a
string `name` and nothing else. A duplicate name, or an entry with extra
keys, is left alone and reads as drift, because collapsing it would be a
guess that could lose an attribute silently. Reading the map back also
restores the Effective commit stamp, without which a served collector's
stale-versus-drifted split has nothing to split on.

### 4. What was rejected, and why

**Render the Intended artefact through the collector's own defaulting before
comparing.** Correct, and the honest source for the defaults is the collector
build itself — they are per-component and per-version, and they live in
component Go code (`createDefaultConfig`), not in anything the Catalogue can
read. Implementing it means either carrying a collector's configuration
machinery inside the platform, which makes the platform's verdict depend on
which components the platform was built with rather than which the collector
runs, or maintaining a default table by hand, which is a second
implementation of someone else's behaviour that goes stale on their release
schedule and fails silently when it does. ADR-0046 §2 already ruled the table
out; this ADR declines to build it in the other direction either.

**Probe the collector for its defaults.** Ask a running collector what it
defaults each component to, and compare against that. It needs a capability
no collector offers over OpAMP, so it means a second channel to every
collector, which the serving design does not have and ADR-0032's
statelessness and air-gap posture would have to absorb. It would also make
the verdict depend on a live round-trip to the thing being judged, so a
collector that cannot answer becomes a collector that cannot be judged. The
readings the product crosses are the ones it already has.

## Consequences

- A served collector running exactly what it was sent reads `in_sync`, which
  is what makes the band worth looking at. The environment's `drift` scenario
  moves grain with it: its lever was `send_batch_max_size`, a key the gateway
  artefact never asserts, which is now correctly excused, so the scenario
  adds a pipeline the artefact does not describe and the structural check
  reports it. Both halves stay in the file, so the scenario shows the trade
  rather than only its good side.
- A key the artefact asserts that the collector does not report back stays
  drift, and issue #110's `sending_queue.enabled: removed true` is that case.
  If a collector version drops a key the renderer emits, the finding is real:
  the artefact asserts something the running configuration does not carry,
  and the remedy is the renderer's, not the comparer's. Excusing it would
  need the default table §4 rejects.
- The blindness this buys is named and bounded: a setting the estate never
  described, changed on a component the estate did describe, is invisible to
  the cross. It is bounded by the structural check at the grain above it, and
  by the estate's own leverage — a setting that matters enough to detect is a
  setting the Blueprint should assert, and asserting it makes it judged.
  "Render it if you want it watched" is a workable rule; "watch everything
  and read red forever" was not.
- Delivery status gains a second finding list beside the layer-3 changes.
  Every surface that renders drift renders both, and a surface that shows
  only the change count under-reports.
- ADR-0046 §2's justifying sentence — that both delivery paths report the
  merged input configuration — is false for the collectors this platform
  serves, and should be read as withdrawn. Its ruling survives: the default
  table is still not built, and two authored configurations that differ in
  spelled-out defaults still differ.

## Sources

- Issue #110 (the 77 changes, their classification, and the acceptance
  criteria), found while verifying issue #108 and ADR-0052.
- ADR-0004 (the three readings and the Intended × Effective cross), ADR-0005
  (the three-layer scheme), ADR-0013 (the artefact carries its own identity),
  ADR-0032 (statelessness and air-gap), ADR-0046 (profiles, the struck
  explicit-default equation, shapes never literals).
- `devenv/estate/rendered/platform/gateway.yaml` and the reported
  configuration it comes back as, carried into
  `internal/normalise/testdata/served/`.
