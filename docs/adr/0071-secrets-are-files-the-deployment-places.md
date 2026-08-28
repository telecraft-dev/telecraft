# ADR-0071: Secrets are files the deployment places; the estate names them and never carries them

- Status: accepted
- Date: 2026-08-28

## Context

ADR-0067 decided what a Telecraft deployment is: one Instance server, one
process, flags with an environment variable under each, and every shaped
thing authored in the estate under review. It left one hole in its own
§4, deliberately: process configuration carries "the values of the secrets
the estate names", and how a secret value reaches the process is a separate
decision. This is that decision, and nothing in the packaging plan can be
built without it.

The platform already consumes more secret material than any record
acknowledges:

- The **estate repository credential**: an HTTPS token or an SSH deploy
  key, per repository, primary and each satellite (ADR-0028 §5).
- The **forge-adapter credential**: the App private key that mints
  installation tokens (ADR-0014, ADR-0028 §4). CI's live job supplies it
  as `FORGE_APP_PRIVATE_KEY` beside the two identifiers that are not
  secrets, `FORGE_APP_ID` and `FORGE_INSTALLATION_ID`.
- **Identity provider material**: `auth.OIDC.ClientSecret` today, and SAML
  service-provider key material when it arrives.
- The **session signing key**. `auth.NewSessions` draws a random key when
  none is configured, which is honest for one process and wrong for
  anything else.
- The **telemetry backend credential**, `TELECRAFT_TELEMETRY_API_KEY`
  today, carried as a value on `-api-key`.
- Arriving with the plan: the private half of the licence signing pair,
  wherever issuance lives, and the hosted service's per-tenant identity
  and forge material.

Two boundaries are settled and are restated here so nobody reopens them.
TLS private keys are the terminator's, in every deployment shape
(ADR-0067 §5). And the password hashes in `users.yaml` are git-resident by
design: `telecraft passwd` writes a verifier, and a verifier is not the
thing it verifies.

Three constraints bound the answer. REQ-006 and ADR-0019: air-gapped
deployment is first class and nothing may become a hard dependency on a
hosted service. ADR-0032: the serving path holds a closed list of
rebuildable caches, and a single binary plus a directory is a complete
instance. REQ-060: reuse over build.

## Decision

### 1. A live secret is never in the estate repository, and a hash is not a secret

The estate repository holds what is reviewed: topology, policy, ownership,
the providers an Instance offers, and the rendered artefacts. It holds no
value that grants access to anything. Where the estate needs to point at
secret material it names it, and the name is not the material.

The rule has three parts, in the order they bite:

1. **Nothing in the schema takes a value.** Every estate field that
   concerns secret material takes a name (§2), and there is no field
   anywhere that accepts a literal. The estate files are strict-loaded, so
   an author who writes `clientSecret:` gets a load error naming the field
   that does exist. This is the part that actually holds, because it makes
   the mistake unsayable rather than detectable.
2. **A rendered artefact carries indirection, never a credential.** The
   collector's own backend credentials are the adopter's, they never enter
   Telecraft, they never cross the OpAMP wire, and the artefact references
   them as `${env:...}` for the collector to expand at load (ADR-0018).
   An authored Component whose configuration carries something shaped like
   a credential rather than an indirection raises an authoring finding.
   It is a finding and not a block: ADR-0022 §3 keeps exactly one hard
   block, and a pattern match on entropy is not sound enough to be the
   second.
3. **A secret that reaches the history is rotated, never rewritten out.**
   Rewriting changes every commit hash, invalidates every artefact that
   cites one, and does not un-disclose anything that was pushed. The same
   reasoning ADR-0050 §4 applied to the licence line applies here, with
   the addition that the remedy is real: revoke the credential and issue
   another.

`users.yaml` is unaffected. A password hash is a verifier, it grants
nothing on its own, and putting it under review is the point.

### 2. The estate names, the deployment places, the process reads a file

One mechanism, everywhere:

- An estate file names secret material by a **Secret name**: lower-case
  letters, digits and hyphens, and nothing else. No dots, no slashes, no
  leading hyphen. A name cannot describe a path, so a name can never
  escape where it is resolved.
- The deployment places a file of that name in the **Secret directory**,
  `-secrets-dir` (`TELECRAFT_SECRETS_DIR`). The file's contents are the
  value, with one trailing newline tolerated and stripped, because every
  tool that writes one adds it.
- The process resolves the name against the directory and reads the file.
  It resolves nothing else, and it reaches no network to resolve anything.
- Where a subprocess needs the material, the process hands it over
  explicitly. Git is given the estate credential as a key file or through
  an askpass the process controls, never left to find one in an ambient
  credential helper or a user's keychain. What the Instance can reach is
  what the deployment placed, not what the host happens to remember.

The estate names rather than paths because a path is a property of the
deployment shape and the estate is one document served to every shape: the
same `auth.yaml` has to work under a host process reading `/etc/telecraft/secrets`
and a pod reading a projected volume.

The process's own secrets, which the estate does not name, take an
explicit file path with a default under the Secret directory:
`-session-key-file`, `-estate-credential-file`, `-forge-key-file`,
`-telemetry-key-file`, each with the `TELECRAFT_` environment variable
ADR-0067 §4 gives every flag. The flag carries a path. It never carries
the value.

