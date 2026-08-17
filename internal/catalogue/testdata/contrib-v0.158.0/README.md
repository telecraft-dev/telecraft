# Snapshot of upstream inputs — not a component list

A partial snapshot of `opentelemetry-collector-contrib` at tag `v0.158.0`
(commit `821a9d9c2c1623c4a0ceba5d47b57c48879c3f84`), holding representative
upstream shapes the walker must handle: nested extensions, per-signal
stability divergence, `deprecated_type` aliases, a deprecation notice, a
`class: pkg` module, a module with no `metadata.yaml`, and an `internal/`
module the walker must skip.

Every `metadata.yaml` is a verbatim upstream copy. Each `go.mod` is trimmed
to its `module` and `go` lines — the only lines the walker reads — because
full dependency lists are noise here and some name vendors ADR-0001 bans
from this tree.

This is a fixture snapshot of upstream *inputs*, not hand-curation of the
component list (which REQ-010 prohibits): the list is whatever the walker
finds in a tree, and these files are upstream's, reproducible from the tag
above.
