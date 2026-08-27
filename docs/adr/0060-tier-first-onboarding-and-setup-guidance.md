# ADR-0060: Tier-first onboarding, and setup guidance that stays documentation

- Status: accepted
- Date: 2026-08-27

## Context

Onboarding is a collector joining governance, and the console covers only
half of it. The Claim flow (ADR-0042 §6) is collector-first: the collector
already runs, ungoverned, and the flow brings it in. There is no
governance-first path: an operator who wants a new gateway authors a Tier
by hand in git, then works out from the documentation how to start
collectors that its selector will match.

The corpus already holds both ends of that missing path. ADR-0030 makes
`never_seen` a deliberately neutral state: a freshly authored Tier
awaiting its collectors is a normal Tuesday, visible and never red.
ADR-0010 fixes the serving mechanics and the install-guidance hard rules,
and notes that Supervisor packaging is mature on VMs and Windows while
Kubernetes has no upstream image and a sidecar is structurally
unsupported. ADR-0002 draws the platform's line: configurations, never
binaries; how a binary reaches a host is documented, never owned.

## Decision

1. **One Tier-authoring flow, two doors.** The console gains an "Add a
   Tier" flow: name and position, Environment, owning Team and Owner,
   Blueprint version (the picker doors into the Blueprints view,
   ADR-0061), a draft selector over identity attributes, an optional
   `min_expected`, a delivery path (Served or Foreign), and a substrate.
   The Claim flow's "draft a new Tier" branch becomes the other door into
   this same flow, pre-filled from the herd's shared attributes; the two
   flows never fork.
2. **The exit is a pull request.** Like the composer and the governance
   editor, the flow proposes through the forge adapter with render-in-PR
   (ADR-0028), attributed to the operator (ADR-0014). The console never
   writes live state.
3. **The `never_seen` card is the waiting room.** After the merge, the
   new Tier's card carries setup guidance until its selector first
   matches: what to run, where the artefact or endpoint is, and which
   identity attributes the collector must report so the selector matches
   it (the ordinary first-boot path, ADR-0030 §1). No new pending state
   is invented; the guidance simply makes the neutral state useful.
4. **Setup guidance is documentation, never an artefact.** It is
   generated on view from the Tier, the activated Catalogue version, and
   the estate settings; it is never committed, never rendered under
   `rendered/`, and never judged. Values Telecraft owns are filled in for
   real: the rendered artefact's stable repository path, the
   `supervisor.yaml` content, the OpAMP endpoint, the selector's identity
   attributes. Values the adopter owns appear as explicit placeholders.
   The guidance bakes in ADR-0010's hard rules (named `opamp/<something>`
   extensions, the Downward API identity attribute, durable Supervisor
   storage, `automatic_config_rollback: true`, `accepts_remote_config`
   enabled) so the copy-paste path is the correct path.
5. **Guidance varies by delivery path and substrate.**
   - *Foreign, any substrate*: the rendered artefact's path in git,
     mounted or shipped beside the upstream collector image or package.
   - *Served on a VM or Windows*: upstream Supervisor packages and
     systemd or MSI units, referenced, not vendored.
   - *Served on Kubernetes*: the adopter supplies a combined
     Supervisor-plus-collector image (ADR-0010: upstream ships none and a
     sidecar is structurally unsupported). The image field here is a
     required input, never a default.
6. **Default image by reference, never by distribution.** Where a stock
   collector image works, the guidance pre-fills the upstream collector
   image name, tagged with the activated Catalogue's collector release,
   and offers a custom image field. Telecraft never builds, hosts, or
   mirrors an image. This decision stays inside ADR-0002 and does not
   amend it: a rendered manifest bundle with a working default image on
   the Served path was considered and rejected, because it is a rendered
   DaemonSet in all but name.

## Consequences

- The Claim flow's Tier-drafting branch and this flow converge on one
  implementation; a divergence between them becomes a defect.
- The `never_seen` card gains content, and the finding's age doubles as
  "how long the setup instructions have gone unfollowed".
- Guidance templates join the estate settings surface area: the OpAMP
  endpoint and the self-telemetry destination must be declared before
  the Served guidance can fill itself in.
- No new capitalised domain term is introduced; "setup guidance" stays a
  plain phrase, so the glossary is untouched.
- The tag pre-fill couples guidance to Activation: activating a new
  Catalogue version changes the tag the guidance suggests, which is the
  correct coupling.

## Sources

- Onboarding design conversation (2026-08-27); ADR-0002, ADR-0010,
  ADR-0014, ADR-0028, ADR-0030, ADR-0042, ADR-0061.
