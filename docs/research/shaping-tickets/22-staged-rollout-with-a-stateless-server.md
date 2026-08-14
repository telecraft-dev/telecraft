# How does a staged rollout work when the server holds no state?

Type: grilling
Status: open
Blocked by: none (21 resolved)

## Question

Graduated from **Not yet specified** on 4 August 2026. That entry said policy
versioning and rollout was "not sharp until the write path is decided". The write
path is now decided twice over, by tickets 09 and 11, and the second decision
made this question sharp in a way it was not before.

**The tension.** A policy management platform has to answer "roll this to 10%
first" and "roll it back". Ticket 11 made rollback free: `git revert`, and the
audit trail is git history. But it also made the OpAMP server **stateless
transport that reads git and stores nothing**, and a stateless server serving one
repo path cannot decide that *this* collector gets the new config and *that* one
does not. Cohort membership is state.

So either the state lives somewhere, which contradicts premise 9 as ticket 11
amended it, or **the cohort is expressed in git itself**, which is the more
interesting possibility and the one to test first.

Questions to settle:

1. **Can the cohort be a property of the repo rather than the server?** A
   directory or branch per cohort, with collectors selected into one by the same
   selector mechanism ticket 07 already uses to match collectors into a Stage.
   If that works, the server stays stateless and rollout is a git operation like
   everything else.
2. **Who computes cohort membership, and is that computation state?** A selector
   evaluated fresh on every request is not state. A "10% of collectors, and keep
   it the same 10%" is, unless it is derived deterministically from something
   stable like a hash of the collector's identity.
3. **What does a rollout look like as a pull request?** Premise 10 makes the pull
   request the approval. A staged rollout is several changes over time, so is it
   several pull requests, or one merged change plus a promotion mechanism that is
   not a code change at all?
4. **Does the foreign population get any of this?** Argo, Helm and SSM do their
   own progressive delivery, badly or well. Ticket 20 found the Argo family at
   43% and Helm at 77%. Amp-Up should probably not compete with Argo Rollouts. If
   staged rollout only works for the served population, say so, because that is
   a real asymmetry between the two halves of the estate.
5. **What is the failure signal that stops a rollout?** OpAMP gives `FAILED` with
   an error message per collector, adopted in ticket 11, and the Supervisor
   reverts on failure. That is a genuine health signal a git-only path does not
   have. Does anything consume it, or is halting a human decision?
6. **Is versioning of the library the same problem or a different one?** Ticket
   11 added the `library_drift` outcome for a repo that no longer meets a raised
   tier. Raising a tier across a large estate is a rollout too, and it may be the
   same mechanism or a different one.

**Read ticket 21 first.** If it concludes against mandating the Supervisor, there
is no server, and this ticket collapses to "whatever your applier already does",
which is a legitimate and much cheaper answer.
