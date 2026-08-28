---
title: Sign-in file format
description: The auth.yaml format, the providers an Instance offers, and the mapping from identity provider groups to Owners.
order: 10
---

# Sign-in file format

`auth.yaml` sits in the estate's ownership directory, beside `teams.yaml` and
`users.yaml`. It says two things: which ways of signing in this Instance
offers, and what a group your identity provider asserts means inside the
estate.

The file is optional. Without it, the Instance offers basic auth alone,
verified against the password hashes `users.yaml` already carries.

```
teams.yaml          # the team tree
users.yaml          # who may sign in, and the Owner each acts as
auth.yaml           # how they sign in, and what a group means
idp-metadata.xml    # a SAML identity provider's own metadata document
```

Editing the file is a pull request, exactly like editing who may author. A
merged change takes effect on the next poll of the estate; nothing restarts.

## No field takes a secret

Every field that concerns secret material names it. The name is not the
material: the deployment places a file of that name in the secret directory,
and the process reads it there.

```sh
printf %s "$CLIENT_SECRET" > /run/secrets/staff-oidc
```

A secret name is lower-case letters, digits and hyphens, so a name can never
describe a path. A name nobody placed a file for stops the start rather than
withdrawing sign-in quietly:

```
serve: provider "staff": the secret "staff-oidc" is named, and there is no file of that name in /run/secrets
```

## providers

One document with a `providers` key holding a list. The order is the order the
sign-in surface offers them in.

