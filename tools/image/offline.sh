#!/usr/bin/env bash
# Runs a built image with networking disabled and requires it to serve
# (ADR-0068 §6): the air-gap rule checked rather than asserted.
#
# The container is started on no network at all, so an image that needed one
# fetch to become ready fails here, before anything is published, rather than
# in somebody's air gap. Nothing can reach a container on no network from
# outside it either, so the probes run in a second container sharing the
# first's network namespace. That container's image is pulled before the
# check starts, which is the harness fetching its own tool rather than the
# image under test fetching anything.
#
# The estate is a directory rather than a bare repository: the image carries
# no git, so `-repo` is a shape the host process and the development
# environment have and this image does not.
#
# Environment:
#   CHECKER_IMAGE  the image the probes run from (default: curlimages/curl)
#   ESTATE         an estate checkout to serve (default: the small estate the
#                  console's own tests are written against)
#
# Usage: tools/image/offline.sh <image reference>

set -euo pipefail

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root"

image=${1:-}
if [ -z "$image" ]; then
  echo "offline: name the image to check" >&2
  exit 2
fi
checker=${CHECKER_IMAGE:-curlimages/curl:latest}
source_estate=${ESTATE:-internal/console/testdata/estate}

work=$(mktemp -d)
container=""
cleanup() {
  [ -n "$container" ] && docker rm -f "$container" >/dev/null 2>&1
  rm -rf "$work"
  return 0
}
trap cleanup EXIT

# An Instance refuses to start with nobody able to sign in, so the estate
# gets one user, hashed by the image's own passwd command. The secret lives
# for the length of this check and signs nothing in.
estate="$work/estate"
cp -R "$source_estate" "$estate"
chmod -R a+rX "$estate"
hash=$(printf 'offline-check' | docker run --rm -i --network none "$image" passwd)
cat > "$estate/users.yaml" <<USERS
users:
  - email: operator@example.com
    name: The operator
    owner: gateway-owners
    password: $hash
USERS

docker pull --quiet "$checker" >/dev/null

# There is no network here and so nothing terminating TLS in front, and the
# image serves on 0.0.0.0, which the fail-closed guard of ADR-0067 §5 reads
# as a plain HTTP URL on a host something could sit between. Plain HTTP is
# what this check means, so it says so rather than being handed a loopback
# external URL that would leave the guard untested against the image's own
# default address.
echo "offline: starting $image with no network"
container=$(docker run -d --network none \
  --volume "$estate:/estate:ro" \
  "$image" serve -estate /estate -insecure-http)

probe() {
  docker run --rm --network "container:$container" "$checker" \
    --silent --show-error --max-time 5 "$@"
}

# A container that refused to start answers nothing, and waiting the full
# thirty seconds for that buries the one line that says why in a timeout.
# So each turn of the loop asks whether the process is still there, and the
# first turn that finds it gone reports with its log.
status=""
for _ in $(seq 1 30); do
  status=$(probe --output /dev/null --write-out '%{http_code}' http://127.0.0.1:4321/readyz || true)
  [ "$status" = "200" ] && break
  if [ "$(docker inspect --format '{{.State.Running}}' "$container" 2>/dev/null)" != "true" ]; then
    echo "offline: the container stopped before it answered /readyz" >&2
    docker logs "$container" >&2 || true
    exit 1
  fi
  sleep 1
done
if [ "$status" != "200" ]; then
  echo "offline: /readyz answered $status, so the image never became ready" >&2
  docker logs "$container" >&2 || true
  exit 1
fi
echo "offline: /readyz is green"

body=$(probe http://127.0.0.1:4321/ || true)
case "$body" in
  *"built without the console"*)
    echo "offline: the image holds a binary with no console in it" >&2
    exit 1 ;;
  *"<div id=\"root\""*) ;;
  *)
    echo "offline: the console route did not answer with the bundle's document" >&2
    exit 1 ;;
esac
echo "offline: the console loads"

opamp=$(probe --output /dev/null --write-out '%{http_code}' http://127.0.0.1:4320/v1/opamp || true)
if [ -z "$opamp" ] || [ "$opamp" = "000" ]; then
  echo "offline: the OpAMP endpoint answered nothing on 4320" >&2
  exit 1
fi
echo "offline: the OpAMP endpoint answers on 4320 ($opamp to a plain GET)"

echo "offline: $image serves with no network"
