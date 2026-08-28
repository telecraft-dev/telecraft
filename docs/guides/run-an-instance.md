---
title: Run an Instance
description: Serve the console, the platform API and the OpAMP endpoint from one process over one estate, with sign-in and health probes.
order: 6
---

# Run an Instance

An Instance is one running Telecraft: one process over one estate, with its
own users, its own activated versions and its own verdicts. The same
`telecraft serve` that delivers configuration to collectors also serves the
console you sign in to and the API behind it.

It holds nothing that outlives it. Restarting signs everybody out and loses no
record: users, teams, governance and every authored object live in the estate
repository, and the readings come off the wire.

This guide assumes you have [rendered an estate](author-and-render.md) and
have [served configurations](serve-configs.md) from it. Everything on this
page is that server with its second address opened.

## Build the binary with the console inside it

The console travels inside the binary, so one artefact is the whole Instance.
Build it in two steps, from the repository root:

```sh
cd console
npm ci
npm run build
npm run bundle
cd ..
go build -o telecraft ./cmd/telecraft
```

`npm run bundle` stages the built console where the binary embeds it from.
Skip those four lines and the binary still builds and still serves the API;
the console route then answers a page saying the console was not built into
it.

## Give somebody a way in

Sign-in needs one user the estate knows. Hash a secret, then add the user
beside `teams.yaml` in your estate checkout:

```sh
./telecraft passwd
```

The secret is read from standard input and the hash is printed. Put it in
`users.yaml`:

```yaml
users:
  - email: jo@example.com
    name: Jo Author
    owner: gateway-owners
    password: pbkdf2-sha256$600000$...
```

`owner` names an Owner in the team tree. What Jo may author follows from that
Owner's Team, so there is no second place to grant anything.

Commit the file. Everything the Instance reads about people is under review
like every other authored object.

## Run it

```sh
./telecraft serve -estate ../estate-demo
```

```
console and API on http://127.0.0.1:4321
OpAMP on 127.0.0.1:4320
the session key was drawn at start, so sessions last as long as this process
serve: serving head 870c9b8a26458402c1982359bcdea90fdb7ef73d on 127.0.0.1:4320, fetch interval 30s
```

Open <http://127.0.0.1:4321> and sign in with the email and secret you
hashed. The console reads the estate at the head the server is serving, and
picks up a merge on the next poll without a restart.

Humans and collectors arrive on separate addresses, so you can expose one and
not the other:

| Flag | What listens |
|---|---|
| `-http` | The console, the API and the two probes. |
| `-listen` | The OpAMP endpoint, at `/v1/opamp`. |

The OpAMP endpoint closes on an empty address:

```sh
./telecraft serve -estate ../estate-demo -listen ""
```

```
console and API on http://127.0.0.1:4321
the OpAMP endpoint is closed
```

That is the shape of an Instance whose collectors are all Foreign.

## Put TLS in front

The process holds no certificate. Both endpoints speak plain HTTP, and TLS
terminates in an ingress, a load balancer, a reverse proxy, or nowhere at all
on a loopback address.

Tell the process what the outside sees:

```sh
./telecraft serve -repo https://forge.example/acme/estate.git \
  -http 0.0.0.0:4321 \
  -external-url https://telecraft.example
```

`-external-url` does two things. Its scheme decides whether session cookies
are marked Secure, and it is the address a redirect sign-in returns to.

It fails closed. An external URL naming a host that is not a loopback
address, over plain HTTP, is refused:

```
serve: the external URL "http://telecraft.example" sends passwords and sessions across a network in clear text. Terminate TLS in front and name the https URL, or pass -insecure-http to say that plain HTTP is meant here
```

Add `-insecure-http` if you mean it.

## Place the secrets

Telecraft reads secret material from files in one directory, whatever the
deployment shape. Nothing carries a value on the command line or in the
environment.

```sh
./telecraft serve -estate ../estate-demo -secrets-dir /run/secrets
```

