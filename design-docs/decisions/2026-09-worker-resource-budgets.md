# Worker limits are resource limits

Decision captured 2026-09-17. The implementation uses
`lib-agent-harness v0.1.1-0.20260917165512-38afd4a1c88e`.

## Problem

A project worker stopped at context preparation with
`worker_model_allowance: model_calls=16`. Each completion, including context
summaries, consumed a call. The allowance survived resumes, so resuming an
exhausted assignment immediately stopped it again. Managed brokers inherited a
16-call default while the standalone CLI defaulted to 24.

Two other limits had the same problem: a lifetime allowance of 128 commands and
a 30-minute execution deadline. None measured the resource the owner wanted to
limit. Subscription usage was already checked before dispatch and resume, but
running workers could continue making model requests without another check.

## Alternatives and decision

Raising the call ceiling would only postpone the same failure. Execution slices
would add continuation machinery without adding a useful authorization boundary:
the daemon already checks between operations and before each model completion.
A persistent native session could improve reasoning continuity and streaming,
but would still need resource policy and a verified tool-execution boundary.

Keep daemon-controlled completion for this change and replace lifetime counts
with two independent resource controls:

- **Subscription headroom:** keep the existing 90%-consumed default, checking it
  before every worker completion, including summaries and recovery attempts.
  Share the meter and window policy in `internal/quota` between managed and
  standalone brokers. The running broker supplies its actual engine, binary and
  login home; only thresholds are read from live settings. Editing defaults must
  not make an active worker inspect a different account.
- **Optional assignment token budget:** `limits.worker_token_budget`, default
  `0` (disabled). Count reported input, including cached input according to the
  harness normalization, plus output, across summaries and resumes. This is an
  assignment budget, not a global daily token budget or a currency limit.

Call counts remain diagnostic history. Neither successful calls nor commands
consume an execution allowance, and `--max-turns` is explicitly deprecated.
There is no task-wide deadline or replacement execution slice. The daemon owns
the lifetime; individual model requests, commands and container operations keep
their timeouts and output bounds.

Do not replace the removed ceilings with a failed-turn heuristic: repeated red
tests can be legitimate diagnostic work. Existing provider-recovery controls,
stalled-progress escalation and reconciliation of uncertain effects remain.
The assistant's own `limits.max_model_calls_per_day` and `max_model_turns` remain
separate conversation controls; neither is a worker token budget.

## Resource waits and owner controls

A refused completion makes no model request and consumes no new usage. Preserve
its transcript, queued direction and workspace. Report `usage_wait` with the
specific reason rather than a model failure, and spend no recovery allowance.

Subscription exhaustion and unavailable telemetry under the fail-closed policy
are rechecked automatically. Honour `NextCheckAt`, then re-read policy and usage
before continuing. A provider reset time is informative, not a mandatory sleep:
a threshold change or a fresh reading can make work eligible sooner. Missing or
unrecognized hold metadata does not authorize automatic continuation.

Token exhaustion and unestablished consumption need an owner decision and an
explicit resume. Owner-action holds outrank ordinary resource waits in project
attention. Both the dashboard and assistant can inspect usage and use the same
worker controls. Owner pause/stop, global coordination pause and no-dispatch
mode take precedence over automatic continuation.

The standalone broker enforces its own configured admission policy but does not
resume itself. Its connecting coordinator owns continuation, just as the daemon
does for managed brokers. A resource hold releases execution capacity after
cleanup; it does not end the assignment or discard its progress.

## Durable accounting

Persist a request reservation after non-billable probes and admission, before
launching inference. Settle it by request ID; duplicate settlements do not add
usage twice. A failed settlement must reach the caller before proposals can
execute, and an unresolved reservation must prevent another request from
replacing it, even when the token budget is disabled.

Usage is unknown when the provider omits or cannot establish it, when a pending
reservation survives a crash, or when history predates the ledger. Convert old
history once, preserving call counts and context. After a persistence problem
is repaired, an explicit resume of a non-running assignment converts a remaining
reservation to one unknown call rather than repeating an unbreakable hold.

With a token budget enabled, unknown consumption holds the assignment. Raising
the budget cannot measure missing usage; deliberately disabling it and resuming
allows continuation while retaining the uncertainty. A failed preflight before
reservation adds no unknown consumption.

Provider parsing belongs in `lib-agent-harness`. Success and error paths use the
same validated terminal accounting, including usable reports accompanying a
nonzero CLI exit. Explicit zero is known; missing fields, negative or overflowing
counts, malformed streams and ambiguous terminal reports remain unknown. Errors
retain their classification and never become executable proposals. The optional
HTTP adapter applies equivalent validation to its own response format. Persisted
input and output totals must also remain representable when added together.

## Measurement limits

Account telemetry is cached for about a minute, and the provider may cache its
own report. Already admitted work can therefore exceed the percentage threshold.
Unavailable readings follow the explicit `allow` or `pause` policy; they are
never interpreted as zero consumption.

The token budget is a threshold before the next application completion, not a
prepaid hard cap. A CLI or provider can perform multiple internal requests inside
one admitted invocation, and that invocation can cross the threshold before its
usage is reported. No API-price estimate is presented as a subscription charge.

## Preferred future execution contract

A persistent native CLI session per assignment, with native tools disabled and
only daemon-owned application tools available over MCP, could preserve reasoning
continuity while adding streaming, steering and native context compaction. Tool
effects would stay in the isolated offline container; the CLI would keep using
its host subscription login. That would also avoid repeated CLI startup and
reconstruction of the full conversation for each completion.

This contract is not implemented here. The current harness session interface
would need explicit MCP server configuration and a caller authorization boundary;
its server-request handling currently fails closed. Those features also need
verified isolation of hooks, settings and native tools. Letting a native session
execute host tools, or moving credentials and network access into the worker
container, would change the existing security boundary.

Native sessions do not themselves solve resource budgeting. Their internal
requests are less directly controlled by the daemon; admission, usage reporting,
tool checkpoints and interruption would need a deliberate enforcement contract.
The durable ledger and resource-wait policy remain useful under either transport.

## Evidence and verification

The decision was based on the repository and harness implementation, including
`completion/{claude,codex,usage}.go` and `session/{options,transport,telemetry}.go`.
Regression tests cover crossing the former call and command ceilings, old-state
resume, summary accounting, refused requests, persistence failures, explicit
recovery, unknown usage, overflow, quota/telemetry recovery, live thresholds with
fixed login identity, owner-control precedence and dashboard attention.
