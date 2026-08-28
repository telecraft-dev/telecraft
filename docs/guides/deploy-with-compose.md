---
title: Deploy with Compose
description: Run an Instance on one host from the published image, over an estate the host keeps current, with TLS terminated in front and sign-in bootstrapped by basic auth.
order: 8
---

# Deploy with Compose

This is the smallest real deployment: one host, one Instance, and a reverse
proxy in front of it. Everything the Instance reads is on the host, and
nothing it needs comes from a network you do not run.

The files are in `deploy/compose/`, in the source at the release you are
deploying:

| File | What it is |
|---|---|
| `compose.yaml` | The Instance server, and an example terminator under a profile. |
| `.env.example` | Every value the compose file reads. Copy it to `.env`. |
| `proxy/telecraft.conf` | The terminator's configuration: the console on 443, and the OpAMP endpoint with its WebSocket upgrade carried through. |

`devenv/compose.yaml` is a different file for a different job. It starts
collectors to develop against and it deploys nothing.

## Before you start

- A host with a container runtime and the Compose plugin.
- `git` on the host. The image carries none, so the host is what keeps the
  estate checkout current.
- Two images: the Telecraft image, and the terminator's. Both are pulled
  once, and both can come from a mirror of your own.
- An estate repository. [Author and render](author-and-render.md) is where
  one comes from; the steps below make a local one if you have no forge.

Copy the directory to wherever you keep deployment files, and work there:

```sh
mkdir -p /srv/telecraft
cp -r deploy/compose/. /srv/telecraft/
cd /srv/telecraft
cp .env.example .env
```

## Create the estate repository

Two shapes work. If your estate lives on a forge, clone it and skip to
[Give somebody a way in](#give-somebody-a-way-in):

```sh
git clone https://forge.example/acme/estate.git /srv/telecraft/estate
```

For a standalone or air-gapped Instance, the repository is a bare one beside
the checkout, on the same host:

```sh
git init --bare /srv/telecraft/estate.git
git -C /srv/telecraft/estate.git symbolic-ref HEAD refs/heads/main
git clone /srv/telecraft/estate.git /srv/telecraft/estate
```

Push an estate into it. A fresh bare repository is empty, and an Instance
serves what the renderer wrote, so the first push is the estate [author and
render](author-and-render.md) produced: `teams.yaml`, the team directories,
and the `rendered/` tree.

Authors clone `/srv/telecraft/estate.git` over SSH, push branches to it, and
merge in a checkout of their own. Nothing reaches a hosted service, and the
Instance reads the checkout rather than the bare repository.

Point `ESTATE_DIR` in `.env` at the checkout:

```sh
ESTATE_DIR=/srv/telecraft/estate
```

## Give somebody a way in

Sign-in starts with basic auth: one user the estate knows, hashed into a file
under review. Hash a secret with the same image the Instance runs, where
`SECRET` is the password you are setting:

```sh
printf %s "$SECRET" | docker run --rm -i ghcr.io/telecraft-dev/telecraft:release passwd
```

Put the hash in `users.yaml` at the root of your estate checkout, beside
`teams.yaml`:

```yaml
users:
  - email: jo@example.com
    name: Jo Author
    owner: gateway-owners
    password: pbkdf2-sha256$600000$...
```

`owner` names an Owner in the team tree, and what Jo may author follows from
it. Commit the file and push it, so the Instance serves a user the repository
records:

```sh
git -C /srv/telecraft/estate add users.yaml
git -C /srv/telecraft/estate commit -m "Add Jo to the estate"
git -C /srv/telecraft/estate push
```

## Place the session key

A session is a signed token. Draw a key so sessions survive a restart, and
make it readable by the user the image runs as, `65532`:

```sh
mkdir -p /srv/telecraft/secrets
head -c 32 /dev/urandom > /srv/telecraft/secrets/session-key
chown 65532 /srv/telecraft/secrets/session-key
chmod 0400 /srv/telecraft/secrets/session-key
```

Point `SECRETS_DIR` in `.env` at that directory. Every secret the Instance
reads is a file in it, named by whatever names it.

## Start it

```sh
docker compose up -d
```

The Instance answers on the loopback address:

```sh
curl -sS http://127.0.0.1:4321/readyz
```

```
ready
```

`/readyz` answers `starting` until the first snapshot is held, and `/healthz`
answers while the process runs. Read the start-up lines with `docker compose
logs telecraft`:

```
console and API on http://0.0.0.0:4321
OpAMP on 0.0.0.0:4320
```

Open <http://127.0.0.1:4321> and sign in with the email and the secret you
hashed. Collectors reach the OpAMP endpoint at `ws://HOST:4320/v1/opamp`,
where `HOST` is this host's address. [Serve
configurations](serve-configs.md) is what to put on the collector side.

## Keep the checkout current

The Instance reads the estate directory again on every poll, so a merge
arrives without a restart. What brings the merge down to the host is a `git
pull` outside the container:

```sh
git -C /srv/telecraft/estate pull --ff-only
```

