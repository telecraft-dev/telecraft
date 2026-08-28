#!/usr/bin/env bash
# Renders the chart and holds it to the manifests in
# charts/telecraft/testdata/golden/, then requires the combinations the
# decisions refuse to fail rather than install.
#
# Two halves, and the second is the one that would rot quietly:
#
#   The goldens. Each values file under charts/telecraft/testdata is
#   rendered and compared with the manifest set beside it, so a change to a
#   template shows up in review as the change it makes to what an operator
#   installs rather than as a diff in a template nobody renders. The three
#   files are the three shapes the guide documents: the smallest install
#   that runs, the whole surface with TLS terminated in front, and the
#   air-gapped shape with every image re-pointed at a mirror and the
#   checkout supplied from outside the pod.
#
#   The refusals. A chart that quietly installed two replicas, or an
#   external URL that puts a password on a network in clear text, would put
#   the failure in a pod's log hours later. Each case here has to fail, and
#   the message has to say the thing the operator needs to read.
#
# This is a shell script rather than a Go test because the platform runs no
# toolchain from Go and Helm is one, which
# `internal/schemaregistry.TestNoToolchainBinaryIsInvoked` holds the whole
# tree to. `go run ./tools/chartlint` is the half of the chart's checks that
# renders nothing, and it runs wherever `go test ./...` does.
#
# Environment:
#   UPDATE   set to rewrite the goldens from what the chart renders now
#
# Usage: tools/chart/golden.sh

set -euo pipefail

root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root"

command -v helm >/dev/null || { echo "golden.sh: helm is not installed" >&2; exit 2; }

chart=charts/telecraft
testdata=$chart/testdata

render() {
  helm template telecraft "$chart" --namespace telecraft "$@" | normalise
}

# Helm puts a blank line before its document separator in some versions and
# not in others, so a golden written from one of them fails on the other and
# the check pins a Helm patch release rather than the chart. This drops a
# blank line that sits immediately before a `---`, and touches nothing else:
# a blank line anywhere a manifest means one, inside a block scalar most of
# all, comes through as it was.
normalise() {
  awk '
    /^$/ { blank++; next }
    {
      if ($0 != "---") { while (blank-- > 0) print "" }
      blank = 0
      print
    }
    END { while (blank-- > 0) print "" }
  '
}

failures=0

echo "golden.sh: the rendered manifests"
for values in "$testdata"/*.yaml; do
  name=$(basename "$values" .yaml)
  golden="$testdata/golden/$name.yaml"
  rendered=$(render --values "$values")
  if [ -n "${UPDATE:-}" ]; then
    printf '%s\n' "$rendered" > "$golden"
    echo "  $name: written"
    continue
  fi
  if [ ! -f "$golden" ]; then
    echo "  $name: no golden. Regenerate with UPDATE=1 tools/chart/golden.sh" >&2
    failures=$((failures + 1))
    continue
  fi
  if ! diff -u <(normalise < "$golden") <(printf '%s\n' "$rendered"); then
    echo "  $name: the rendered manifests changed. Where the change is meant, regenerate with UPDATE=1 tools/chart/golden.sh" >&2
    failures=$((failures + 1))
    continue
  fi
  echo "  $name: unchanged"
done

if [ -n "${UPDATE:-}" ]; then
  echo "golden.sh: goldens written; the refusals are not checked in this mode"
  exit 0
fi

# Each line is a description, the values to set, and a phrase the refusal
# has to carry. The base is the smallest install that runs, so every case
# differs from a working install by the one thing it is about.
refuses() {
  local what=$1 wants=$2
  shift 2
  local out
  if out=$(render --values "$testdata/minimal.yaml" "$@" 2>&1); then
    echo "  $what: the chart rendered, and it refuses this" >&2
    failures=$((failures + 1))
    return
  fi
  case "$out" in
    *"$wants"*) echo "  $what: refused" ;;
    *)
      echo "  $what: the refusal does not say \"$wants\":" >&2
      printf '%s\n' "$out" >&2
      failures=$((failures + 1)) ;;
  esac
}

echo "golden.sh: what the chart refuses"
refuses "a second replica" "One Instance runs one server" \
  --set replicaCount=2
refuses "no external URL" "Set server.externalUrl" \
  --set server.externalUrl= --set server.insecureHttp=false
refuses "plain HTTP across a network" "clear text" \
  --set server.insecureHttp=false
refuses "a licence in two places" "not both" \
  --set server.licence.secretName=a --set server.licence.configMapName=b
refuses "an ingress with no host" "ingress.host" \
  --set ingress.enabled=true
refuses "routing an endpoint that is closed" "an endpoint the server is not listening on" \
  --set ingress.opamp.enabled=true --set ingress.opamp.host=opamp.example \
  --set server.opamp.enabled=false
refuses "the sidecar with no repository" "estate.sync.repo names the estate repository" \
  --set estate.sync.repo=
refuses "the sidecar with no image" "estate.sync.image.repository names the image" \
  --set estate.sync.image.repository=

if [ "$failures" -gt 0 ]; then
  echo "golden.sh: $failures finding(s)" >&2
  exit 1
fi
echo "golden.sh: clean"
