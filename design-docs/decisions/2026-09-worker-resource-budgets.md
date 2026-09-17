# Worker limits are resource limits

Captured 2026-09-17. Code-internal decision for agent-assistant; shared harness
interface pinned to `v0.1.1-0.20260917130449-29dfad423865`.

## What went wrong

A project worker stopped after hours of useful work with
`worker_model_allowance: model_calls=16` at context preparation. Three separate
faults produced that:

- The cap counted successful thinking. Every constrained inference spent one,
  including the summaries the worker used to compact its own context, so the
  last thing it could afford was the operation it needed in order to continue.
- It was cumulative across resumes and never reset, so an exhausted assignment
  died again the moment it resumed.
- The managed worker manager never set the knob, so the broker's own default of
  16 applied while the standalone flag defaulted to 24. Raising it was possible
  only for a manually operated broker.

Each execution attempt was separately cancelled after 30 minutes of wall clock.
Subscription headroom was checked before starting, resuming or instructing a
worker, but never before the requests a running worker made.

## The decision

Worker limits became the resources a worker consumes. Not turns, not tools,
not prompts, not elapsed time.

- **Subscription headroom**, the existing 90%-consumed policy, ran before every
  worker inference, through a callback the daemon injected into the broker. The
  policy and its meter moved to `internal/quota` so a standalone broker applied
  the same rules rather than a second copy of them. This was headroom on a
  shared account; it was not a token budget and not a currency cap.
- **An optional per-assignment token budget**, `limits.worker_token_budget`,
  default 0 meaning disabled. It counted the provider's own reported usage,
  input including cached input as the library normalized it, plus output.
- **The cumulative call cap stopped being authority.** `model_calls` survived as
  diagnostic history and for reading old state; a resume never reset it.
  `--max-turns` was retired with an explicit notice rather than silently kept.
- **The 30-minute cancellation was removed**, with no execution slices replacing
  it. Slices were considered and rejected: admission already ran before every
  request and pause and stop were already checked between operations, so a slice
  boundary would have added a mechanism without adding a decision. The daemon
  context owned the lifetime; individual requests, commands and container
  operations stayed bounded.
- **The only remaining loop protection was observable failure.** A bounded run
  of turns in which every requested operation failed stopped for inspection; any
  success reset it. The daemon's existing stalled-progress escalation was
  unchanged.

`limits.max_model_calls_per_day` and `limits.max_model_turns` remained what they
always were: bounds on the assistant's own conversation and tool loop. They were
documented as such, and were not worker lifetime budgets.

## A refusal is a wait, not a failure

A held request was refused before the transport, so nothing was sent, nothing
was accounted, and the conversation — including any direction the owner had
queued — was untouched. The run reported `usage_wait` with its reason.

This was deliberately not a provider failure. It spent no recovery allowance,
scheduled no retry, set no failure classification, and never reported overload.
A wait whose reset the provider named was re-admitted by the daemon on its own;
a wait on a token budget, or on consumption that could not be established, was
the owner's decision and waited for an explicit resume. Owner pause and stop, a
global pause and a no-dispatch boot all took precedence over either.

## Unknown consumption is not free

A reservation was persisted after admission and before the request left, and
settled by request ID so a repeated or late settlement changed nothing. Three
things therefore became explicit unknown consumption rather than nothing:

- a call the provider reported without usage,
- a reservation that outlived its process,
- the history of a run recorded before this ledger existed, converted exactly
  once at startup.

With a token budget configured, an assignment carrying unknown calls held. The
only way past it was the owner deliberately disabling the budget: raising a
number cannot measure what a provider never reported. A preflight that failed
before the request left added no unknown consumption, because the reservation
was written after the library's non-billable probes.

## Honest bounds

- Account telemetry was cached for about a minute and the provider may cache its
  own report, so admission during that window could overshoot the threshold.
- The token budget was checked before each request, so the request that crossed
  it finished. The overshoot was bounded by one request; it was not a reserve.
- Tokens were counted, not money. No pricing table existed and no estimated cost
  was presented as a subscription charge.
- Concurrency was per broker and defaulted to one, so a long assignment held its
  project's execution slot for its whole life.

## The preferred future execution contract, not built here

The target is a **persistent native CLI session per assignment, with native
tools disabled and only daemon-owned application tools available to it**, served
to the session over MCP by the daemon and executed inside the existing isolated
container. That would keep today's isolation exactly as it is — the model on the
host with the subscription login, effects in a `--network none` container — while
recovering what the constrained completion transport costs today: reasoning
continuity across steps, native tool calling instead of a JSON action envelope
parsed from the final message, streaming and steering, native compaction against
the real token window rather than a byte guard, and one process per assignment
instead of three per inference.

This is recorded as a direction, not a capability. It is not implemented, no
half-integrated native path exists in this codebase, and no unused harness API
was added for it. It needs two additive changes in `lib-agent-harness` that do
not exist in the pinned version:

1. **MCP server configuration in `session.Options`.** `commandArgs` passes no
   `--mcp-config` for Claude and opens Codex as a bare `app-server`; the only
   present lever is the caller-controlled home directory, which is not a
   supported interface and, for Claude, also re-enables the hooks and settings
   the completion transport deliberately disables.
2. **A caller tool-authorization callback.** `streamWire.serverRequest` fails
   closed on every server-originated request — command approval declined,
   permission approval empty, MCP elicitation declined. There is no way for the
   daemon to authorize or execute a tool the session asks for, which is exactly
   the boundary this design depends on.

Letting a native session own tool execution *without* those two changes was
considered and rejected: it either moves effects to the host under the CLI's own
sandbox, or requires the subscription credential and network access inside the
worker container. Both break the isolation boundary the product depends on.

Native sessions are also **not** what fixes resource budgeting, and adopting
them would make it coarser. The completion transport's `BeforeRequest` hook is
an exact pre-billing gate on every inference; a session reports usage per turn,
and the only mid-turn lever is interrupting work in progress. Agent quality and
resource policy are separate problems, and this change is the second one.

## External evidence consulted

Read from the pinned module source rather than documentation:
`completion/codex.go`, `completion/claude.go`, `completion/completion.go`,
`completion/codex_probe.go` for the constrained transport, its per-call process
cost and its usage normalization; `session/session.go`, `session/options.go`,
`session/types.go`, `session/telemetry.go`, `session/transport.go` for native
session capabilities, telemetry and the fail-closed request boundary.
