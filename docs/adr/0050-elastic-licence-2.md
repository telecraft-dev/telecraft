# ADR-0050: Elastic License 2.0, and saying "source-available" rather than "open source"

- Status: accepted
- Date: 2026-08-20

## Context

The repository has been public since 2026-08-12 and has never carried a
licence file. `README.md` asserted `Apache-2.0` in a three-word section
added the day after the first commit; GitHub reported no licence for the
repository, no `LICENSE` existed in any commit, there were no SPDX headers,
and `console/package.json` carried no `license` field. The assertion was
therefore a claim about a licence rather than a grant of one, and the two
sibling repositories — `estate-demo` and `telecraft.dev` — carried no
licence file either.

That is a decision that had not been made, sitting in a repository that
reads as though it had been.

Telecraft is intended to be a product as well as a codebase. The failure
mode a permissive licence carries for a platform of this shape is specific
and known: the differentiator here is the evaluator — deriving an
expectation from configuration and checking it — and that is precisely what
is cheapest for a third party to run as a managed service without
contributing anything back.

## Decision

1. **Elastic License 2.0, applied at the root of the repository.** The
   verbatim text lives in `LICENSE`. It grants use, copying, distribution
   and derivative works, and withholds three things: providing the software
   to third parties as a hosted or managed service, circumventing licence
   key functionality, and removing licensing notices. Telecraft has no
   licence key and no plan for one, so in practice the operative limitation
   is the first.

2. **An adopter running Telecraft for their own estate is unrestricted**,
   including in production and including commercially. This is the whole
   population the product is designed for, and the licence should not read
   as though it were aimed at them.

3. **The project describes itself as "source-available", never as "open
   source".** ELv2 is not OSI-approved, and the distinction is one the
   people this product is aimed at care about and will check. Two published
   sentences claimed open source (`README.md`, `docs/index.md`) and the
   marketing site claimed it twice more with an `Apache-2.0` colophon; all
   are corrected. A surface that calls Telecraft open source is now a
   defect, in the same way that a surface showing a signal colour without
   its lane name is (ADR-0047 §5).

4. **No relicensing of history, and the Apache line is not erased.** The
   README said `Apache-2.0` between 2026-08-12 and this ADR. Rewriting that
   out of the history would change every commit hash, invalidate the
   `v0.1.0-rc.1` tag and its published release, and misrepresent what the
   repository said during those days. A licence grant is not retracted by
   editing the record of it; it is retracted, if at all, by never having
   made it. What we have instead is an accurate present licence and a
   history that shows when it arrived.

5. **Third-party components keep their own licences**, which are more
   permissive and unaffected: the two bundled typefaces under the SIL Open
   Font License, whose texts travel with the faces because the licence
   requires it, and the Lucide utility icons under ISC (ADR-0047 §6).

## Consequences

- ELv2 does not conflict with ADR-0001. That ADR governs vendor words in
  code and normative documentation — seam names, implementation names, the
  neutral core — and a licence is neither. The `docs` scope of
  `vendorlint.yaml` carries no rule matching the licence's name, and
  `LICENSE` at the root sits outside every scope, so the lint is unaffected.
  It is worth saying plainly that the licence's author is also the vendor
  behind the first `TelemetryProvider` implementation, and that this changes
  nothing about the core's neutrality: ADR-0001 constrains what the software
  does, not whose licence text it ships under.
- Contributions now arrive under ELv2. There is no contributor licence
  agreement and no contributor other than the author, so nothing needed
  anyone's consent; a second contributor makes a CLA a question worth
  asking, and it is not one yet.
- The sibling repositories still carry no licence. `estate-demo` is
  authored YAML rather than product code and `telecraft.dev` is a static
  page, but "no licence" means "no rights granted", which is not what either
  is for. Both want a decision of their own rather than inheriting this one
  silently.
- A future move to an OSI licence stays open: relicensing towards more
  permissive terms needs only the copyright holder's agreement, and today
  there is exactly one.

## Sources

- ADR-0001, ADR-0047, ADR-0049.
- `LICENSE`, `vendorlint.yaml`, `README.md`, `docs/index.md`.
- Repository state as read on 2026-08-20: no licence file in any of 232
  commits, GitHub reporting none declared, 0 forks and 0 stars.