A host process fills that directory with files owned by the service user. A
compose file presents its `secrets:` block at `/run/secrets`. A Kubernetes
deployment projects a Secret as a read-only volume. Whatever writes the
files, filling the directory is the whole interface: rotating a secret is
rewriting its file, with nothing to restart and nothing to renew.

Two files have documented names, and both are optional:

| File | What it holds |
|---|---|
| `session-key` | The session signing key, at least 32 bytes. |
| `telemetry-key` | The credential for the telemetry backend. |

Point either somewhere else with `-session-key-file` or
`-telemetry-key-file`. A path you name and the process cannot read stops the
start; a file you never placed is an absence, and an absence means the
capability is unavailable rather than broken.

## Keep sessions across a restart

A session is a signed token, never a record. With no key placed, one is
drawn at start and every session ends with the process:

```sh
head -c 32 /dev/urandom > /run/secrets/session-key
./telecraft serve -estate ../estate-demo -secrets-dir /run/secrets
```

Two processes with the same key accept each other's sessions. Rotating it
signs everybody out, which takes a restart and is what a suspected
compromise wants.

## Offer sign-in through your identity provider

Basic auth is the bootstrap. For everyone else, declare the providers in
`auth.yaml`, beside `teams.yaml` and `users.yaml`:

```yaml
providers:
  - kind: oidc
    name: staff
    issuer: https://issuer.example
    client_id: telecraft
    secret: staff-oidc
  - kind: basic
```

Each entry names its kind, the name the sign-in surface shows, its issuer,
and the *name* of its secret. No field takes a value. Place the client
secret in a file of that name:

```sh
printf %s "$CLIENT_SECRET" > /run/secrets/staff-oidc
```

A secret name is lower-case letters, digits and hyphens, so a name can never
describe a path.

Register `https://telecraft.example/api/v1/auth/staff/callback` as the
redirect URI with your provider, replacing `staff` with the name you gave the
entry.

Editing `auth.yaml` is a pull request, exactly like editing who may author. A
file that names a secret nobody placed stops the start rather than
withdrawing sign-in quietly:

```
serve: provider "staff": the secret "staff-oidc" is named, and there is no file of that name in /run/secrets
```

With no `auth.yaml`, the Instance offers basic auth alone.

## Probe it

Two paths answer without a session, and answer a status word and nothing
else:

| Path | Answers |
|---|---|
| `/healthz` | `200 ok` while the process runs. |
| `/readyz` | `503 starting` until the first snapshot is held, `200 ready` after it. |

A later fetch that fails keeps the last snapshot and readiness stays green: a
stale head still serves correct configuration for the commit it names.

There is one probe for the whole process. The OpAMP endpoint gets none of its
own.

## Configure it from the environment

Every flag has an environment variable: `TELECRAFT_` plus the flag name
upper-cased, with dashes as underscores. A flag beats an environment
variable, which beats the default.

```sh
export TELECRAFT_ESTATE=/srv/estate
export TELECRAFT_HTTP=0.0.0.0:4321
export TELECRAFT_EXTERNAL_URL=https://telecraft.example
export TELECRAFT_SECRETS_DIR=/run/secrets
export TELECRAFT_FETCH_INTERVAL=15s
./telecraft serve
```

There is no configuration file. What describes the estate is authored in the
estate; the process carries only what git must not, and every secret among
that is a file rather than a value.

## What this Instance answers

Every read endpoint of the platform API is served from the estate at head:
the shelf, the drawers, the collector list, the topology, the rollout ledger,
the Blueprints, the Catalogue and its retained versions, the activations and
the governance policy.

The endpoints that propose a change, and the two evaluators the composing
surfaces call, are not answered yet. They come back as:

```json
{ "error": "this instance does not answer /api/v1/validate yet" }
```

So Compose and the claim flow do not work against an Instance yet. Reading
the estate does.

## What next

- [Stage a Rollout](stage-a-rollout.md) moves one Tier's population onto a
  new Blueprint version in cohorts.
- [Activate a version](activate-a-version.md) moves the estate onto a new
  Catalogue or Schema Registry version.
- [Explore the demo](explore-the-demo.md) is the same console over a curated
  estate, with a build-time snapshot in place of a server.
