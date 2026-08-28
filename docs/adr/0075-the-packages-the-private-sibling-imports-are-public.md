# ADR-0075: The packages the private sibling imports leave `internal/` for `pkg/`

- Status: accepted (amends ADR-0072 §11)
- Date: 2026-08-28

## Context

ADR-0072 §11 put the hosted service in `telecraft-dev/hosted`, private, and
fixed the direction of the dependency: that repository depends on this one,
nothing here reaches it, and the binary's module graph never names it.

What that decision did not settle is how the dependency is expressed. Every
package the hosted service reads is under `internal/`, which the Go tool
reaches only from inside this module. So the invariant as written was not
buildable: the hosted half compiled while it sat in this repository and
stopped compiling the moment it left.

Two ways out, and only one of them survives being said out loud. The private
repository can carry copies of the packages it needs, or this repository can
make them importable. ADR-0069 §7 already refused the copy for the
Provisioner, and the argument is the same one level up: a copy is a contract
that cannot watch its original change, so it fails silently, late, and in the
repository nobody is reviewing.

## Decision

### 1. Five packages move to `pkg/`

`register`, `seed`, `forge`, `ownership` and `auth` move out of `internal/`
and into `pkg/`. They keep their names, their contents and their tests. The
import path is the only thing that changes, and it changes everywhere in one
commit.

These five are what the hosted half imports and nothing more. `forge` is the
seam and not the adapter: `internal/provider/forge` stays where it is,
because the hosted code uses the vocabulary and never a particular forge.

### 2. `pkg/` is neutral core, on the same terms as `internal/`

`vendorlint.yaml` already names `pkg/**` in the `core` scope, so the
ADR-0001 vendor-word rule follows these packages across the move with
nothing to change and no boundary to redraw. That is why `pkg/` is the
destination rather than the module root: the rule that keeps vendor words
out of the core is not weakened by making a package public.

### 3. Public means a consumer, not an invitation

`pkg/` holds the packages something outside this repository imports. It is
not a staging area for code that might be useful to somebody, and a package
does not move there because it reads well. The test is whether a named
consumer depends on it, and today there is exactly one.

Nothing is promised about compatibility beyond what the project promises
about everything else it publishes, which the major version being zero
already says.

## Consequences

- The hosted half builds outside this repository, which is what ADR-0072 §11
  asserted and could not have been true before this.
- The vendor-word lint, the formatting check and the whole test suite cover
  these packages exactly as they did, because the scope globs and the tests
  moved with them.
- Accepted decisions that cite `internal/auth/oidc.go` and its neighbours now
  cite paths that have moved. They are the record of what was decided when it
  was decided, and they are not edited to follow a rename.
- A second consumer, if one arrives, meets a public surface that was designed
  for one. Widening it is a decision rather than a convenience.

## Sources

- ADR-0001 (the neutral core and the vendor-word lint), ADR-0069 §7 (why a
  copy is refused), ADR-0072 §11 (the private sibling and the one-way
  dependency), ADR-0073 §4 and §5 (what the hosted half does with the forge
  seam and the register).
- `vendorlint.yaml` (the `core` scope, which already names `pkg/**`),
  `cmd/telecraft/boundary_test.go` (the two tests that hold the direction).
