# Claude CLI interruption and shared harness interface

Captured on 2026-09-16 with Claude Code 2.1.272, model alias `haiku`, effort `low`.
This was a bounded live text-only experiment, not an automated integration test.

## Observed controls

The experiment invoked `claude -p` directly, using `--input-format stream-json`,
`--output-format stream-json`, `--verbose`, and `--include-partial-messages`.
There was no SDK dependency and no direct model API integration. Native login
was reused. Tools, hooks, MCP servers, and custom instructions were disabled.
Session persistence was enabled specifically to test resumption.

After an initialize control handshake, each prompt was sent as a user message.
During text generation, the experiment sent:

```json
{"type":"control_request","request_id":"interrupt-example","request":{"subtype":"interrupt"}}
```

The caller awaited both the matching successful control response and the turn's
terminal result before sending another prompt or closing stdin.

Two paths passed:

1. Interrupt then send a new user message through the same live CLI process.
   The model retained a marker supplied only in the original prompt and obeyed
   the replacement task.
2. Interrupt, close the process, launch `claude -p --resume <session-id>`, and
   send a replacement task. The model recalled markers from both earlier tasks.

The two text interruptions reached terminal results in approximately 29ms and
10ms after the interrupt was sent. These were two observations, not latency
promises. No active tool, subprocess, side effect, crash, or hard kill was tested.
This established composed redirection; it did not establish native mid-turn
steering equivalent to Codex's `turn/steer`.

Interrupted results had subtype `error_during_execution` and `is_error: true`.
They reported zero token counters despite having streamed text. A consumer must
correlate its requested interruption with terminal events and preserve unknown
usage rather than recording these turns as free. The process's reported cost
also retained the preceding completed turn's value on the later interrupted
turn, so blindly summing result costs would double-count.

## Native login environment finding

The initial allowlist (HOME, PATH, TMPDIR) lost access to the existing native
Claude login. Read-only auth checks isolated the missing variable: adding USER
restored login; adding LOGNAME alone did not. No credential was copied or read
by the experiment. ClaudeEnvironment was amended to retain USER while still
excluding ambient API keys, OAuth tokens, and home overrides.

## Proposed shared contract

As of this experiment, the proposed package remained `lib-agent-harness`.
Every provider would implement the same public method signatures. Capabilities
would describe each method as native, composed, unsupported, or unknown, with
its strategy and constraints. Capabilities would be resolved against the actual
binary version, selected model, execution profile, and session options rather
than a permanent engine-name table.

Steer would express the intent to redirect active work. A configurable policy
would select native steering, interrupt-and-continue, or interrupt-and-resume.
A strict caller could require native semantics. The return receipt and event
stream would identify the chosen strategy and any replacement turn ID.
The provider adapter would implement the fallback; applications would not branch
on engine names. Unsupported operations would return a typed error.

For a composed operation, the library would serialize session mutations, verify
the expected active turn, wait for interruption to settle, preserve the session,
and submit the new prompt exactly once. A completed-turn race would return an
explicit outcome rather than silently targeting newer work. An uncertain
interrupt or failed resume would not trigger a blind retry. User-message queueing
would remain an application policy distinct from steering.

Resume references would bind native session identity to execution policy, home,
and working directory. Ephemeral sessions would advertise resume as unavailable.
Interruption would never imply rollback of tools or external side effects.

Sources:
- [Claude SDK control protocol implementation](https://github.com/anthropics/claude-agent-sdk-python/blob/main/src/claude_agent_sdk/_internal/query.py)
- [Claude CLI reference](https://code.claude.com/docs/en/cli-reference)
- [Codex app-server protocol](https://learn.chatgpt.com/docs/app-server)
