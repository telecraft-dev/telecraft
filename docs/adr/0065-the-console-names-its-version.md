# ADR-0065: The console names its version

- Status: accepted (decides the question ADR-0049's consequences left open)
- Date: 2026-08-28

## Context

ADR-0049 made releases tags on `main` and pointed the public demo at a
moving `release` tag, and its consequences recorded the gap that left:
nothing on the demo says which release it is built from. The demo's chrome
carries the estate's provenance (the snapshot's repository and commit),
which is the provenance of what is shown, not of the software showing it.
The ADR called closing the gap worth doing and declined to decide it.

The question is not demo-shaped. Any console build has a provenance: a
release the operator deployed, a commit a contributor is running, or a
working tree with no version information at all. A reader filing a report,
checking whether a fix has reached them, or comparing what they see against
the documentation needs the same answer on an instance as on the demo, so
the answer belongs to the console, not to the demo's wrapper.

Three prior decisions shape where the answer can live. ADR-0058 §3 divides
the chrome: it keeps identity, navigation, search, the Tour control, and
the profile control, things about getting around and about the reader,
while readings about the estate live in the context strip. ADR-0042's
surface inventory gives every Workspace to the estate's objects, none of
them a home for a fact about the software. And ADR-0045 §5's zero-CDN rule
is enforced over the built bundle by a check whose JavaScript allowlist
admits only string literals the bundle never fetches, each entry a
reviewable event.

## Decision

1. **The version appears in the profile section of the chrome.** It is a
   quiet entry at the bottom of the profile panel, in the mono face, on
   every build: instance and demo alike. The version is a fact about the
   reader's session (which software is serving it), and the chrome's
   profile corner is where facts about the reader already live under
   ADR-0058 §3's division. It joins no Workspace and no context strip:
   the strip reads the estate, and the version is not a reading of the
   estate. The entry is the version string alone, with no label and no
   explanatory copy.

2. **The value is injected at build time, and resolves in a fixed
   order.** A `TELECRAFT_VERSION` environment variable set at build time
   wins, so a packaging step can state the version exactly. Otherwise the
   build captures `git describe --tags --always --dirty`, tolerating
   failure. When neither yields anything, the value is the literal
   `development`. There is no runtime endpoint: the bundle is the thing
   whose provenance is being named, so the bundle carries the answer, and
   a backend's opinion of its own version would be a different fact.

3. **The entry links to where the build came from.** An exact
   `vMAJOR.MINOR.PATCH` tag, pre-release suffixes included, opens the
   release page at
   `https://github.com/telecraft-dev/telecraft/releases/tag/VERSION`,
   because under ADR-0049 the tag is the release. A describe string that
   carries a commit sha, including a bare sha from a tagless checkout,
   opens that commit; a string that proves neither, including an exact
   tag carrying the `-dirty` marker, opens the repository root. The link
   opens in a new tab with `rel="noreferrer"`. `development` is a word,
   not a link: a build with no version information has nowhere honest to
   lead. The link is a navigation the reader chooses, never a resource
   the bundle loads, so it enters the zero-CDN check's never-fetched
   allowlist rather than bending ADR-0045 §5's rule about runtime
   dependencies.

4. **The version is provenance, never a health signal.** It takes the
   muted ink and the mono face, and no severity styling ever rides it: an
   old version is not a warning, a pre-release is not a caution, and
   `development` is not an error. Any surface that wants to judge a
   version against something is a different decision.

## Consequences

- The demo's gap closes from inside the console: once `estate-demo`'s
  build environment lets `git describe` see the version tags, the demo
  names its release with no demo-specific code. Until then the demo shows
  what its build can prove, which may be a commit or `development`, and
  that is the degradation working as designed, not a defect to paper over.
- The zero-CDN check's never-fetched allowlist grows by one entry, the
  repository address. The demo provenance in the same panel remains text,
  not a link; whether it should follow is not decided here.
- A dirty or tagless local build shows a describe string or
  `development` rather than a release, so what a contributor sees in the
  panel is the truth about their tree, which is the point.
- The value is fixed at build time. A bundle served long after it was
  built names the version it was built from, which is the question being
  answered; nothing re-checks for newer releases, and nothing should.
- No new capitalised domain terms, so the glossary is unchanged.

## Sources

- ADR-0042; ADR-0049, whose consequences posed the question; ADR-0045 §5;
  ADR-0058 §3; design conversation of 2026-08-28.
