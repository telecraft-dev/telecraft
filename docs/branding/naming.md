# Naming (G0)

- Status: **decided — the project is Telecraft**
- Date: 2026-08-12

## The decision

**Telecraft.** *Tele* is the material — telemetry, the project's entire
subject. *Craft* is the authoring rung — composing telemetry profiles from
Components and Blueprints is skilled work, and no serious tool in this space
brands on craftsmanship. Chosen over the runner-up **Signalkeep** (cleaner
collision profile, weaker meaning) after "Amp-Up" was retired and the
Hopyard/Signalpost metaphor family was rejected ("beer plus trains").

## Verification at decision time (2026-08-12)

| Check | Result |
|---|---|
| npm `telecraft` | free |
| `telecraft.dev`, `telecraft.io`, `telecraft.sh` | unregistered (RDAP) |
| `telecraft.com` | registered (third party) — `.dev` is the home |
| US trademark (software category, serial 76268079) | **cancelled 2022** |
| GitHub org/user `telecraft` | **taken/reserved** — creation flow rejects it despite API 404; lesson: a GitHub 404 is not availability |
| Handle fallbacks | `telecraft-dev` (preferred; mirrors telecraft.dev), `telecraft-io`, `telecrafthq` all signal free |

## Risk register — accepted knowingly

1. **Telecraft E-Solutions Pvt Ltd** (India): active AV/networking
   integrator. Different category and territory; low risk for an open-source
   project. Revisit with counsel before any commercial offering.
2. **MadrasMC/telecraft** (27★): a Minecraft↔Telegram bridge. Different
   domain entirely; shared search results are the only cost.
3. **Telegraf** (InfluxData): phonetic neighbour in the observability aisle.
   Accepted; the audience distinguishes, and the words differ.
4. **"-craft" Minecraft echo**: mitigated by identity design (below), not by
   the name.

## Actions on the human (time-sensitive once public)

- [x] Register `telecraft.dev` — done 2026-08-12; live with the coming-soon
      page (repo `telecraft-dev/telecraft.dev`, Pages via Actions, DNS on
      Cloudflare). `telecraft.io` still optional.
- [ ] Claim npm `telecraft` — placeholder staged; blocked on an interactive
      `npm login`/2FA publish only the human can complete.
- [x] GitHub org: `telecraft-dev` created (bare `telecraft` handle is
      squatted; optional support name-release request still available).
- [x] Main repo live at `telecraft-dev/telecraft`; planning foundation open
      as PR #1.

## Identity direction (seeded `identity.md`)

> Settled in [`identity.md`](identity.md) and [ADR-0047](../adr/0047-visual-identity-and-design-tokens.md)
> on 2026-08-19. The direction below is unchanged; it is kept here as the
> record of where it came from.


Warm-industrial craftsmanship, not gaming: wordmark-first, workshop/precision
aesthetic (calipers, blueprint linework — which also nods to Blueprints as a
domain object), telemetry-signal motifs. Explicitly avoid: blocky/pixel
styling (Minecraft echo), railway kitsch, hop/beer imagery. Serious in the
docs, warm in the community surfaces — same mark.

## Renames that follow (done in the G0 closing commit)

Repo docs sweep: README rewritten under Telecraft; requirements header; the
commit-stamp resource key is **`telecraft.commit`** (ADR-0013). The local
directory/repo rename happens with the GitHub repo rename above.
