# Security

Telecraft is source-available under the [Elastic License 2.0](LICENSE), and
people run it against their own estates. If you find a flaw in it, this page
tells you where to send it, what to expect once it arrives, and what a
compromised Telecraft can reach.

## Supported versions

Fixes land on `main` and reach you in the next release. Nothing older is
patched.

Telecraft is pre-1.0. A release is a tag on `main` with no maintenance branch
behind it; the [releases page](docs/contributing/releases.md) explains how one
is cut. A tag never moves once pushed, so a fix is always the next version,
never a new commit under an old name.

| What you are running | What a fix looks like |
|---|---|
| The current release | The next tag, cut once the fix merges |
| `main` | The commit that merges the fix |
| An earlier tag | Upgrade to the current release. There is no backport. |
| A fork, or a build you have patched | Yours to carry, though the advisory tells you what changed and where |

"The current release" means the tag that `release` points at, which
`git ls-remote --tags origin release` resolves. There are no long-term
support lines, and there will not be any before v1.0.0: the project would
rather patch one version properly than several badly.

## Reporting a vulnerability

Report privately through GitHub, from **Security** > **Report a
vulnerability** on the repository, or straight from the
[advisory form](https://github.com/telecraft-dev/telecraft/security/advisories/new).
That channel stays private between you and the maintainers until an advisory
is published, and it needs no published address on either side.

Don't open a public issue for a suspected vulnerability. Everything else,
including a finding you have already decided is not a security problem,
belongs in the
[issue tracker](https://github.com/telecraft-dev/telecraft/issues).

A report is easiest to act on when it carries:

- The version or commit you are running, and how you built it.
- What an attacker gets, and what access they need before they can start.
- The smallest reproduction you have, with any authored files it needs and
  the secrets stripped out of them.
- Anything you already know about mitigation, including whether a
  configuration change closes it.

The same channel covers the project's other repositories, the demo estate and
the site, which carry no policy of their own. Report those here.

## What to expect

Telecraft is a small project. These are response times it can keep, not the
ones an enterprise policy would claim, and the clock starts when the report
arrives.

| Stage | Within |
|---|---|
| An acknowledgement that a person has read the report | 5 working days |
| A verdict: accepted, not a vulnerability, or more information needed, with the reasoning either way | 10 working days |
| An update while a fix is being written, whether or not there is news | 14 days, repeating |
| The fix itself | No fixed date. The advisory names the release that carries it. |

The project asks two things of you in return. Hold public detail until the
advisory is published or 90 days have passed, whichever comes first, so
operators have somewhere to upgrade to before they have something to defend
against. And if you have reason to think the flaw is already being exploited,
say so in the report: that timeline compresses to whatever it takes to warn
people.

There is no bounty. Advisories credit the reporter by whatever name they ask
for, and omit the credit if they'd rather have none.

## What a compromise reaches

Two design decisions shape the blast radius, and each cuts both ways. Read
both before you judge the severity of something you have found.

### Nothing sits in the telemetry path

No component of Telecraft forwards, stores, or terminates telemetry. It ships
configurations, never binaries, so an attacker who owns a Telecraft instance
cannot stop your telemetry, read it in flight, or alter it on the way past.
If the instance is compromised, or offline for any other reason, your
collectors carry on doing what they were last configured to do.

What such an attacker reaches instead is the *next* configuration. Telecraft
renders plain otelcol YAML into git and, where you use the serving rung,
delivers it to collectors over OpAMP. Change what a collector is told next
and you can have it drop a signal, ship to an endpoint of your choosing, or
read a file it has no business reading. That is the real ceiling, and it is
high. Three things sit under it: every change lands as a git commit, so it is
reviewable and revertible rather than silent; the artefact is plain YAML you
can read without Telecraft; and the serving path is opt-in per collector, so
GitOps deployments keep their own merge gate in the way.

Treat the instance's credentials as reachable too: it holds what it needs to
read your telemetry backends, to write to the estate repository, and to talk
to your forge. Its read scope is instance-wide, so if you need hard read
isolation, run one instance per isolation domain.

### You cannot be assumed to get the fix quickly

Air-gapped deployment is first class. Telecraft has no hard dependency on any
hosted service, it doesn't phone home, and it never updates itself. Licence
verification is no exception: it reads the file the deployment named, checks
the signature against keys inside the binary, and opens no socket at any
point. So the maintainers can't see who is running which version, can't
notify you, and can't shorten the distance between a fix existing and a fix
reaching an instance behind an air gap. That distance is whatever your change
process takes, and it is the reason the table above promises a release rather
than a remediation date.

Advisories are written for that reader. Each one names the affected versions
precisely rather than saying "older versions", carries whatever mitigation
applies without an upgrade (a configuration change, a revoked credential, a
port closed at the edge), and describes the flaw in enough detail to act on
offline. To hear about one, watch the repository for releases and security
advisories: nothing else tells you.
