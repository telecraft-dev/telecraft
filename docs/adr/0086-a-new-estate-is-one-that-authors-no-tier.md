# ADR-0086: A new estate is one that authors no Tier, and the Instance serves it

- Status: accepted
- Date: 2026-08-30

## Context

ADR-0082 got a reader as far as the sign-in surface of a brand-new estate.
It did not get them any further. `telecraft init` writes a team tree, a
user and nothing else; `telecraft serve` starts over it, a reader signs in,
and every estate document answers 503. The console the reader reaches is
empty, and the flow ADR-0060 §1 gives them to author a first Tier is one of
the documents that is not there.

It is not one refusal. It is a chain, each link reached only once the one
before it is fixed: the rendered tree the documents are computed at is
absent, then the requirements library directory is absent, then `rows.yaml`
is absent, and there is no reason to think that is the end of it.

The tempting fix is to make each loader tolerate the absence it refuses.
That was tried and it is wrong, and there is a test that says so.
`devenv`'s `TestLoadInputsNamesTheInputItCannotRead` asserts, deliberately,
that an estate with its requirements library removed is an error. It is
right to. Every one of these refusals guards the same thing: a verdict
computed from nothing. An absent requirements library would score every
Tier compliant against a floor nobody wrote. An absent `rows.yaml` would
pass every check. An absent rendered tree would let collectors be described
as serving a config the sources no longer hold. Under-governed is the
failure mode this product exists to refuse (ADR-0025 §4), and each of these
loaders refuses it correctly.

So the loaders are not the question. The question is what a new estate is,
and that had never been decided. It was being decided one loader at a time,
by whoever hit the next 503.

## Decision

### 1. A new estate is one that authors no Tier

Not one missing a rendered tree, not one missing a requirements library,
not one missing `rows.yaml`. Those are absences of things, and an absence
of a thing is not a state of an estate.

The Tier is the unit of rendering and of governance (ADR-0025). Every input
in the chain above exists to render a Tier or to judge one: the rendered
tree is what Tiers render to, the requirements library is what they are
judged against, `rows.yaml` and the Catalogue artefacts are what the
judgement reads. An estate that authors no Tier has nothing for any of them
to describe.

That is what makes this a boundary rather than a convenience. The refusals
guard a verdict, and an estate with no Tier has no verdict for them to
guard. There is nothing to be lenient about, so nothing is being made
lenient.

### 2. It is decided once, over the estate, before any input is read

Newness is a property of the estate, and it is established in one place:
the topology load, which already distinguishes an estate that authors no
Tier from one whose objects cannot be read (`ErrNoTiers` and
`ErrNoTeamsTree`, ADR-0060's groundwork). `renderer.NoTiersAuthored` asks
that question, and it is the only place the two sentinels are spelled out.

Deciding it there, and first, is what keeps this from becoming the chain
again. On a new estate none of the inputs below is read, so none of their
refusals is reached and none of them needs a new tolerance. On any other
estate every one of them is read and every refusal stands exactly as it
did.

This is why the state is not read off the rendered tree, which was the
first thing tried and is the one reading that quietly breaks. An estate
that authors Tiers and has lost its rendered tree looks identical from
there, and serving that as an empty estate is precisely the lie the
recompute invariant exists to prevent (ADR-0028 §2).

### 3. Its documents are computed, not refused, and nothing in them is invented

The Instance builds the document set of a new estate from what a new estate
actually has: the team tree it was created with, and empty everything else.
The documents are the same projections a full build produces, run over an
estate that genuinely holds one team and no objects, so every empty list is
a reading rather than a placeholder.

The readings are still taken. The seams are live whatever the estate holds,
and what comes back is an empty reading rather than no reading, which is
the difference between an estate with nothing in it and an Instance that
has not looked. The 503 stays for the second case, which is what it was
written for.

### 4. Serving a new estate widens nothing a collector may receive

ADR-0010 and REQ-002 stand. An artefact is rendered from a Tier, a new
estate has none, so there is nothing to serve and nothing is served. The
serving path already reached this conclusion for itself: a snapshot of a
new estate carries no entries and no Unmatched artefact, a collector that
connects is told nothing, loudly, and that is unchanged. What this decision
does is make the document build agree with the serving path instead of
refusing where it serves.

### 5. Authoring the first Tier ends the state, and every refusal returns

The estate stops being new the moment it authors a Tier. From that commit
there is a verdict again, so there is something for each of the refusals to
guard again, and each of them applies unchanged: a Tier with no rendered
tree is unrendered, and a Tier judged against no requirements library is
judged against nothing.

That is the intended boundary and not an oversight. It does mean an estate
between its first Tier and its first library is refused, which is a real
gap and a narrow one: the console cannot author a Tier until the estate
holds a Blueprint and a declared Environment, so the estate that can reach
that state has already been authored into well past the seed.

## Consequences

`telecraft init` followed by `telecraft serve` now gives a console a reader
can sign in to and read. That was the whole of the circle ADR-0072 §4 and
the hosted guide already promised and the product did not keep.

`renderer.NoTiersAuthored` is the one place the question is asked. A third
sentinel, if one is ever needed, is added there and every caller gets it;
callers spelling the sentinels out separately is how the answer would
drift.

`render` and `check` are unaffected and still refuse a new estate. They are
right to: pointing either of them at an estate with no Tier is almost
always pointing them at the wrong directory, and neither of them is the
console somebody adds their first Tier through.

The document build now has two entry points rather than one branch inside
it. That is deliberate. A branch inside the build would put the decision
next to the loads it exists to skip, where the next person to add a load
would have to notice it; two entry points put it above them, where it is
the first thing either path passes.

## Sources

- ADR-0010 and REQ-002 (an artefact is rendered from a Tier, and nothing
  empty reaches a collector)
- ADR-0025 §4 (the Tier is the rendering unit, and under-governed is the
  failure mode)
- ADR-0028 §2 (the recompute invariant, and why the rendered tree cannot be
  the discriminator)
- ADR-0060 §1 (the flow that authors a first Tier is in the console)
- ADR-0072 §4 (a created Organisation's address answers on creation)
- ADR-0082 (an Instance on loopback mints its own first sign-in, which got
  a reader as far as the sign-in surface)
