---
title: Organisation register format
description: "The register a deployment reads to run several Organisations: one record per Organisation, its fields, the rules the loader applies, and register-check."
order: 11
---

# Organisation register format

One Telecraft deployment can run several Organisations. Each Organisation is
one Instance: one process, one estate, one set of people, and its own
address. Nothing is shared between two of them.

The **register** is how a deployment knows which Organisations it runs. It is
a directory of authored records held in git, one file per Organisation, and
it is reviewed like every other configuration in Telecraft: adding a record
is a pull request, and merging it is what creates the Organisation.

A deployment that runs one Organisation has no register and needs none.
`telecraft serve` has no Organisation flag, no register flag, and no tenancy
setting: it serves one estate, which is what it has always done.

Running more than one Organisation from a single deployment needs an
Enterprise Edition licence. A deployment with no licence runs one, and
everything else on this page still applies to it. See
[Place a licence](../guides/licensing.md).

## Where the register lives

Its own repository, separate from any estate and separate from the code that
reads it. It holds names, addresses and lifecycle state, and nothing that any
Organisation authored.

One record per file, and the file is named for the Organisation it holds:

```
acme.yaml
beacon-rail.yaml
corvid-freight.yaml
```

Two sign-ups then arrive as two files rather than as two edits to one, and a
review reads one record at a time.

## The record

```yaml
name: beacon-rail
display_name: Beacon Rail
state: active
address: https://beacon-rail.telecraft.example
estate:
  kind: connected
  repository: https://git.example.com/beacon-rail/estate.git
administrators:
  - oidc:8f21c0ac
```

| Field | Required | Meaning |
|---|---|---|
| `name` | yes | Addresses the Organisation, and names the file it lives in. Lower-case letters, digits and hyphens, no leading or trailing hyphen, 63 characters at most. |
| `display_name` | yes, while active | What surfaces show. Any text. |
| `state` | yes | `active` or `retired`. |
| `address` | yes, while active | The URL the Organisation's Instance is reached at, over `http` or `https`. |
| `estate.kind` | yes, while active | `hosted` for a repository the deployment keeps, `connected` for one the Organisation owns. |
| `estate.repository` | for a connected estate | The git remote the estate is read from. |
| `administrators` | no | The identity subjects that hold the account. Account authority grants nothing inside an estate, and owning something in an estate grants nothing on the account. |

A hosted estate names no repository. It is created with the Organisation, so
the deployment already knows where it is.

## The rules the loader applies

Each of these refuses the whole register and names the file it found the
problem in. Every problem in every file is reported at once, because the
register is read as one reviewed change.

- **A name belongs to one Organisation, and is never issued twice.** A
  retired Organisation keeps its record, which is what holds its name.
- **The record's `name` matches its file name.**
- **No two Organisations are reached at one address**, and **no two read one
  estate**.
- **Nothing here carries a credential.** There is no field that takes secret
  material, and an address or a remote carrying a password is refused. A
  refusal names the file and never repeats the value. Credentials are files
  the deployment places, and the estate names them.
- **A record that is still run for names what running it needs**: a display
  name, an address, and an estate.
- **A retired record needs only its name and its state.** Its Instance is
  gone and there is nothing left to address.

## Lifecycle

`active` is an Organisation the deployment runs an Instance for. `retired` is
one whose Instance has been destroyed; the record stays, and it is never
provisioned again.

A record that disappears from the register altogether is not a retirement.
The deployment reports the Instance it is running and the register does not
name, and destroys nothing: retiring an Organisation is deliberate.

## register-check

`register-check` loads a register and prints what it holds. Run it on the
pull request that changes one.

```sh
go build ./cmd/register-check
./register-check REGISTER_DIR
```

`REGISTER_DIR` is the directory of records; with no argument it reads the
current directory.

```
loaded 3 Organisations, 2 active
  acme         active   https://acme.telecraft.example         hosted estate
  beacon-rail  active   https://beacon-rail.telecraft.example  https://git.example.com/beacon-rail/estate.git
  corvid       retired
```

| Exit code | Meaning |
|---|---|
| `0` | The register loaded. |
| `1` | The register did not load. Every problem is on stderr. |
| `2` | The arguments were not understood. |

An empty directory loads and reports nothing. A deployment that serves no
Organisation yet has an empty register.
