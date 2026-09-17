# Editing and reordering queued owner messages

Captured 2026-09-17. Code-internal decision for agent-assistant following
v0.9.0. Not yet built when this was written; it records the shape agreed
before touching the delivery path.

## The problem

The owner can queue several messages, but can only add one or cancel one. A
typo, a missing sentence, or a change of mind about order means cancelling and
retyping. The owner asked for two things: edit a message that has not started,
and reorder the queue — including inserting a new message between existing
ones, as in reordering A B C to A D B C.

The queue is durable and daemon-owned. It drains independently of whether a
browser is connected, which is deliberate: unstarted messages survive a daemon
interruption, and a disconnected browser does not cancel work. Up to twenty
turns may be pending and exactly one runs at a time.

## Why the guarantee is about order, not dependence

The tempting framing is that queued messages depend on each other, so editing
one must block the rest. That is not the reason, and reasoning from it gives
the wrong answer: a reorderable queue implies the owner decides dependence,
and the daemon cannot know it either way.

The guarantee is narrower and firmer: **the queue the owner is looking at is
the queue that runs.** Every rule below follows from that, and none of them
assume anything about what the messages mean.

## The hold is server-side and leased

A browser cannot hold a message back. The daemon already has it and will start
it. So holding is a daemon concept: a marker naming a queued turn, a reason,
and an expiry.

A hold blocks starting that turn **and everything after it**. Blocking only the
edited turn would let a later message overtake it and produce an order the
owner never saw. Turns before it are unaffected and keep running.

The expiry is what stops a closed tab from wedging the queue. The browser
refreshes the lease while an editor or a drag is open; when it stops, the hold
lapses and the queue resumes. Worker owner-holds already survive daemon and
broker restarts, so the durable-hold shape is not new here.

Rejected: a client-side pause, which the daemon would ignore. Rejected: an
unbounded hold, which a crashed browser would never release.

## The daemon never holds unsaved text

An edit is a save. The draft lives in the browser until the owner commits it,
and a turn's text only enters the model's conversation when the turn starts.

That removes the awkward case entirely. If a lease lapses mid-edit, there is
nothing half-written on the daemon to reason about: the queue resumes with the
last saved text, and the browser — discovering its hold is gone — says so
rather than silently sending something the owner never confirmed. Reverting is
a property of the design, not an operation.

## Editing is legal only while queued

Not while running: the text is already in the model's conversation.

Not while delivery is unconfirmed. Those states exist because the browser does
not know whether the daemon received the message; editing there risks a second
delivery carrying different text.

The owner can still open an editor at the moment the daemon starts the turn.
That race is resolved, not avoided: an edit carries the revision it was opened
against, and a lost race is reported as "this message already started" rather
than being applied to a running turn or dropped in silence. Revision-pinned
acceptance already works this way for work items.

A turn keeps its identity across an edit, so the idempotency key that protects
against duplicate delivery still holds. Its revision moves, so a daemon that
has already read the old text cannot start it.

## Order is set atomically, against a revision

Order is the queue's own sequence, changed by one operation that names the
whole intended order and the revision it was decided against. A queue that
changed underneath — another tab reordered, a turn started, a message was
cancelled — rejects it, and the owner sees the current queue rather than a
merge of two intentions.

Rejected: pairwise swaps, which are several round trips, each an opportunity
for a turn to start in the middle of a reorder that is only half applied.

Inserting is part of this. Enqueue appends today, so placing a new message
between existing ones is new surface rather than a special case of reordering.

## Reordering has a keyboard path

Drag and drop is not the only way to move a message. The rest of the dashboard
holds a keyboard standard — the folder picker navigates and activates entirely
from the keyboard — and a queue that can only be reordered by pointer would
regress it.

## What this touches

The at-most-once delivery path: the queue drain, and the client machinery that
distinguishes a confirmed refusal from transport loss. That is the highest-risk
code in the application and was deliberately left alone during the September
structural pass, because getting it wrong means a duplicated or lost message
rather than a cosmetic fault.

Tests therefore cover the failure shapes rather than the happy path: a hold
blocks a start and lapses on expiry, an edit that loses its race is refused, a
reorder against a stale revision is refused, a turn before a hold still runs,
and no edit can produce a second delivery.

## Open

Whether a lapsed lease should also close the editor, or leave the draft in
place for the owner to resubmit against the current revision. Leaving it is
kinder and costs nothing, since the draft was never the daemon's concern.
