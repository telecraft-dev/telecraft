---
title: Use the hosted service
description: Sign up for an Organisation on cloud.telecraft.dev, connect a repository, sign your people in, and know what is promised about keeping it.
order: 15
---

# Use the hosted service

We run Telecraft for you at `cloud.telecraft.dev`. Your Organisation is one
Instance at an address of your own, `<your-name>.cloud.telecraft.dev`, with its
own estate, its own people, and nothing shared with anybody else's.

It is the same Telecraft you can run yourself. There is no capability here
that a self-managed deployment of the same release does not have, and nothing
in this guide describes something you could not do on your own hardware. What
you are buying is the running of it: the address, the certificate, the
upgrades, the backups, and somebody whose job it is to notice.

## Sign up

Signing up is a request. You ask at `cloud.telecraft.dev` with a Google or
Microsoft Entra ID account, and we open a change against the register of
Organisations that a person reads and merges. Merging it is what creates your
Organisation, so you wait for us rather than for a machine.

You are asked for two names:

- **The name**, which is your address. Lower-case letters, digits and hyphens,
  63 characters at most. It is yours for good: a name is never given to
  anybody else, even after an Organisation closes.
- **The display name**, which is what surfaces show. Write it however you
  like.

When the change merges, your Instance answers at your address and your estate
repository holds one commit: a team, you as its first Owner, and the ways of
signing in below.

## Where your estate lives

An Organisation is created with a **Hosted repository**: an ordinary git
remote we keep for you. Clone it, push to it, run your own checks against it.
It is a complete estate: Telecraft reads it, judges it, and delivers from it
on the day your Organisation exists.

What a Hosted repository cannot do is Authoring. Opening a change proposal
needs a git host with pull requests on it, and a bare repository has none, so
the write endpoints refuse and say what is missing.

To author, connect a repository you own:

1. Follow the link from the front door to install Telecraft on your git host.
2. Choose the account and the repositories it may reach. The install screen is
   your git host's, and what it grants is yours to read and to withdraw there.
3. You come back with an identifier, and we open a change adding it to your
   record. When it merges, Authoring works.

You paste no key and we hold no long-lived credential of yours. Uninstalling
Telecraft at your git host is a complete withdrawal: proposals stop being
available and say why, your estate stops refreshing and every surface carries
its age, and your telemetry is untouched, because we were never in its path.

Moving between the two shapes is a git push and a change to your record, in
both directions.

Telecraft asks for three permissions and nothing else: read repository
metadata, read and write contents, and read and write pull requests. It cannot
change your branch protection, it cannot touch your CI directory, and it never
asks for anything outside the repositories you chose. Where you grant less
than it asks for, the surfaces say what is unavailable and which permission
would make it available.

## Sign your people in

We run no identity provider. Your people sign in through one they already
have.

Every Organisation is created offering **Google** and **Microsoft Entra ID**.
Where you have connected a repository, that git host's sign-in is offered too.

To add your own provider, edit `auth.yaml` in your estate and open a pull
request, exactly as you would on a deployment you ran yourself. See the
[sign-in file format](../reference/sign-in.md). Two things follow from that
being a reviewed change in your own repository:

- Who may change it is whoever may merge into your ownership directory. It is
  your rule, not ours.
- If you remove your last working provider, you fix it by merging a change to
  that file. That needs a git credential and no sign-in to Telecraft at all,
  so you cannot lock yourself out of your own estate.

There is no basic auth in a hosted Organisation, and no form anywhere that
writes provider configuration.

## Supply the value of a secret

Your estate names the secrets it needs, such as a client secret for the
provider you added. It never carries their values.

You supply a value at the front door. Afterwards the surface shows the name,
when it was last set, and which administrator set it. It never shows the value,
and no endpoint returns one: a value is replaced or removed, and never read
back. That is true for us as well as for you.

Supplying a value and merging a change to `auth.yaml` are two different
authorities held by two different people. Each surface says which one it is
asking for.

## What is not offered yet

**We do not serve collectors.** Your Organisation's address carries the
console and the API, and not the endpoint collectors fetch from. Telecraft
judges collectors you deliver to by other means exactly as it judges the ones
it serves, and your delivery path is git.

If you want configuration served over the wire today, run an Instance
yourself: [Serve configurations](serve-configs.md) is unchanged and the whole
of it is free.

## What is promised about keeping it

Three things of yours are durable: your repository, your secrets, and the
record that names your Organisation. Everything you read on the console is
computed from the first of those plus the collectors that are talking to it,
and it recomputes.

For a Hosted repository:

- It is snapshotted daily and mirrored to storage in a second region.
- Snapshots are kept for 30 days.
- A restore goes to the most recent snapshot, so up to 24 hours of authored
  commits can be lost. Findings, verdicts and readings are rebuilt from the
  estate, so a restore loses nothing else.
- There is no point-in-time recovery between snapshots, no retention past 30
  days, and no undelete afterwards.

If you connected a repository of your own, it is backed up by whoever hosts
it, and we hold no durable copy of your estate at all.

**Every clone is a complete backup**, history included. That is a property of
git rather than a feature of ours. Clone your estate and you have a copy we
cannot lose for you.

## Leaving

`git clone`. There is no export format, no archive to request, and nothing to
reconstruct: your estate is the whole of your authored work, and a clone is a
complete copy of it.

## Paying, and not paying

An Organisation is a subscription. Nothing about your estate is counted:
not collectors, not Services, not Tiers. The subscription is handled by a
payment provider, whose own portal is where you change a card or an address.
No payment details reach anything we run.

If a subscription lapses, change proposals are refused and say why. Reading,
judging and delivery are exactly as they were: collectors go on fetching, and
your estate goes on being readable and cloneable.

[Environments](../concepts/environments.md#holding-development-staging-and-production)
covers how to hold development, staging, and production, and what an
Organisation for each of them costs you.

## Closing an Organisation

An administrator asks, we merge the change, and the Instance and its secrets
are destroyed. The repository is kept for 30 days so that somebody who asked
in error, or who has not finished cloning, can still be given it. After that it
is deleted. The name is never issued again.

## What we can reach

We operate your Instance, so we can reach what it holds. An operator with
access to the cluster can reach any Organisation's material, and that operator
is us. That is what hosting is, and it is the one thing that is true here and
can never be true of a deployment you run yourself. It is stated in full in the
[security policy](https://github.com/telecraft-dev/telecraft/blob/main/SECURITY.md).

Self-managed Telecraft gains nothing and loses nothing from any of this. It
never contacts us, it holds no address of ours, and it works with the network
unplugged.
