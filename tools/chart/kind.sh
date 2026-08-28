#!/usr/bin/env bash
# Installs the chart on a kind cluster and requires what it installed to
# serve (ADR-0068 §5, ADR-0074 §5).
#
# `helm lint` and the golden templates check what the chart renders. Nothing
# in either runs the manifests, so what is left to catch is the half a
# renderer cannot see: an image the pod cannot pull, a probe on the wrong
# port, a volume two containers disagree about, a mount that leaves the
# server unable to read what the sidecar wrote, a security context the
# kubelet refuses. Those arrive as a pod that never becomes ready, and this
# is where that happens instead of in an adopter's cluster.
#
# Both estate shapes are installed, into namespaces of their own, because
# they are two different pods:
#
#   synced   the init container clones and the sidecar pulls, which is what
#            the chart does unless it is turned off.
#   mounted  the sidecar is off and the checkout arrives in a volume the
#            operator supplied.
#
# The source estate is a bare repository and a checkout on the host, both
# mounted into the kind node, so neither install reaches a forge and the
# check has nothing to be flaky about. `git` reaches the bare repository by
# path, which is git-the-tool over the air-gap floor (ADR-0032 §3).
#
# Environment:
#   IMAGE       the image reference to install (default: telecraft:ci, which
#               is what tools/image/build.sh loads)
#   GIT_IMAGE   the image the estate containers run (default: alpine/git)
#   CLUSTER     the kind cluster name (default: telecraft-chart)
#   ESTATE      the estate to serve (default: the small estate the console's
#               own tests are written against)
#   KEEP        set to keep the cluster after the check, to look at it
#
# Usage: tools/chart/kind.sh

set -euo pipefail

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root"

image=${IMAGE:-telecraft:ci}
git_image=${GIT_IMAGE:-alpine/git:latest}
cluster=${CLUSTER:-telecraft-chart}
source_estate=${ESTATE:-internal/console/testdata/estate}

for tool in kind kubectl helm docker; do
  command -v "$tool" >/dev/null || { echo "kind.sh: $tool is not installed" >&2; exit 2; }
done

work=$(mktemp -d)
forwards=()
cleanup() {
  for pid in "${forwards[@]:-}"; do
    [ -n "$pid" ] && kill "$pid" 2>/dev/null
  done
  if [ -z "${KEEP:-}" ]; then
    kind delete cluster --name "$cluster" >/dev/null 2>&1
  else
    echo "kind.sh: the cluster $cluster is still up"
  fi
  rm -rf "$work"
  return 0
}
trap cleanup EXIT

# The estate the two installs serve. An Instance refuses to start with
# nobody able to sign in, so it gets one user, hashed by the image's own
# passwd command. The secret lives for the length of this check.
estate="$work/estate/checkout"
mkdir -p "$work/estate"
cp -R "$source_estate" "$estate"
hash=$(printf 'chart-check' | docker run --rm -i "$image" passwd)
cat > "$estate/users.yaml" <<USERS
users:
  - email: operator@example.com
    name: The operator
    owner: gateway-owners
    password: $hash
USERS

# A bare repository beside the checkout, so the synced install has a remote
# to clone that is not a network.
git -C "$estate" init --quiet --initial-branch main
git -C "$estate" add -A
git -C "$estate" -c user.email=chart@check -c user.name="The check" \
  commit --quiet -m "the estate under check"
git clone --quiet --bare "$estate" "$work/estate/estate.git"
chmod -R a+rX "$work/estate"

cat > "$work/kind.yaml" <<CONFIG
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    extraMounts:
      - hostPath: $work/estate
        containerPath: /estate
CONFIG

echo "kind.sh: creating the cluster"
kind create cluster --name "$cluster" --config "$work/kind.yaml" --wait 120s

# Both images are loaded rather than pulled by the node: the image under
# check exists nowhere a registry could serve it from, and loading the git
# image too keeps the pods from reaching out mid-check.
echo "kind.sh: loading $image and $git_image"
docker pull --quiet "$git_image" >/dev/null
kind load docker-image --name "$cluster" "$image" "$git_image"

# The two installs. Each answers on a loopback external URL, which is what
# a port-forward makes true and what the fail-closed guard of ADR-0067 §5
# admits without being told to.
install() {
  local release=$1 port=$2
  shift 2
  helm install "$release" charts/telecraft \
    --namespace "$release" --create-namespace \
    --set image.repository="${image%%:*}" \
    --set image.tag="${image##*:}" \
    --set image.pullPolicy=Never \
    --set server.externalUrl="http://localhost:$port" \
    "$@" \
    --wait --timeout 180s
}

