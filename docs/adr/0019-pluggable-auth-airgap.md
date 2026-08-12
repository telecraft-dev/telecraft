# ADR-0019: Pluggable authentication (OIDC, SAML, basic); ownership-derived authorization; air-gap first-class

- Status: accepted
- Date: 2026-08-12 (session G1)
- Amends: ADR-0014 (GitHub becomes the first-party forge integration, not an
  assumption)

## Context

The platform must be deployable in air-gapped environments where GitHub — or
any SaaS — does not exist. Git remains the right backend for maintenance and
lifecycle (ADR-0003 stands), but identity cannot be delegated to a forge
that may not be there.

## Decision

1. **Authentication is pluggable.** First-party providers: **OIDC** (covers
   Keycloak, Entra, Okta — including self-hosted IdPs in air gaps), **SAML**
   (legacy enterprise), and **basic auth** (bootstrap and break-glass;
   production guidance points at OIDC/SAML). Forge OAuth (GitHub sign-in) is
   a convenience available when that forge integration is active, never a
   requirement.
2. **Authorization is derived from ownership** (ADR-0016) with one source of
   truth — the ownership metadata in the estate repo — and two enforcement
   modes:
   - Where the forge supports review routing, the platform **generates** the
     forge's code-ownership file (CODEOWNERS; GitHub, GitLab and Gitea all
     honour the format) from ownership metadata. Merge rights are the
     forge's.
   - Where no review machinery exists, the platform's own **merge gate**
     enforces owner-approval before a change lands. Deferred until an
     adopter needs it; the seam is designed now.
3. **Attribution survives without a forge account**: commits are authored
   with the authenticated identity's name and email from OIDC/SAML claims,
   so git history remains the audit trail (ADR-0003, ADR-0014's intent).
4. **No hard dependency on any SaaS.** GitHub is the first-party forge
   integration and the hosted default; the git host is a seam.

## Consequences

- New requirement REQ-006 (air-gap deployable) and REQ-017 (pluggable auth).
- The Team hierarchy source stays a seam (`teams.yaml` first-party,
  ADR-0017); OIDC/SAML group-claim mapping is a later provider.
- **G2 inherits an air-gap constraint**: the component Catalogue must be
  vendorable — an air-gapped instance cannot fetch collector-contrib
  metadata at runtime.
- The console read-scope remains instance-wide (ADR-0018); hard read
  isolation = one instance per isolation domain.
