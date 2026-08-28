---
title: Place a licence
description: "Where the licence file goes, what telecraft licence prints, and what an Instance does in each of the four states a licence can be in."
order: 14
---

# Place a licence

Telecraft runs in one of two Editions.

**Standard Edition** is Telecraft as this documentation describes it. It needs
no licence, costs nothing, and is unrestricted in production and commercially.
It is what the public demo runs and what you get by doing nothing.

**Enterprise Edition** is Standard Edition plus the capabilities a valid
licence names. One capability is behind it today: running more than one
Organisation from a single deployment.

A capability Standard Edition offers today stays in Standard Edition. Nothing
you use now moves behind the licence in a later release.

## Read what you are running

```sh
telecraft licence
```

```
Standard Edition
```

That is a whole answer, and it is the ordinary one. With a licence file:

```sh
telecraft licence -licence-file /run/licence/acme.licence
```

```
Enterprise Edition, licensed to Acme Ltd, expires 3 March 2027
  licensee      Acme Ltd
  licence       tc-2026-0007
  issued        1 August 2026
  expires       3 March 2027
  entitlements  many-organisations
  file          /run/licence/acme.licence
```

The command reports rather than judges, so it exits `0` whatever it finds.

## Place the file

The licence is one file. Put it where the process can read it and name it:

```sh
telecraft serve -estate ESTATE_DIR -licence-file /run/licence/acme.licence
```

or through the environment, like every other flag:

```sh
export TELECRAFT_LICENCE_FILE=/run/licence/acme.licence
telecraft serve -estate ESTATE_DIR
```

There is no default path, so nothing is read that you did not name. The file
is not a secret and does not belong under `-secrets-dir`: it is signed, and it
grants nothing to whoever reads it.

The file is read at start and again whenever it changes on disk, so replacing
it takes effect without a restart.

## What each state does

| The file | What the Instance does |
|---|---|
| None named | Standard Edition. Nothing is wrong, and nothing says anything is. |
| Named and not accepted | Standard Edition, and one line on the terminal names the file and what is wrong with it. Every surface that names the Edition says the file was not accepted. |
| Valid, inside its dates | Enterprise Edition, with the capabilities it names. |
| Valid, expired or not started yet | Enterprise Edition. What you already run keeps running; what is refused is widening, such as adding another Organisation, and the refusal names the date. |

Dates are read against the host clock. A host whose clock is wrong reads its
licence as outside its dates, which you can correct yourself.

One rule sits above all four: **a licence never changes what a collector
receives.** No state here touches the renderer, the OpAMP endpoint, the
readiness probe, or the artefact a collector fetches. An Instance starts and
serves whatever its licence says and whatever it fails to say.

## Where the Edition appears

In the console, under the version in the profile section of the chrome:

```
Enterprise Edition, licensed to Acme Ltd, expires 3 March 2027
```

An expired licence reads `expired 3 March 2027` in the same place, in the same
quiet type. It is a fact about your session, like the version above it.

Where a capability is unavailable, the surface that would have offered it says
so in one sentence, and says what would make it available.

## Verification never leaves the host

The signature is checked against keys inside the binary. There is no
activation step, no registration, and no call to us at any point, so an
air-gapped Instance verifies exactly as a connected one does.

The licence names you and never a machine. Nothing is counted against it, and
copying the file to a second Instance works. What a licence covers is what
your agreement says it covers.

## What next

- [Organisation register format](../reference/organisations.md) is the record
  a deployment reads to run several Organisations.
- [Run an Instance](run-an-instance.md) is the process the licence file is
  placed for.
