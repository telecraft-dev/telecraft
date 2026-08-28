#!/usr/bin/env bash
# Builds the container image from a staged context (ADR-0068 §2).
#
# The default builds both architectures and keeps neither, which is what a
# pull request wants: the build is the check, and nothing is published from
# a branch. LOAD=1 builds one architecture into the local daemon so you can
# run it; PUSH=1 pushes the index, which only the release job does.
#
# Environment:
#   IMAGE      the repository to tag (default: ghcr.io/telecraft-dev/telecraft)
#   VERSION    the tag and the version label (default: `git describe`)
#   PLATFORMS  comma-separated (default: linux/amd64,linux/arm64)
#   TAGS       extra tags, space separated, such as `release` on a stable tag
#   LOAD=1     load the result into the local daemon (one platform only)
#   PUSH=1     push the index
#   STAGED=1   the context is already staged, so skip tools/image/stage.sh
#
# Usage: tools/image/build.sh

set -euo pipefail

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root"

image=${IMAGE:-ghcr.io/telecraft-dev/telecraft}
version=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo development)}
platforms=${PLATFORMS:-linux/amd64,linux/arm64}
revision=$(git rev-parse HEAD 2>/dev/null || echo "")

if [ "${STAGED:-0}" != "1" ]; then
  VERSION="$version" tools/image/stage.sh
fi

args=(buildx build
  --file Dockerfile
  --platform "$platforms"
  --build-arg "VERSION=$version"
  --build-arg "REVISION=$revision"
  --tag "$image:$version")

for tag in ${TAGS:-}; do
  args+=(--tag "$image:$tag")
done

if [ "${PUSH:-0}" = "1" ]; then
  args+=(--push)
elif [ "${LOAD:-0}" = "1" ]; then
  case "$platforms" in
    *,*) echo "build: LOAD=1 takes one platform; set PLATFORMS" >&2; exit 2 ;;
  esac
  args+=(--load)
fi

args+=(.)

echo "build: docker ${args[*]}"
docker "${args[@]}"