| Field | Type | Required | Description |
|---|---|---|---|
| `kind` | string | yes | `basic`, `oidc` or `saml`. |
| `name` | string | no | What the sign-in surface shows, and what the round-trip paths carry. Defaults to the kind. |
| `groups_claim` | string | no | The claim or attribute this provider carries group membership in. See [groups](#groups). Not valid on `basic`. |

### basic

Bootstrap and break-glass, verified against the hashes in `users.yaml`.
Nothing else is configured. Production sign-in belongs on `oidc` or `saml`.

```yaml
providers:
  - kind: basic
```

Generate a hash with `telecraft passwd`, which reads the secret from standard
input and prints the value for the user's `password` field.

### oidc

The authorisation-code flow against any OpenID Connect issuer, including a
self-hosted one with no route to the internet.

| Field | Type | Required | Description |
|---|---|---|---|
| `issuer` | string | yes | The issuer URL, exactly as the provider names itself. Discovery is served beneath it. |
| `client_id` | string | yes | What this Instance is registered as with the issuer. |
| `secret` | string | yes | The **name** of the client secret, never its value. |

```yaml
providers:
  - kind: oidc
    name: staff
    issuer: https://issuer.example
    client_id: telecraft
    secret: staff-oidc
```

Register `https://telecraft.example/api/v1/auth/staff/callback` as the redirect
URI with your provider, replacing `staff` with the name you gave the entry.

The ID token must carry an email claim, because the email is what joins the
identity to `users.yaml`. Nothing else is fetched: the flow reads the token it
is given.

### saml

The service-provider-initiated flow against a SAML 2.0 identity provider. The
Instance sends the person out on the redirect binding and the identity provider
returns them by posting a signed assertion back.

| Field | Type | Required | Description |
|---|---|---|---|
| `entity_id` | string | yes | What this Instance calls itself: the entity identifier the identity provider knows it by, and the audience every assertion must be restricted to. |
| `metadata_file` | string | yes | The identity provider's metadata document, saved beside `auth.yaml`. A file name, never a path. |
| `secret` | string | no | The **name** of the service provider key pair: the certificate and private key, PEM, in one file. Set it when the identity provider requires signed authentication requests or encrypts its assertions. |
| `email_attribute` | string | no | The attribute the assertion carries the email address in. |
| `name_attribute` | string | no | The attribute the assertion carries the display name in. |

```yaml
providers:
  - kind: saml
    name: staff
    entity_id: https://telecraft.example/saml
    metadata_file: idp-metadata.xml
```

Save the identity provider's own metadata document beside `auth.yaml` and name
it here. It is read at load, so a document that names no single sign-on
endpoint, or no signing certificate, stops the start. It is not secret: it is
the identity provider's public description of itself, and it belongs under
review, because its signing certificate changing is exactly the kind of change
somebody should see.

Register `https://telecraft.example/api/v1/auth/staff/callback` as the
assertion consumer service with your identity provider, on the HTTP POST
binding, replacing `staff` with the name you gave the entry.

**SAML needs HTTPS.** The assertion arrives as a form post from the identity
provider's page, and the cookie that carries the sign-in attempt across that
post is only sent over an encrypted connection. Serve the Instance behind TLS
and give it an external URL that begins with `https://`, or the start refuses
with the reason.

Sign-in is service-provider-initiated: the Instance sends the person out and
matches the assertion that comes back to the attempt that asked for it. An
assertion posted at somebody who never started a sign-in here is refused, so
an identity-provider-initiated link into the console will not work.

Leave `email_attribute` and `name_attribute` unset unless your identity
provider releases those claims under names of its own. Without them the
assertion is read for the usual names, and the address falls back to the name
identifier when that is an email address.

## groups

An optional list mapping a group the identity provider asserts to the Owner its
members act as. It resolves who somebody is at sign-in. It never writes the
team tree: the Owners and the Teams stay in `teams.yaml`, and a rule naming an
Owner that file does not hold is a load error.

| Field | Type | Required | Description |
|---|---|---|---|
| `group` | string | yes | The group, exactly as the identity provider spells it. Matching is exact. |
| `owner` | string | yes | The Owner its members act as. Must be in the team tree. |

```yaml
providers:
  - kind: oidc
    issuer: https://issuer.example
    client_id: telecraft
    secret: staff-oidc
    groups_claim: groups

groups:
  - group: platform-engineering
    owner: gateway-owners
  - group: security
    owner: pii-guardians
```

Two halves, and they are separate on purpose. The provider entry says *where*
the groups are, because that is the identity provider's own vocabulary: a claim
name for OIDC, an attribute name for SAML. The `groups` list says *what a group
means*, and there is one such list for the whole Instance, because two identity
providers asserting the same group name are naming the same people.

Rules that mention groups but that no provider feeds are a load error: a mapping
nothing reaches would place nobody while reading as though it worked.

### How somebody is placed

1. If `users.yaml` names their email, that entry decides. It is the explicit
   statement about one person, and a rule never overrides it.
2. Otherwise, the first rule in `groups` whose group they carry decides.
3. Otherwise they cannot sign in, and the refusal names both files.

Order is authority. Somebody in two mapped groups acts as the Owner of the
first rule that matches, so you decide precedence by the order you write the
rules in.

Membership is resolved again on every request, not decided once at sign-in.
Repointing a group at another Owner, or moving that Owner in the tree, changes
what somebody may do on their next request. A rule you add for a group they
hold but were not carrying applies at their next sign-in.

### What a session carries

A session carries the groups this file names and no others. A directory can put
one person in hundreds of groups, and the ones the estate says nothing about
could never change an answer.

### When to use it

Use `users.yaml` alone while the list of people is short enough to read. Reach
for the mapping when it is not, or when your identity provider is already where
joiners and leavers are managed. The two work together: name the handful of
people who need a specific Owner, and let a rule place the rest.

Basic auth asserts nothing about groups. People who sign in with a password are
placed by `users.yaml` and nowhere else.

## Loading

The whole file is refused, and the start stops, when any of these is true:

- It declares no providers.
- A provider names a kind this build does not offer.
- Two providers share a name.
- A field carries a value where the schema takes a name.
- A named secret has no file of that name in the secret directory.
- A SAML metadata document is missing, unreadable, or names no endpoint or
  certificate.
- A group maps to an Owner the team tree does not hold, or a group is mapped
  twice.
- Groups are mapped and no provider names the claim they arrive in.

Every problem in the file is reported at once, so fixing it is one pass.

## See also

- [Estate layout](estate-layout.md) for where these files sit.
- [Run an Instance](../guides/run-an-instance.md) for serving the console and
  placing secrets.
- [Command line reference](cli.md) for `telecraft serve` and `telecraft passwd`.
