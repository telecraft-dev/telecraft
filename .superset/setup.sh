#!/usr/bin/env bash
# What a fresh Superset workspace needs before anyone can build or run.
#
# Two things, and deliberately only two. `console/node_modules` is
# gitignored, so a new worktree has none and every console command fails
# until it does. The Go module cache is shared across worktrees, so
# `go mod download` is usually a no-op that only pays for itself the first
# time or after a dependency change.
#
# Not here, on purpose:
#
#   Playwright browsers   `npx playwright install chromium` writes to a
#                         shared ~/.cache and takes far longer than the
#                         rest of setup put together. Only `npm run e2e`
#                         needs it, so it stays a one-off on the host.
#   devenv                `devenv/devenv up` pins the compose project name
#                         to telecraft-devenv and binds 127.0.0.1:9200, so
#                         one workspace starting it takes those from every
#                         other workspace. Start it by hand, in one place.
#   Env files             Nothing secret is gitignored here.
#                         console/.env.demo is committed.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/.." && pwd)"
cd "$root"

# shellcheck source=./node.sh
. "$here/node.sh"
use_node

printf '\n>> console dependencies\n'
# `npm ci` rather than `npm install`: it installs exactly the lockfile and
# is the same command CI runs, so a workspace cannot quietly drift.
(cd console && npm ci --no-audit --no-fund)

printf '\n>> Go modules\n'
go mod download

printf '\nWorkspace ready. Node %s, Go %s.\n' "$(node -v)" "$(go version | awk '{print $3}')"