That is the one narrowing this ADR makes to ADR-0067 §4, and it is a
narrowing rather than a contradiction: the flag-then-environment-then-default
convention stands, and for secrets what travels through it is where to
read, not what to read. Environment variables reach `/proc/<pid>/environ`,
every child process, a crash dump, `docker inspect`, and the pod
specification a cluster reader can list. A file reaches whoever can open
it, which is a smaller and more familiar set. Files also rewrite in place,
which is what makes §5 possible.

Identifiers are not secrets and stay ordinary configuration: the forge App
identifier, the installation identifier, the OIDC client identifier, the
issuer URL. The rule is about material that grants access, not about
anything that merely names an account.

### 3. Every deployment shape is the same directory, differently filled

| Shape | How the directory is filled |
|---|---|
| Host process | Files under the service user's own directory, mode 0600, placed by whatever already configures that host. |
| Container and compose | The compose file's `secrets:` block, which presents them at `/run/secrets`, or a read-only bind mount. Never `environment:`, and never an image layer. |
| Kubernetes | A Secret projected as a read-only volume. The chart takes the name of a Secret you created; it never takes a value, because a values file lands in a repository. |
| Hosted | Whatever custody the hosted control plane holds, projected per tenant as the same directory of the same names (§6). |

**External secret managers are documented and never depended on.** An
operator that writes a Kubernetes Secret, a sidecar that renders a file, a
templating step in a pipeline, an encrypted file decrypted at deployment
time: all of them fill a directory, and filling a directory is the whole
interface. Telecraft therefore integrates with none of them, and will not:
a first-party client for a hosted secret manager is a REQ-006 dependency
wearing a plugin's clothes, and the file interface already works with
every one of them, including the ones that do not exist yet.

Air-gap parity falls out rather than being arranged. The floor mechanism,
files a person put there, is the same mechanism the largest deployment
uses. There is no path where the air-gapped shape is the degraded one.

**The process does not police file permissions.** A check that refused a
world-readable file would refuse the correct Kubernetes shape, whose
projected volumes are mode 0644 inside the container by default. Guidance
about modes belongs in the deployment documentation, per shape, where it
can be right.

### 4. Absence is a declared "cannot"; refusal is a fault, and it is loud

ADR-0036 split `UNSET` into incapable and silent: an implementation that
can never populate a reading declares so and renders as not applicable,
while one that is capable and quiet is a fault and is loud. Credentials
take the same split.

**A secret the estate names, and the process cannot resolve, refuses the
start.** The message names the file that named it, the name, and the
directory searched. The estate is reviewed, and it asserts what this
Instance offers; serving something narrower than the reviewed estate
because a file was missing is the instance lying about its own
configuration. A typo in a Secret name silently withdrawing single sign-on
is exactly the failure this refuses.

**A secret the process configures, and that is absent, declares the
capability unavailable.** Nothing is claimed that is not there:

| Absent | What the Instance does |
|---|---|
| Forge key | Change proposals are unavailable. The write endpoints refuse and name what is missing, never a 500 (ADR-0067 §1). |
| Telemetry backend credential | The `TelemetryProvider` declares itself incapable, so Observed readings are `Known: false` and every Outcome that needs one is `unknown`. Not a failure, and never red. |
| Estate credential | Nothing is claimed until something is fetched. `/readyz` stays 503 until the first snapshot is held (ADR-0067 §6), which is the behaviour an unreachable remote already has. |
| Session key | A random key is drawn and the start-up line says so: sessions last as long as the process. This is ADR-0067 §4 unchanged. |

**A secret that is present and refused by its counterparty is a fault, and
it is loud.** The capability declared itself available and then failed, so
it reads as failing, with its `as_of`, on the surface that owns it. It is
never a silent demotion to unavailable, because that would let a rotated
credential look like a deployment that had never configured one.

On the surface, all of this is a reading and nothing more: what is
unavailable, and the name of the material it needs. No surface ever
displays a secret's value, no API endpoint returns one, and nothing is
written to a log, an error, or a self-telemetry attribute. A message may
name a secret; it never quotes one.

### 5. Rotation is writing the file, and Telecraft coordinates none of it

Every secret except the session key is read from its file **at the point
of use**: the estate credential when a fetch runs, the forge key when a
token is minted, the client secret at the token exchange. The value is
held for the operation and not beyond it.

So rotation is one act, writing the file, with no restart and no product
support. A projected Kubernetes Secret updated in place is picked up by
the next operation; so is an operator's rewrite, a sidecar's re-render,
and a person editing a file on a host. Telecraft holds no lease, renews
nothing, watches nothing, and runs no renewal loop. There is no
secret-manager client to configure and no scheduled task to fail quietly.

The session key is the exception and it is read once, at start, because it
is on the path of every request. Rotating it signs everybody out, which is
precisely the behaviour a suspected key compromise wants, and it takes a
restart. That cost is stated rather than engineered around: overlapping
keys, signing with one and verifying against several, is a build, and
REQ-060 asks what it buys. It buys a seamless rotation of a key that
should be rotated rarely, and a sign-out is a fair price for that. If key
rotation stops being rare, the build gets its argument.

