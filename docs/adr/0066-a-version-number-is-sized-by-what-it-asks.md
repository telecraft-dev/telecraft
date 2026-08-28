# ADR-0066: A version number is sized by what it asks of a consumer

- Status: accepted (amends ADR-0049 §1's sizing table)
- Date: 2026-08-28

## Context

ADR-0049 §1 sized the minor version, while the major is zero, as "anything
a consumer can notice". Lived with from v0.5.1 to v0.7.0, that test failed
in one direction: on a product, nearly everything is noticeable, so nearly
every release was a minor candidate and the number stopped discriminating.
The record shows the rule losing to judgement each time it was applied.
v0.5.1 shipped a visible chrome change as a patch because a minor "did not
feel big enough". v0.5.6 shipped an accepted presentation decision
(ADR-0059) as a patch. v0.7.0 and the version entry that followed it both
sized minor for work no consumer had to act on. A rule that every release
re-litigates by feel is the absence of a rule, and visibility is the wrong
axis: every release is visible, few have consequences.

A consumer here is anyone who takes this repository at a ref, which
ADR-0049 already enumerates: `estate-demo` building the demo, the
documentation and marketing sites consuming the design artefact, and an
operator building the CLI and authoring an estate against the documented
formats. A reader of the demo is an audience, not a consumer: what they
see may move on any release.

## Decision

1. **Two questions, asked in order, first yes wins.** While the major
   version is zero:

   - **Must a consumer change anything to take this release?** Migrating
     an authored file, changing a CLI invocation, re-pinning a renamed
     token, re-rendering a committed estate, adjusting to a changed
     platform API document. Yes: **minor**, and the release notes lead
     with what must change and how.
   - **Can a consumer do something they could not do before?** A new CLI
     command or flag, a new platform API document, a new field in an
     authored format, a new Workspace or view, a new release artefact.
     Yes: **minor**.
   - Neither: **patch**, however visible. Fixes, redesigns, copy,
     density, performance and polish all live here.

2. **The release notes carry the answers.** Every release's notes open
   with `Breaking:` and `New:`, each possibly `none`, so a number can be
   audited against its notes and a wrong number is a reviewable fact
   rather than a difference of feel.

3. **Existing tags stand.** ADR-0049's rule that a tag never moves is
   untouched; nothing is renumbered. v0.7.0 would have been a patch under
   this rule and remains v0.7.0.

4. **Everything else in ADR-0049 §1 holds**: the shape of the tag, the
   pre-release suffix, and the meaning of `v1.0.0` as a state rather than
   a date.

## Consequences

- Minors become rare and load-bearing: a consumer who sees the minor move
  knows to read the notes before re-pinning, and a patch is safe to take
  on sight. This matches how the wider ecosystem reads a `0.y.z` number,
  so the signal now travels without a covering letter.
- The cost is that a large visible body of work can ship as a patch and
  the number will not advertise it. The demo and the release notes do
  that work; the number was never good at it.
- The version entry in the profile section (ADR-0065) ships as a patch.
- `docs/contributing/releases.md` replaces its sizing table with the two
  questions, and the repository's agent context follows it.
- No new capitalised domain term; the glossary is untouched.

## Sources

- ADR-0049 §1; ADR-0059; ADR-0065.
- The release history v0.5.1, v0.5.6, v0.6.0, v0.7.0, read against the
  rule that produced it.
- Sizing conversation of 2026-08-28.
