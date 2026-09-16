# Context continuity and provider recovery

Captured 2026-09-16. Code-internal decision for agent-assistant v0.7.0;
shared harness interface pinned to a75410cd3cfd.

## Two different kinds of context

The application used constrained CLI completions: a new inference process received
application-owned messages and could propose only daemon-authorized tools. Native
CLI sessions were a separate library surface. Automatic CLI compaction therefore
did not manage the application's durable dialogue or worker transcript.

The former 128 KiB message guard was a byte safety budget, not a model token-window
measurement. Long workers could stop at that guard. The PA replayed only recent
dialogue, potentially losing older decisions from its prompt.

The change introduced two forms of continuity:

- The PA saved a rolling summary of older completed dialogue, keeping recent
  messages exact and retaining the original conversation in durable state.
- During longer tool loops, the assistant and worker summarized older resolved
  exchanges before the working-message budget filled. Every system instruction,
  owner message, unresolved tool exchange and at least the latest complete exchange
  remained exact. The target was two recent complete exchanges; hard pressure could
  reduce this to one. No-tools summaries used the selected CLI profile and consumed
  the same admission budget as ordinary inference.

Worker checkpoints and the original transcript were stored together. PA tool-loop
compaction archived the full input privately under daemon state before replacing
working context. A failed archive or invalid summary stopped compaction. Summary
text was explicitly lossy evidence, never permission or proof of completion.
If immutable instructions and unresolved work alone exceeded the budget, execution
stopped visibly instead of truncating them. New assignments started with fresh
transcripts; resuming the same assignment kept its transcript and checkpoints.

The trigger was 75% of the effective message budget after tool/framing overhead.
Checkpoint counts and byte counts were application telemetry, not invented token
occupancy percentages. These implementation bounds were not new configuration
keys. Transcript retention controls remained future work.

## Native manual compaction

The shared library added `Session.Compact(ctx)` for idle Codex sessions using
`thread/compact/start`. The request acknowledgement was not completion: callers
received a turn, drained its events and awaited the native terminal event.
Context occupancy was invalidated until a fresh measurement arrived.

Claude's interactive `/compact` did not establish a verified stream-json control
operation. Its manual capability stayed unsupported, rather than pretending a
summary prompt had reset the native session. Native Claude automatic compaction
remained separate from application-managed completion context.

## Transient errors and uncertain execution

Typed, secret-safe completion errors distinguished confirmed overload/rate-limit/
unavailable rejections from authentication, context limits and unknown outcomes.
Classification required native error envelopes. Partial output, malformed streams,
transport loss and timeouts were not permission to repeat an inference blindly.
The library classified failures; retry policy remained in the application.

PA completions used up to three retries with jittered exponential delays, a
15-second local delay cap and a two-minute recovery window. A provider Retry-After
was never shortened to fit that window. Each actual request reserved capacity.
The retry boundary covered the failed completion only; already-executed tools were
not replayed. The chat displayed retry state and the next attempt time.

Workers used durable `retry_wait` state instead of sleeping inside the completion
loop. Backoff began at five seconds, doubled with jitter, and respected any longer
provider delay. Six consecutive provider failures or the cumulative model-call cap
stopped automatic recovery. Successful inference reset the consecutive-failure
counter. The daemon rechecked authority, owner/global pause, subscription headroom
and capacity before resuming the existing assignment after its deadline.

Cooldown survived restart and could be paused or stopped at worker level.
Unconfirmed cleanup or resume delivery remained a reconciliation problem; it did
not justify another execution attempt. Authentication and ambiguous failures
surfaced as blockers. Recovery never switched models, accounts or billing routes.

## External evidence consulted

- [Codex App Server](https://learn.chatgpt.com/docs/app-server): manual compaction's
  asynchronous lifecycle and native thread start/resume semantics.
- [Claude Code context management](https://code.claude.com/docs/en/how-claude-code-works):
  native automatic compaction and the limitations of summarized history.
- [Anthropic errors](https://platform.claude.com/docs/en/api/errors): 529 overload,
  429 limits, retry guidance and errors that can occur after streaming starts.
- [Claude incident, 3 September 2026](https://status.claude.com/incidents/461yvfrzpwtt):
  elevated errors affecting multiple models, including Opus 5. Provider failure was
  a real operational condition, not necessarily a login problem.
- [Codex canonical errors](https://github.com/openai/codex/blob/main/codex-rs/protocol/src/error.rs)
  and [exec events](https://github.com/openai/codex/blob/main/codex-rs/exec/src/exec_events.rs):
  terminal exec failures exposed message strings, requiring narrow format matching.
