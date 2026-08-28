---
title: Deploy on Kubernetes
description: Install the Telecraft chart, keep the estate checkout current beside the server, place secrets as files, put TLS in front, and install with no registry at all.
order: 9
---

# Deploy on Kubernetes

The chart deploys one Instance: one Deployment of one process serving the
console, the platform API and the OpAMP endpoint over one estate. It deploys
nothing else. No collector, no Supervisor, no DaemonSet, and nothing that
renders your configuration.

It installs the same image [Run the container
image](run-the-container-image.md) describes, running the same command [Run an
Instance](run-an-instance.md) documents, so the flags on this page are ones you
have already met.

## Before you start

You need:

- A Kubernetes cluster, 1.23 or later, and Helm 3.
- An estate repository the cluster can reach, and a credential for it if it
  is private.
- A container image with `git` and a POSIX shell in it, from a registry your
  cluster pulls from. The chart names none, so you name the one you already
  hold.
- Somewhere for TLS to terminate. An ingress controller is the usual answer.

## Install

```sh
helm install telecraft oci://ghcr.io/telecraft-dev/charts/telecraft \
  --version 0.7.0 \
  --namespace telecraft --create-namespace \
  --set estate.sync.repo=https://forge.example/acme/estate.git \
  --set estate.sync.image.repository=registry.example/git \
  --set estate.sync.image.tag=2.45 \
  --set ingress.enabled=true \
  --set ingress.host=telecraft.example \
  --set ingress.className=nginx
```

Watch it come up:

```sh
kubectl rollout status deployment/telecraft -n telecraft
```

The server is ready once it holds a snapshot of the estate. Open
<https://telecraft.example> and sign in with a user from the estate's
`users.yaml`.

For anything past a first look, write the values down in a file and install
from that instead. Nothing in the chart takes a secret value, so the file is
safe to commit:

```sh
helm upgrade --install telecraft oci://ghcr.io/telecraft-dev/charts/telecraft \
  --version 0.7.0 --namespace telecraft --values telecraft.yaml
```

## The estate arrives in a volume

The image carries no `git`, so the checkout the server reads lives in a volume
beside it and something else keeps that volume current. There are two shapes,
and you pick one.

### A sidecar clones and pulls

This is what the chart does unless you turn it off. An init container clones
the estate into a shared volume before the server starts, and a sidecar pulls
it on an interval. Both run the image you named:

```yaml
estate:
  sync:
    enabled: true
    image:
      repository: registry.example/git
      tag: "2.45"
    repo: https://forge.example/acme/estate.git
    ref: main
    intervalSeconds: 30
```

The server re-reads the directory on its own poll, so a commit reaches the
console about a poll after the sidecar pulls it, with no restart. Read what
the sidecar is doing:

```sh
kubectl logs deployment/telecraft -n telecraft -c estate-sync
```

For a private repository, create a Secret and name it. The chart projects it
read-only for the estate containers alone and never for the server:

```sh
kubectl create secret generic estate-credential -n telecraft \
  --from-literal=password=YOUR_FORGE_TOKEN
```

```yaml
estate:
  sync:
    credentialSecret: estate-credential
```

Over HTTPS the Secret holds `password`, and `username` where your forge wants
one other than `x-access-token`. Over SSH it holds `ssh-privatekey`, and
`known_hosts` where you pin the host key. Rewriting either file is the whole
of rotating it: the next fetch picks up what is on disk.

To fetch some other way, replace the two commands. What you set runs in the
init container and the sidecar alike, so the sidecar's is the one that has to
loop:

```yaml
estate:
  sync:
    command: [/bin/sh, -c]
    args: ["..."]
```

### You supply the volume

Turn the sidecar off and name a volume that already holds a checkout.
Whatever fills it keeps it current:

```yaml
estate:
  sync:
    enabled: false
  volume:
    persistentVolumeClaim:
      claimName: estate-checkout
```

This is the shape for an estate that reaches the cluster some way that is not
a clone: a replicated volume, a job that unpacks a bundle, an operator of
your own.

## Secrets are files you place

Telecraft reads every secret it needs as a file in one directory. Your estate
names a secret; you create a Secret holding a key of that name; the chart
projects it read-only and tells the server where to look.

Two of those names are the server's own. `session-key` signs sessions, and
without it a restart signs everybody out. `telemetry-key` is the credential
for the telemetry backend, where you set one.

```sh
kubectl create secret generic telecraft-secrets -n telecraft \
  --from-file=session-key=./session-key \
  --from-literal=staff-oidc=YOUR_CLIENT_SECRET
```

```yaml
server:
  secrets:
    secretName: telecraft-secrets
```

Every key in the Secret becomes a file of that name, and `auth.yaml` in your
estate resolves against those names. Nothing in your values file holds a
value. An external secret operator that writes the Secret needs no support
from the chart: filling the directory is the whole interface.

## The addresses, and what terminates TLS

The process holds no certificate. Both endpoints speak plain HTTP inside the
pod, and TLS terminates at the ingress or at whatever else you put in front.

Two hosts, because humans and collectors arrive on separate ports and you can
expose one and not the other:

