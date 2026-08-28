# ADR-0076: The licence package moves to `pkg/` for the issuing tool

- Status: accepted (amends ADR-0075 §1)
- Date: 2026-08-28

## Context

ADR-0070 §6 puts issuance in `telecraft-dev/licensing`, private, holding the
signing key and the tool that uses it. §2 keeps the format in this
repository: `Write` sits beside the reader in one package, because a format
with two implementations is a format that drifts, and the implementation that
would break is the reader compiled into every deployed binary.

Those two hold together only if the tool can import the package. It cannot.
`internal/licence` is reachable from inside this module and nowhere else, so
the tool's choices were a copy of the format or nothing, and a copy is what
§2 exists to refuse. This is the problem ADR-0075 met one week earlier with
the hosted half, arriving at a second door.

ADR-0075 §3 anticipated this: `pkg/` was designed for one consumer, and
widening it is a decision rather than a convenience. This is that decision,
and it is the same argument ADR-0069 §7 made about the Provisioner. A copy is
a contract that cannot watch its original change, so it fails silently, late,
and where nobody is reviewing. Here it would fail in the one place where
failing costs a paying adopter the thing they bought: a licence issued
against a format the binary no longer reads is unreadable, and the adopter
meets it as a file that will not work.

## Decision

### 1. `licence` moves to `pkg/licence`

The package keeps its name, its contents and its tests. The import path is
the only thing that changes, and it changes everywhere in one commit.

Nothing travels with it. The package imports the standard library and no
package of this repository's, which is what verification being a pure
function of the file, the shipped keys and the host clock already required,
and `TestNothingHereCanReachANetwork` moves with the package and keeps it so.

### 2. What crosses, and what never crosses back

The tool imports the format: the document, the writer, and the reader it
verifies its own output through, so a licence that leaves the tool has
already been read by the same code the binary runs.

The compiled-in key list stays here and is added to by a release. The signing
key stays there. ADR-0070 §6 is unchanged in both directions: the private
repository depends on this one, nothing here reaches it, and no private key
enters this repository, its CI, an image or a release artefact.

### 3. `pkg/` now has two consumers, on ADR-0075 §3's terms

Both are private siblings and both depend one way. The test for admission is
unchanged: a named consumer imports the package, or the package stays where
it is. Nothing is promised about compatibility beyond what the major version
being zero already says.

## Consequences

- The issuing tool builds outside this repository, which is what ADR-0070 §6
  asserted and could not have been true before this.
- The tool tracks a version of this module, so a change to the format reaches
  it as a version to move to rather than as a difference nobody is looking
  for.
- The vendor-word lint, the formatting check and the whole test suite cover
  the package exactly as they did, because `vendorlint.yaml` names `pkg/**`
  in the `core` scope and the tests moved with it.
- Accepted decisions citing `internal/licence` now cite a path that has
  moved. They are the record of what was decided when it was decided, and
  they are not edited to follow a rename.
- The empty verifying-key list was correct while no key existed and is a
  defect from the moment one does. `docs/contributing/releases.md` carries
  what a release checks, and the check itself is a test.

## Sources

- ADR-0070 §2 (the format, and the writer beside the reader) and §6
  (issuance in the private sibling, and the direction of the dependency),
  ADR-0075 §1 (the five packages that moved) and §3 (public means a
  consumer), ADR-0069 §7 (why a copy is refused), ADR-0001 (the neutral core
  and the vendor-word lint).
- `pkg/licence/write.go`, `pkg/licence/keys.go`, `vendorlint.yaml`,
  `docs/contributing/releases.md`.
- Issues #204 (offline verification) and #224 (the issuing tool and the first
  verifying key).
