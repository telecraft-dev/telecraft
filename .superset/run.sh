#!/usr/bin/env bash
# The console dev loop: the fixture backend on 4700, Vite on 5173.
#
# Both are needed. Vite serves the console but proxies /api to
# 127.0.0.1:4700 (console/vite.config.ts), and the fixture backend is what
# answers there: a real server over the documented platform API, backed by
# console/fixtures/estate.json. Sign in with the credentials it prints at
# start-up.
#
# On the port guard below: Superset does not hand each workspace a port
# range, it discovers whatever is listening. The proxy target in
# vite.config.ts is a literal, so if 4700 is already held by another
# workspace's backend, Vite here would move itself to 5174 and then proxy
# to that other workspace's fixture estate. Nothing would look broken and
# every answer would come from the wrong place. Refusing to start is the
# only honest outcome, so run the loop in one workspace at a time.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$here/.." && pwd)"
cd "$root/console"

# shellcheck source=./node.sh
. "$here/node.sh"
use_node

if lsof -nP -iTCP:4700 -sTCP:LISTEN >/dev/null 2>&1; then
  cat >&2 <<'EOF'
Port 4700 is already listening, so the fixture backend cannot start here.

Vite proxies /api to 127.0.0.1:4700 as a literal, so carrying on would
serve this workspace's console against whoever already owns that port.
Stop the other dev loop first: Superset's ports panel lists it by
workspace, and will jump you to the terminal that owns it.
EOF
  exit 1
fi

[ -d node_modules ] || { echo "console/node_modules is missing; run .superset/setup.sh" >&2; exit 1; }

# Job control, so each child below leads its own process group and can be
# signalled as a group. Without it the cleanup only reaches the direct
# child: `npm run dev` is a wrapper around Vite, so killing npm leaves Vite
# holding 5173, and the next Run would then trip this script's own guard.
set -m

# Both children in the background rather than one in the foreground, so
# that bash is free to service a signal. A foreground `npm run dev` defers
# every trap until it exits, which is exactly never for a dev server, so
# the Run button's stop would orphan both processes.
node tools/fixture-backend.mjs --port 4700 &
backend=$!
# --host pins Vite to 127.0.0.1. Its default `localhost` resolves to ::1
# first on macOS, which leaves the server reachable by name but not at the
# address the rest of this repository uses (the fixture backend, the
# Playwright baseURL and the devenv all speak 127.0.0.1).
npm run dev -- --host 127.0.0.1 &
vite=$!

cleanup() {
  trap - EXIT INT TERM
  kill -TERM -"$backend" -"$vite" 2>/dev/null || true
  wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM

printf '\n>> fixture backend on http://127.0.0.1:4700\n>> console on http://127.0.0.1:5173\n\n'

# Return as soon as either half dies: a dev loop with only one of them is
# not a dev loop, and leaving the survivor running would hold its port. A
# poll rather than `wait -n`, because macOS still ships bash 3.2 as
# /bin/bash and that is what a plain `bash` finds there.
while kill -0 "$backend" 2>/dev/null && kill -0 "$vite" 2>/dev/null; do
  sleep 1
done
