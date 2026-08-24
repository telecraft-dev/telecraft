#!/usr/bin/env bash
# Puts a Node 24 or newer on PATH, or fails saying why.
#
# Sourced by setup.sh and run.sh. Both need it: `console/package.json` sets
# `engines.node` to >=24 and CI installs 24, but Vite 7 is the one that
# actually refuses to start below its own floor, so a workspace on the
# machine's default Node fails at `npm ci` or at the first `vite` with an
# error that says nothing about versions.
#
# Every branch here asks a version manager for 24 rather than installing
# one. If none of them is present the machine is not set up to build the
# console, and that is a thing to fix once rather than something a
# workspace should paper over.

# Keep in step with `engines.node` in console/package.json.
readonly NODE_MAJOR=24

node_major() { command -v node >/dev/null 2>&1 && node -p 'process.versions.node.split(".")[0]' 2>/dev/null; }

use_node() {
  [ "$(node_major)" = "$NODE_MAJOR" ] && return 0

  if [ -s "${NVM_DIR:-$HOME/.nvm}/nvm.sh" ]; then
    # nvm is a shell function, so it only exists once its script is sourced.
    # shellcheck disable=SC1091
    . "${NVM_DIR:-$HOME/.nvm}/nvm.sh"
    nvm use "$NODE_MAJOR" >/dev/null 2>&1 && return 0
  fi
  if command -v fnm >/dev/null 2>&1; then
    eval "$(fnm env)" 2>/dev/null || true
    fnm use "$NODE_MAJOR" >/dev/null 2>&1 && return 0
  fi
  if command -v mise >/dev/null 2>&1; then
    eval "$(mise env -s bash 2>/dev/null)" && [ "$(node_major)" = "$NODE_MAJOR" ] && return 0
  fi
  if command -v volta >/dev/null 2>&1; then
    volta run --node "$NODE_MAJOR" node -v >/dev/null 2>&1 && return 0
  fi

  cat >&2 <<EOF
This repository's console needs Node $NODE_MAJOR (console/package.json,
engines.node). The Node on PATH is $(node -v 2>/dev/null || echo 'not installed'),
and no version manager here could produce $NODE_MAJOR.

Install it once, on the host rather than in this workspace:

  nvm install $NODE_MAJOR      # or: fnm install $NODE_MAJOR
                     # or: mise use -g node@$NODE_MAJOR
EOF
  return 1
}