echo "kind.sh: installing the synced shape"
install synced 18321 \
  --set estate.sync.image.repository="${git_image%%:*}" \
  --set estate.sync.image.tag="${git_image##*:}" \
  --set estate.sync.image.pullPolicy=Never \
  --set estate.sync.repo=/source/estate.git \
  --set estate.sync.intervalSeconds=5 \
  --set estate.sync.extraVolumeMounts[0].name=source \
  --set estate.sync.extraVolumeMounts[0].mountPath=/source \
  --set extraVolumes[0].name=source \
  --set extraVolumes[0].hostPath.path=/estate \
  --set extraVolumes[0].hostPath.type=Directory

echo "kind.sh: installing the mounted shape"
install mounted 18322 \
  --set estate.sync.enabled=false \
  --set estate.volume.hostPath.path=/estate/checkout \
  --set estate.volume.hostPath.type=Directory

# --wait already required every pod ready, which is the readiness probe
# green, which is the server holding a snapshot. What is left is to reach
# the two addresses through the Service the chart created, because a Service
# whose ports named the wrong container port is a working pod nothing can
# reach.
check() {
  local release=$1 http=$2 opamp=$3

  kubectl port-forward --namespace "$release" "service/$release-telecraft" \
    "$http:80" "$opamp:4320" >/dev/null 2>&1 &
  forwards+=($!)
  for _ in $(seq 1 30); do
    curl --silent --output /dev/null --max-time 2 "http://127.0.0.1:$http/healthz" && break
    sleep 1
  done

  local status
  status=$(curl --silent --output /dev/null --write-out '%{http_code}' "http://127.0.0.1:$http/readyz" || true)
  if [ "$status" != "200" ]; then
    echo "kind.sh: $release answered $status on /readyz" >&2
    kubectl logs --namespace "$release" "deployment/$release-telecraft" --all-containers >&2 || true
    exit 1
  fi
  echo "kind.sh: $release is ready"

  case "$(curl --silent --max-time 5 "http://127.0.0.1:$http/" || true)" in
    *'<div id="root"'*) echo "kind.sh: $release serves the console" ;;
    *) echo "kind.sh: $release did not serve the console document" >&2; exit 1 ;;
  esac

  status=$(curl --silent --output /dev/null --write-out '%{http_code}' --max-time 5 "http://127.0.0.1:$opamp/v1/opamp" || true)
  if [ -z "$status" ] || [ "$status" = "000" ]; then
    echo "kind.sh: $release answered nothing on the OpAMP port" >&2
    exit 1
  fi
  echo "kind.sh: $release answers on the OpAMP port ($status to a plain GET)"
}

check synced 18321 18320
check mounted 18322 18319

# The sidecar's own job, which is the half a static install never exercises:
# a commit lands in the remote, the sidecar pulls it, and the server moves
# its head to it without a restart.
echo "kind.sh: a commit reaches the synced install"
before=$(kubectl get pods --namespace synced -o jsonpath='{.items[0].metadata.name}')
date > "$estate/NOTE.md"
git -C "$estate" add -A
git -C "$estate" -c user.email=chart@check -c user.name="The check" commit --quiet -m "a change"
git -C "$estate" push --quiet "$work/estate/estate.git" main
head=$(git -C "$estate" rev-parse HEAD)
for _ in $(seq 1 30); do
  if kubectl exec --namespace synced "$before" -c estate-sync -- \
       git -C /estate rev-parse HEAD 2>/dev/null | grep -q "$head"; then
    echo "kind.sh: the sidecar pulled $head"
    break
  fi
  sleep 2
done
if ! kubectl exec --namespace synced "$before" -c estate-sync -- \
     git -C /estate rev-parse HEAD 2>/dev/null | grep -q "$head"; then
  echo "kind.sh: the sidecar never reached $head" >&2
  kubectl logs --namespace synced "$before" -c estate-sync >&2 || true
  exit 1
fi
after=$(kubectl get pods --namespace synced -o jsonpath='{.items[0].metadata.name}')
if [ "$before" != "$after" ]; then
  echo "kind.sh: the pod restarted while the estate advanced" >&2
  exit 1
fi

# The refusal that matters most, against a live cluster rather than a
# renderer: there is no supported way to run two of these.
if helm upgrade synced charts/telecraft --namespace synced --reuse-values \
     --set replicaCount=2 >/dev/null 2>&1; then
  echo "kind.sh: the chart installed a second replica" >&2
  exit 1
fi
echo "kind.sh: the chart still refuses a second replica"

helm uninstall synced --namespace synced >/dev/null
helm uninstall mounted --namespace mounted >/dev/null
echo "kind.sh: the chart installs, serves and uninstalls"
