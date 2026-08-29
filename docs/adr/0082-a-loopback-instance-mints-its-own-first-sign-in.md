# ADR-0082: An Instance on a loopback address mints its own first sign-in

- Status: accepted (amends ADR-0019 §1's users.yaml requirement for the loopback case)
- Date: 2026-08-29

## Context

ADR-0081 removed the toolchain from the first five minutes. What is left in
them is worse.

Somebody who has downloaded the CLI and cloned an estate cannot see the
console. `serve` refuses to start without `users.yaml`, and producing one
means running `telecraft passwd`, hand-writing YAML, and knowing that
`owner:` names an Owner from the team tree rather than a Team. Getting that
last part wrong is the likely outcome, because nothing on the path says so
until the server refuses to start.

Every one of those steps is right for a deployment. None of them is right
for somebody deciding whether to keep reading. The estate is the whole
membership story and stays that way, air-gap first and under review: that is
ADR-0019 §1 and nothing here weakens it. What it never settled is what an
Instance should do when there is no membership story yet and the only person
who could reach it is the person who started the process.

## Decision

### 1. No `users.yaml`, and a loopback console, mints one sign-in for the life of the process

When `users.yaml` is absent from the estate **and** `-http` is bound to a
loopback address, `serve` starts anyway. It mints one user in memory, acting
as an Owner of a root Team, with a password drawn at start, and prints both
once:

```
serve: no users.yaml in this estate, so this process minted one sign-in for
       itself. It is not written anywhere and it dies with the process.
         email     bootstrap@localhost
         password  <drawn at start>
       Add users.yaml to the estate to replace it.
```

Nothing is written to the estate. The credential lives as long as the
process, which is already true of the sessions it issues: the session key is
drawn at start for the same reason.

### 2. Off loopback, the absence stays fatal

A non-loopback `-http` and no `users.yaml` refuses to start, exactly as it
does today. This is what makes the whole thing safe to have: the minted
sign-in cannot be reached from another host, so it is not a credential the
network can find. The check is on the bound address and not on a flag,
because a flag can be set by somebody who has not thought about it and an
address cannot be set by accident.

### 3. It yields the moment the estate has an answer

`users.yaml` appearing at the next poll replaces the minted user. The estate
is authoritative the instant it says anything, and there is no state to
clean up because there was never any state.

The minted user is not written into any report, any finding's owner, or any
authored object. It signs in and reads. An estate with no `users.yaml` has
nothing anybody could author into anyway.

## Consequences

The quickstart's self-hosted path becomes: download, clone, serve, open the
browser. `telecraft passwd` and a hand-written `users.yaml` move to
[Run an Instance](../guides/run-an-instance.md), where the deployment is
being described.

The log line is the only place the password exists. A reader who loses it
restarts the process and gets another one, which is the correct cost.

This puts a second thing in the product that only holds on loopback, beside
`-insecure-http`. Two is a pattern and worth naming: the boundary this
product draws for a convenience is the address it is reachable at, never a
flag that says the operator meant it.

Nothing here changes what a deployment does. A deployment has a
`users.yaml`, because a deployment has people.
