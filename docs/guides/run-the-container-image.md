---
title: Run the container image
description: Pull the image, serve an estate from a container, place the secrets, run the CLI from the same artefact, and carry the whole thing across an air gap.
order: 7
---

# Run the container image

The image holds three things: the `telecraft` binary with the console inside
it, the Catalogue for the collector version it was built against, and the
licence. It runs the same command a host process runs, so everything in
[Run an Instance](run-an-instance.md) applies here, with a container's way of
passing configuration in.

It carries no shell, no package manager, and no interpreter, and the process
runs as user `65532`.

## Pull it

```sh
docker pull ghcr.io/telecraft-dev/telecraft:release
```

Two tags:

| Tag | Points at |
|---|---|
| `vMAJOR.MINOR.PATCH` | One release. It never moves. |
| `release` | The current stable release. It moves with each one. |

Pin the digest in a deployment. Every release names its own:

```sh
docker pull ghcr.io/telecraft-dev/telecraft@sha256:DIGEST
```

`DIGEST` is the value the release notes carry.

## Serve an estate

Mount an estate checkout read only, and publish the two addresses:

```sh
docker run --rm \
  --volume /srv/estate:/estate:ro \
  --publish 4321:4321 \
  --publish 4320:4320 \
  ghcr.io/telecraft-dev/telecraft:release \
  serve -estate /estate
```

```
console and API on http://0.0.0.0:4321
OpAMP on 0.0.0.0:4320
the session key was drawn at start, so sessions last as long as this process
```

Open <http://127.0.0.1:4321> and sign in.

| Port | What listens |
|---|---|
| 4321 | The console, the API, and the two probes. |
| 4320 | The OpAMP endpoint, at `/v1/opamp`. |

`serve` is the image's default command, so the arguments after the image name
are the flags you want to change. Binding both addresses on every interface
inside the container is the only configuration the image sets for you.

The estate needs at least one user in `users.yaml` before the Instance
starts. Hash a secret with the same image, where `SECRET` is the password you
are setting:

```sh
printf %s "$SECRET" | docker run --rm -i ghcr.io/telecraft-dev/telecraft:release passwd
```

### Keep the checkout current

The image carries no `git`, so `-repo` has nothing to fetch an estate with.
Serve a directory instead, and keep that directory current from outside the
container: a job on the host that pulls, or a container beside this one that
syncs the repository into a volume both of them mount. Each poll re-reads the
directory, so a merge arrives without a restart. On Kubernetes that second
container is what the chart installs, and
[Deploy on Kubernetes](deploy-on-kubernetes.md) sets it up.

On a host that has `git`, `-repo` works as
[Run an Instance](run-an-instance.md) describes.

## Configure it from the environment

Every flag has an environment variable: `TELECRAFT_` plus the flag name
upper-cased, with dashes as underscores. A flag beats an environment
variable, which beats the default.

```sh
docker run --rm \
  --volume /srv/estate:/estate:ro \
  --publish 4321:4321 \
  --env TELECRAFT_ESTATE=/estate \
  --env TELECRAFT_EXTERNAL_URL=https://telecraft.example \
  --env TELECRAFT_FETCH_INTERVAL=15s \
  ghcr.io/telecraft-dev/telecraft:release
```

TLS terminates in front of the container. Tell the process what the outside
sees with `TELECRAFT_EXTERNAL_URL`, and it refuses to start on a non-loopback
host over plain HTTP unless `-insecure-http` says you mean it.

## Place the secrets

Telecraft reads secret material from files in one directory. No secret
travels as an environment variable, and none belongs in an image layer.

```sh
docker run --rm \
  --volume /srv/estate:/estate:ro \
  --volume /srv/telecraft/secrets:/run/secrets:ro \
  --publish 4321:4321 \
  ghcr.io/telecraft-dev/telecraft:release \
  serve -estate /estate -secrets-dir /run/secrets
```

The files must be readable by user `65532`. A compose file's `secrets:` block
presents them at `/run/secrets` already, which is why that is the path in the
examples. Rotating one is rewriting its file.

## Run the CLI from the same image

The image is the whole CLI, so a pipeline with no Go toolchain runs the same
commands the Instance runs, from the same artefact:

```sh
docker run --rm --volume "$PWD:/estate:ro" \
  ghcr.io/telecraft-dev/telecraft:release \
  check -library /estate/requirements \
    -estate /estate/rows.yaml \
    -endpoint https://telemetry.example \
  > report.json
```

Every command in the [CLI reference](../reference/cli.md) works the same way:
name it after the image, mount what it reads, and redirect what it writes.
[Check conformance](check-conformance.md) covers this one's inputs and its
exit codes.

## Activate the Catalogue it carries

The image carries the Catalogue for its pinned collector version at
`/usr/share/telecraft/catalogues/`. It arrives in your estate the way a
Catalogue carried across an air gap does: copy it in, read the impact report,
then activate it.

```sh
id=$(docker create ghcr.io/telecraft-dev/telecraft:release)
docker cp "$id:/usr/share/telecraft/catalogues/." /srv/estate/catalogues/
docker rm "$id"
```

[Activate a version](activate-a-version.md) is the rest of it.

## Carry it across an air gap

The image is the only thing that has to travel. `VERSION` is the release tag,
such as `v0.8.0`:

```sh
docker save ghcr.io/telecraft-dev/telecraft:VERSION -o telecraft-VERSION.tar
```

Move the file, then load it where it is going:

```sh
docker load -i telecraft-VERSION.tar
```

Nothing else is fetched. The console is inside the binary and reaches no
other origin, and the Catalogue baseline is in the image. What the air gap
still supplies from inside is the secrets, the estate, and any Catalogue
newer than the one the image carries.

## What is where inside the image

| Path | Holds |
|---|---|
| `/usr/local/bin/telecraft` | The binary, and the image's entrypoint. |
| `/usr/share/telecraft/LICENSE` | The terms the software is licensed under. |
| `/usr/share/telecraft/catalogues/` | The Catalogue baseline. |

## Build the image yourself

From a checkout, two scripts do it. The first builds the console,
cross-compiles both Linux binaries, and stages everything the image copies.
The second builds the image from that staged directory:

```sh
tools/image/stage.sh
PLATFORMS=linux/amd64 LOAD=1 tools/image/build.sh
```

`tools/image/build.sh` stages first unless you set `STAGED=1`, and builds both
architectures unless you name one. `tools/image/offline.sh` then starts an
image with networking disabled and requires it to serve.

## What next

- [Deploy with Compose](deploy-with-compose.md) puts this image on one host
  behind a terminator, with the secrets and the estate around it.
- [Deploy on Kubernetes](deploy-on-kubernetes.md) installs this image from a
  chart, with the checkout kept current beside it.
- [Run an Instance](run-an-instance.md) is the same server as a host process,
  with the whole flag surface.
- [Activate a version](activate-a-version.md) moves the estate onto a new
  Catalogue or Schema Registry version.