Run it on a timer, as often as you want changes to arrive. The Instance
re-reads the directory every `TELECRAFT_FETCH_INTERVAL`, so the two together
are how long a merge takes to reach a collector.

## Put TLS in front

The Instance server holds no certificate. Both of its addresses speak plain
HTTP, and something in front terminates TLS. An external URL naming a host
that is not a loopback address, over plain HTTP, sends passwords and sessions
across a network in clear text, and the process refuses to start on one.

Put the certificate and its key where the proxy reads them:

```sh
mkdir -p /srv/telecraft/tls
cp fullchain.pem privkey.pem /srv/telecraft/tls/
```

Then change four values in `.env`, so the Instance publishes on loopback and
the proxy holds the public addresses:

```sh
TELECRAFT_EXTERNAL_URL=https://telecraft.example
TLS_DIR=/srv/telecraft/tls
CONSOLE_ADDRESS=127.0.0.1:4321
OPAMP_ADDRESS=127.0.0.1:4320
```

Start the profile that adds the terminator:

```sh
docker compose --profile tls up -d
```

The console is now at <https://telecraft.example>, and collectors connect to
`wss://telecraft.example/v1/opamp`. Certificates come from whatever already
issues them for this host: nothing in the deployment fetches one.

Any terminator you already run replaces this one. What the Instance needs
from it is the console on one address, and the OpAMP endpoint with its
WebSocket upgrade carried through.

## Point at an identity provider

Basic auth is the bootstrap. Declare the providers your Instance offers in
`auth.yaml`, at the root of the estate beside `users.yaml`:

```yaml
providers:
  - kind: oidc
    name: staff
    issuer: https://issuer.example
    client_id: telecraft
    secret: staff-oidc
  - kind: basic
```

No field takes a value. `secret` names the secret, and the deployment places
a file of that name beside the session key:

```sh
printf %s "$CLIENT_SECRET" > /srv/telecraft/secrets/staff-oidc
chown 65532 /srv/telecraft/secrets/staff-oidc
chmod 0400 /srv/telecraft/secrets/staff-oidc
```

Add an entry of that name to the `secrets:` block in `compose.yaml`, and list
it on the service:

```yaml
services:
  telecraft:
    secrets:
      - session-key
      - staff-oidc

secrets:
  staff-oidc:
    file: ${SECRETS_DIR}/staff-oidc
```

Commit `auth.yaml` and push it, the way you pushed `users.yaml`: who may sign
in is under review like everything else the Instance reads. Register
`https://telecraft.example/api/v1/auth/staff/callback` as the redirect URI
with your provider, replacing `staff` with the name you gave the entry. Then
`docker compose --profile tls up -d` again.

A file the estate names and the deployment never placed stops the start, and
the message names the file, the name, and the directory searched. Drop
`- kind: basic` when everybody signs in through the provider, and keep it
while you are still proving the round trip.

[Run an Instance](run-an-instance.md) has the same providers for a host
process, SAML included.

## Deploy without a network

Nothing in this deployment reaches past the host and the estate you point it
at. Two images have to travel, and after that nothing is fetched.

Save them where you have a network, where `VERSION` is the release tag,
such as `v0.8.0`:

```sh
docker save ghcr.io/telecraft-dev/telecraft:VERSION -o telecraft-VERSION.tar
docker save nginx:1.29-alpine -o proxy.tar
```

Load them where they are going:

```sh
docker load -i telecraft-VERSION.tar
docker load -i proxy.tar
```

If you push them into a registry of your own instead, `TELECRAFT_IMAGE` and
`PROXY_IMAGE` in `.env` are the two values that change.

The console is inside the binary and reaches no other origin, and the image
carries the Catalogue for the collector version it was built against. What
the air gap still supplies from inside is the estate, the secrets, the
certificate, and any Catalogue newer than the one the image carries.
[Run the container image](run-the-container-image.md) copies the Catalogue
out of the image, and [activate a version](activate-a-version.md) is the rest
of it.

## Move to a new release

Set `TELECRAFT_IMAGE` in `.env` to the release you are moving to, then run
the command you started with:

```sh
docker compose pull telecraft
docker compose up -d
```

Add `--profile tls` to both if that is how it runs.

The Instance holds nothing that outlives it: everybody is signed out for the
length of the restart, collectors keep running the configuration they already
hold, and the record is in the estate repository throughout. Every release
names a digest, and `TELECRAFT_IMAGE` takes one in place of a tag when you
want the bytes named rather than resolved:

```sh
TELECRAFT_IMAGE=ghcr.io/telecraft-dev/telecraft@sha256:DIGEST
```

`DIGEST` is the value the release notes carry.

## What next

- [Stage a Rollout](stage-a-rollout.md) moves one Tier's population onto a
  new Blueprint version in cohorts.
- [Place a licence](licensing.md) covers what an Enterprise Edition licence
  changes, and what happens without one.
- [Run the container image](run-the-container-image.md) is the same image
  without Compose, and the whole CLI from the same artefact.
- [Deploy on Kubernetes](deploy-on-kubernetes.md) is the same image on a
  cluster, where the pull that keeps the checkout current runs in a sidecar
  rather than on a timer.
