# Dashboard health, failure evidence and reading order

Date: 2026-09-16. As-of implementation following the owner's UX review of the
dashboard, reviewed live at approximately 1172 × 988. Code-internal except the
shared harness interface already pinned by `go.mod`.

## What the review found

The visual foundation was sound. The information hierarchy was not. The
dashboard exposed execution detail while making the owner hunt for whether work
was progressing, what had stopped, who was handling it, whether a decision was
needed, and what happened next.

The sharpest case: the suggestions worker was blocked, the project read
"Active", the overview read "No decisions waiting on you", and the most recent
assistant reply — accurate when written — said the worker was running. Every
one of those statements was individually true. Together they told the owner
nothing was wrong.

## Health was a presentation gap, not missing policy

`deriveWorkItemStatus` already derived execution state and a reason per outcome.
What did not exist was any rollup, and the overview keyed its only attention
signal off the pending-decision count.

Attention became a pure derivation over the snapshot, computed per request at
the HTTP boundary and never stored. Storing it would have written a derived
diagnosis into durable state, since the store marshals every tagged snapshot
field on each mutation.

Assignments are scanned in their own right rather than only through the outcome
that summarizes them. An outcome reports its liveliest attempt, so a blocked
worker beside a running sibling on the same outcome would otherwise have been
invisible — precisely the reported condition. Reason text is copied verbatim
from whatever produced the state, so one condition never acquires two
descriptions. Open decisions and uninspected interrupted operations are counted
separately from execution: a quiet decision queue is reported as a quiet
decision queue, never as evidence of health.

## The diagnostic gap had two causes, and neither was diagnosable

A blocked worker reported `unknown` in two situations an owner could not tell
apart: a failure that never arrived as a `*completion.RequestError`, and a
typed error whose kind fell outside the preserved set. In both the underlying
error was discarded.

The broker now records which evidence existed — a typed envelope, an untyped
error, an unrecognised provider kind, or the daemon's own preflight context
measurement. This names the evidence, never a reconstructed cause. Provider
text, stderr and credentials stay out of metadata, as before. Attempts recorded
before this field existed report that, rather than being labelled as one of the
cases. Daemon policy stops are deliberately left untagged so a limit is never
presented as a provider diagnosis.

The failure card states what stopped and when, what the evidence does and does
not establish, what progress is preserved, the recovery status and who acts
next, with technical detail behind one disclosure. Rendering it performs no
request and no control action; investigation is an explicit handoff that
proposes a message in the conversation. Resuming remains an owner choice routed
through existing daemon admission.

## Evidence was summarized without being rewritten

Evidence is bounded prose composed by the broker. Introducing a parallel
structured payload would have duplicated the patch excerpt and command output
that the store already rewrites whole on every mutation, and an
acceptance-verified flag authored broker-side is exactly the field that becomes
wrongly truthy later.

Instead the existing lines are grouped for reading by the stable prefixes the
broker authors, and those prefixes are now pinned by a test so a change fails
loudly rather than degrading every grouped view. The canonical lines are
neither rewritten nor dropped, so acceptance still records exactly what the
owner was shown. Command exit status is never presented as acceptance.

## Reading order, duplication and literal text

One worker had appeared in three places with three vocabularies for one status,
two of them mounting the conversation independently and so doubling its
polling. There is now a single assignment view; other places state the
commissioning facts and link to it.

The project page leads with health and current work, then a compact
conversational entry point, with brief, folders, worker configuration and
identifiers collapsed. Queued outcomes became ordered rows stating position and
start condition, sequenced along the dependency chain; work that cannot be
sequenced is still listed rather than hidden.

Worker-authored text is recorded output, not authored formatting, so it renders
literally — shell globs had been parsed as Markdown emphasis. Assistant- and
owner-authored text keeps Markdown.

The activity feed had shown the five oldest events ever recorded: entries were
appended and the view sliced from the front. Ordering is now by recorded time
with a deterministic tiebreak, because those times come from several clocks and
append order is not a proxy for recency.

## Deliberately not done

Memory correction supersedes rather than rewrites, and is its own route: the
existing key-based upsert stays, because the assistant's remember tool
deduplicates on it.

A skewed broker clock was considered as a feed-ordering risk. `UpdateAgent`
already rejects worker timestamps more than a minute ahead of the daemon, so the
exposure is bounded to that minute; no clamp was added.

Superseded work is flagged for evidence review only. Nothing auto-accepts it and
nothing re-implements it.

## Verification

Synthetic fixtures only. The demonstration workspace gained a second fictional
project so running, blocked, awaiting-review and queued states — including the
case where nothing waits on the owner's judgment while a worker has stopped —
are reachable in preview mode rather than only in tests. Tests cover the
attention rollup and its ordering, the two failure-evidence cases, evidence
grouping and the unchanged acceptance payload, queue sequencing, the memory
supersede and its refusal to fork history, and the dashboard's own assertions
that a blocked worker is visible on the overview and one click from its work.
No test contacted a provider, started a worker, or used live owner data.