| Value | What it routes |
|---|---|
| `ingress.host` | The console, the platform API and sign-in. |
| `ingress.opamp.host` | Collectors, at `/v1/opamp`. Off unless you turn it on. |

The chart sets the URL the outside sees from `ingress.host`, so the address
that redirects a sign-in and the address the ingress answers on cannot drift
apart. Where something other than this ingress sits in front, name that
address yourself:

```yaml
server:
  externalUrl: https://telecraft.example
```

The server refuses to start on an external URL that names a network host over
plain HTTP, and the chart refuses to render one, in the same words. Set
`server.insecureHttp` where you mean it.

### Collectors

Nothing in Telecraft authenticates a collector. It matches the identifying
attributes a collector reports and serves what they match, so an OpAMP
endpoint anyone can reach serves rendered configuration to anyone who guesses
a hostname and a plausible set of attributes. Decide what may reach it before
you route it:

```yaml
server:
  opamp:
    enabled: true
ingress:
  opamp:
    enabled: true
    host: opamp.telecraft.example
    tls:
      secretName: opamp-telecraft-tls
```

An Instance whose collectors are all Foreign needs neither. Set
`server.opamp.enabled` to `false` and the address closes rather than sitting
behind a firewall rule somebody has to remember.

## A licence, where you have one

No licence runs Standard Edition, which is the whole free product and warns
about nothing. Where you hold an Enterprise Edition licence, put it in a
ConfigMap or a Secret and name it:

```sh
kubectl create configmap telecraft-licence -n telecraft \
  --from-file=licence=./acme.licence
```

```yaml
server:
  licence:
    configMapName: telecraft-licence
```

## One replica, and what an upgrade costs

The chart installs one replica and refuses more. One Instance is one Instance
server: the reading of served collectors counts the connections one process
holds, so a second replica behind the same address would report the half it
holds as though it were the whole estate. For a second Instance, install the
chart again over its own estate.

The update strategy is Recreate for the same reason, so an upgrade is a short
outage of the console and the OpAMP endpoint. It is not an outage of
telemetry: collectors go on running the configuration they already hold, and
they fetch again when the endpoint answers.

Upgrade by moving the version and the image together:

```sh
helm upgrade telecraft oci://ghcr.io/telecraft-dev/charts/telecraft \
  --version 0.8.0 --namespace telecraft --reuse-values
```

The chart's own version carries the image tag with it, so one number answers
what you are running. Pin the bytes where you would rather not resolve a tag:

```yaml
image:
  digest: sha256:...
```

## Install with no registry to reach

The image is one of two artefacts that have to travel, and [Run the container
image](run-the-container-image.md) covers moving it on its own. Both are OCI,
so whatever you already replicate registries with moves both:

```sh
# Outside.
helm pull oci://ghcr.io/telecraft-dev/charts/telecraft --version 0.7.0
skopeo copy docker://ghcr.io/telecraft-dev/telecraft:v0.7.0 \
  docker-archive:telecraft-v0.7.0.tar
```

Push the image into your own registry, take the chart tarball in, and install
from the file:

```sh
helm install telecraft ./telecraft-0.7.0.tgz \
  --namespace telecraft --create-namespace \
  --set image.repository=mirror.internal.example/telecraft-dev/telecraft \
  --set image.tag=v0.7.0 \
  --values telecraft.yaml
```

Nothing else is fetched. The chart has no dependencies, so the install
resolves no second chart repository; no init container and no hook downloads
anything; and the console is inside the binary and asks no other origin for
an asset. What the pod pulls is the image reference you set, and that is the
only reference in what the chart renders.

Read the rendered manifests before you install if you would rather see that
than take it:

```sh
helm template telecraft ./telecraft-0.7.0.tgz --values telecraft.yaml \
  | grep -E 'image:|https?://'
```

Three things still have to reach the network from inside: the secrets, the
estate, and any Catalogue newer than the one the image carries.

## What the chart refuses

These fail at `helm install`, before anything reaches the cluster, and each
message names the value to change:

| It refuses | Because |
|---|---|
| More than one replica | One Instance runs one server. |
| No external URL | The address decides how sessions and sign-in redirects work, and the server does not guess it. |
| Plain HTTP over a network host | Passwords would cross it in clear text. Set `server.insecureHttp` where you mean it. |
| An ingress with no host | There is nothing to route. |
| Routing the OpAMP endpoint while the server has it closed | The route would reach nothing. |
| The sidecar on with no repository or no image | There is nothing to clone, or nothing to clone it with. |
| A licence in both a Secret and a ConfigMap | Name one. |

## What next

- [Run the container image](run-the-container-image.md) is the same image on a
  single host, and it lists what is where inside it.
- [Deploy with Compose](deploy-with-compose.md) is that host with the estate,
  the secrets and a terminator around it.
- [Place a licence](licensing.md) says what an Instance does in each state a
  licence can be in.
- [Serve configurations](serve-configs.md) installs a collector against the
  OpAMP endpoint this chart exposes.
- [Stage a Rollout](stage-a-rollout.md) moves one Tier's population onto a
  new Blueprint version in cohorts.