The session key is also the one secret whose absence stops being
acceptable the moment there is more than one process. ADR-0067 §2 fixes
the supported shape at one Instance server per Instance, so that day has
not come; when OQ-19 is answered, a configured session key is a
precondition of the answer rather than a discovery made during it.

### 6. The Instance server never holds custody; the hosted control plane may

The hosted service adds a class this posture has no answer for: per-tenant
identity and forge material, held at rest, for tenants who are not
operating the deployment.

This ADR decides the half that is its own, and binds the other half rather
than inventing it:

- **The Instance server's only secret interface is a directory of files
  that something else filled.** That holds in the hosted shape exactly as
  it holds under an air gap. A hosted deployment resolves a tenant's
  Secret names against that tenant's directory, and the serving code path
  is the same code path, so there is one supply mechanism to audit rather
  than two.
- **At-rest custody of per-tenant material is the hosted service's
  decision, not this one.** Where the ciphertext lives, what encrypts it,
  who holds that key, and what an administrator may read back are
  questions the tenancy architecture and the hosted architecture own
  together. Raised as OQ-23.
- **ADR-0032 is not amended here, and not by side effect.** Per-tenant
  material at rest is durable state that the closed list never anticipated.
  If the answer puts custody inside the Instance server's own process, that
  is an amendment to ADR-0032 and it must be argued as one. Putting custody
  in the control plane, which fills directories, leaves the closed list
  untouched, and this ADR states that as the constraint any answer is
  measured against.

Self-managed deployments gain nothing to opt out of: there is no hosted
custody in the binary to disable, because there is none in the binary.

### 7. Short-lived commands keep their environment variable, and say why

`telecraft check` is the CI mode, and a CI runner's secret store hands
values to a process as environment variables. Requiring that runner to
materialise a file first would add a step, and that step is usually a
shell line writing the secret into the workspace, which is worse than
where it started.

So the split is by process lifetime, deliberately, and not by tidiness:
the long-running Instance server takes files; the short-lived,
single-purpose commands (`check` and `observe`) keep
`TELECRAFT_TELEMETRY_API_KEY` and their `-api-key` flag. The exception is
bounded by the reason for it. A command that grows into something
long-running loses the exception with the change that makes it so.

## Consequences

- Packaging has one thing to describe four times. Every shape's guide says
  the same sentence, that Telecraft reads secrets from files in a
  directory, and then says how that shape fills it. The deployment
  documentation the plan calls for is per-shape placement, not per-shape
  mechanism.
- The Helm chart's values seam is settled before the chart is written: it
  names a Secret, never carries one, so a values file is safe to commit and
  external secret operators need no chart support.
- `-api-key` on the Instance server does not exist. What today's
  `TELECRAFT_TELEMETRY_API_KEY` reads as a value on a short-lived command,
  the server reads from `-telemetry-key-file`. The two coexist by §7 and
  the difference is documented rather than smoothed over.
- The write endpoints' refusal shape, which ADR-0067 §1 stated for a
  missing forge credential, generalises: every capability that needs
  material declares whether it has it, and the declaration is what the
  console renders.
- `auth.yaml` gains a `secret:` field naming a Secret name, and the strict
  loader's job includes refusing anything that looks like a value. The
  provider set an Instance offers stays a pull request.
- An authoring-side check for credential-shaped literals in Component
  configuration is new work, sized as a finding rather than a gate, and it
  belongs with the validation path rather than with packaging.
- OQ-20 asks what a collector presents to the OpAMP endpoint. Whatever the
  answer is, it has to reach every collector in every shape, and the
  collector side is not this process, so this ADR constrains only the
  server's half: if the server holds anything to verify against, it is a
  file in the Secret directory like everything else.
- `SECURITY.md` already tells a reader that the instance's credentials are
  reachable if the instance is. That stays true and gains a place to point
  at for what those credentials are and how they are supplied.

## Sources

- ADR-0014 (the App credential), ADR-0018 (`${env:...}` indirection:
  structure is visible, secrets are not), ADR-0019 (pluggable
  authentication, air-gap first class), ADR-0022 §3 (the one hard block),
  ADR-0028 §4 and §5 (the forge seam, and repo onboarding by URL plus
  credential), ADR-0032 (the closed list, git-the-tool), ADR-0036
  (declared incapacity against silent fault), ADR-0050 §4 (a record is not
  corrected by rewriting history), ADR-0067 §1, §2, §4, §5 and §6 (the
  Instance server, its configuration surface, the terminator, the probes).
- `internal/auth/session.go` (the random key and what it costs),
  `internal/auth/oidc.go` (`ClientSecret`), `internal/serving/source.go`
  (the estate source), `internal/provider/forge/github.go` (the App
  credential and the installation token), `cmd/telecraft/check.go`
  (`-api-key`), `.github/workflows/ci.yml` (the live forge job's secrets).
- REQ-006, REQ-017, REQ-033, REQ-060; OQ-19, OQ-20.
