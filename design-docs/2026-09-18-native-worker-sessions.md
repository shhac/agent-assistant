# Native worker sessions

Dated 2026-09-18. Replaces the stateless JSON action-envelope loop that
implementation workers ran through, described in
[worker broker](../docs/worker-broker.md) as it stood before this change.

## Why

A worker turn was one `completion.Complete` call. The daemon serialized the
whole accumulated conversation plus its application tool schemas into a fresh
tool-disabled CLI process, asked for a JSON action, executed it, and repeated.
That contract produced, in order:

- unsupported native tool attempts, because a coding CLI is built to call its
  own tools rather than emit an action envelope;
- a ~128 KiB serialized context budget with repeated lossy application
  summaries, and summary-size failures when the archive outgrew it;
- inefficient re-inspection, because each process started with no file cache,
  no native session and no tool memory;
- a 5-minute whole-completion deadline that ended long healthy turns;
- poor streaming: activity was only visible at action boundaries.

The last real run executed inspections, changed zero files, compacted several
times and ended `deadline_exceeded`.

## Goal

Implementation workers behave like ordinary long-running native Claude Code and
Codex coding agents. The native CLI owns the agent loop, the tool exchange,
session persistence and native compaction. The daemon owns assignments, scoped
authority, workspace and runtime isolation, scheduling, lifecycle, human
decisions and peer communication, usage policy, durable evidence, and
acceptance.

The PA itself is unchanged: it keeps the constrained `completion` contract, has
no general shell and cannot edit projects. `completion` remains the right tool
for PA coordination and small inference jobs and is not being removed.

## Runtime route, and why this one

Two routes were considered.

**A. Native coding tools inside the isolated runtime.** The CLI would run inside
the offline container and use its own file and shell tools there. Rejected: the
container has no network by design, and the CLI's subscription login lives in
the host keychain and the host CLI home. Making this work would require either
copying credential material into the worker workspace or giving the container
network and credentials. Both are prohibited, and neither is worth doing to
obtain a nicer tool loop.

**B. Persistent native session on the host, tools routed into the isolated
runtime.** The CLI stays on the host, where its existing subscription login
already works, and runs a real persistent session. Every workspace operation is
an MCP tool served by the daemon and executed with `docker exec` inside the same
offline container the previous design used. This is the route this change takes;
it is the fallback the owner authorized explicitly.

The important property is that the tool loop is genuinely the harness's. The CLI
decides which tool to call and when, calls it over MCP, receives a result and
continues within one native turn, with native session persistence and native
compaction. Nothing is a tool proposal serialized inside model JSON.

Route B only holds if the CLI's *own* tools are actually gone. A host-resident
CLI that keeps built-in file or shell tools can read the owner's credentials,
keychain-backed material and production data and send them upstream as ordinary
model input. That is a disclosure path, and no sandbox mode that merely forbids
writes closes it. Neither does an empty working directory: a working directory
scopes relative paths, not what a tool may open. This design therefore does not
rely on either, and does not claim either as containment.

### Enforcement boundary

Two boundaries, both mechanical:

**1. Native tools are removed from the session, and that is verified before the
credentialed process ever starts.** The shared library builds the restricted
launch configuration for each engine and then proves it against a local,
uncredentialed rejecting provider: it starts the CLI with a disposable home, a
dummy credential and a loopback endpoint that refuses every request, drives one
synthetic turn, and inspects the actual outbound request body. The request must
carry exactly the daemon's MCP tools — no native tool, and none of ours missing —
and no inherited instruction material. Only then is the real session launched
with the owner's login. If the probe cannot establish this, the session is not
launched at all and the run fails closed with a capability error naming what was
wrong. A rejecting provider performs no inference, so no tool loop can run during
the probe, and the probe process holds no credentials to disclose.

After the real session starts, the same surface is cross-checked a second time
against whatever the CLI itself advertises at initialization — still before the
first prompt and therefore before any inference. A mismatch closes the session.

**2. The only path from the model to project files is a daemon-owned tool that
runs inside the offline container.** Specifically:

- The container keeps every property it had: no network, no host sockets, no
  host home, no provider or cloud credentials, read-only root, dropped
  capabilities, `no-new-privileges`, non-root numeric user, memory/CPU/pid
  limits, one copied-workspace mount, bounded tmpfs. Review-only workers still
  get a read-only mount.
- The scoped project copy, the sensitive-name exclusions, the baseline, the
  patch/command artifacts and the independent PA review of that evidence are
  unchanged.
- `deployment`, `production_data_access` and `purchases` remain immutable
  prohibitions, including for descendants.

The CLI process on the host is configured as follows. None of this is offered as
a substitute for boundary 1:

- Host customization is disabled explicitly: no user or project settings
  sources, no hooks, no inherited MCP servers, no slash commands, no plugin,
  browser, or subagent surfaces. The only MCP server is the daemon's, supplied
  inline.
- The environment is the harness's credential-stripped environment. The login is
  the CLI's own, resolved by the CLI; nothing is copied anywhere, and no API
  credential is substituted for the subscription login.
- The working directory is a private per-session scratch directory. This keeps
  relative paths and project instruction files out of the session; it is *not*
  claimed as a filesystem boundary.
- No permission-bypass mode is used. Unhandled provider-originated permission,
  elicitation and approval requests continue to fail closed.

Both engines get a real restricted route, built from mechanics the library
already had for constrained completion:

| | Claude | Codex |
| --- | --- | --- |
| How native tools are removed | explicit tool allowlist containing only `mcp__<server>__*` | model-catalog restriction — `shell_type` disabled, `apply_patch_tool_type` null, empty experimental tool set — plus the feature switches that disable shell, exec, apps, plugins, hooks, subagents, browser and image surfaces |
| Inherited configuration | no setting sources, hooks disabled, strict MCP config | user config and rules ignored, project docs suppressed, app-owned home validated to carry no global instructions |
| Verified before launch | yes — uncredentialed outbound-request probe | yes — uncredentialed outbound-request probe |
| Verified again at startup | yes — advertised tool set, pre-inference | yes where the protocol reports it; recorded as unverified rather than claimed otherwise |

If either engine's installed build cannot be configured this way, the probe says
so and the worker does not start: the run blocks with an actionable capability
error naming the engine, the offending tool surface and what the operator can do
about it. There is no mode that launches a worker with native host tools
attached, and documenting a residual read path is not a way to ship one.

"Native account networking must not imply arbitrary command network access"
holds on both engines: the CLI talks to its provider, and `run_command` executes
in a container with `--network none`.

This restricted contract is **opt-in per session**. Existing callers that open
ordinary native sessions are unaffected; the library does not change what a
session does by default.

## Session model

One durable native session reference per assignment.

- `session.Ref` (engine, native session/thread ID, home, workdir, account label,
  configuration digest) is persisted in the run's private state next to its
  workspace, baseline and evidence.
- Continuation and restart **resume** that session. Resume is exact: the library
  refuses a reference whose recorded configuration no longer matches.
- The configuration digest covers the *stable* contract — engine, binary, model,
  effort, instructions, policy, and the tool surface by name and schema. It
  deliberately excludes the tool channel's ephemeral material: the listener path
  and the per-launch bridge credential change on every restart, and including
  them would invalidate every stored reference each time the daemon started.
  That credential lives in an owner-only file, is never written into the
  reference, never appears in launch arguments, events, evidence or tool
  results, and is not reachable by the model's tools.
- A *new* task gets a *new* session, seeded with relevant project context, not
  the previous assignment's entire coding conversation.
- The run record stores a session phase (`none`, `starting`, `open`,
  `closed`) alongside the reference, written before the process is launched, so
  a crash cannot produce a second worker for the same assignment: restart
  reconciliation sees a recorded phase, verifies and stops any recorded
  container, and converts the run to an explicit recoverable state instead of
  starting again.
- **Stopping the container does not establish that the CLI stopped.** The CLI is
  a host process, and a daemon crash orphans it: it keeps its provider
  connection and keeps spending. Reclaiming it is therefore part of recovery,
  not an afterthought. The library launches the CLI in its own process group and
  anchors liveness on the tool bridge, which is the daemon's own binary running
  as a child of the CLI and holding an exclusive lock for exactly as long as the
  CLI subtree lives. On restart, a still-held lock is proof that an orphaned
  subtree exists; the lock file records the group that holds it, so recovery
  terminates that group and then re-checks the lock before declaring the run
  recoverable. A group that cannot be confirmed gone leaves the run reserved for
  inspection rather than being reported as clean. Orphaned tool execution stops
  independently the moment the daemon's listener goes away, because every tool
  call has to reach the daemon to do anything.
- Uncertain effects are reconciled, not repeated. A turn interrupted after tools
  ran is resumed in the same native session, where the model can see what it
  already did, rather than replayed.

### Legacy assignments

Assignments created by the old contract have a transcript and no native session.
They are not auto-resumed and their preserved transcript is never described as a
native session. On restart they become `blocked` with an explicit
migration decision naming what is preserved (workspace, baseline, artifacts,
commands, steering receipts, usage ledger, authority) and what is not (the model
conversation). An explicit owner resume performs a one-time handoff: a new
native session is opened against the **same** workspace copy and baseline, and
its first turn carries a bounded, generated handoff brief — task, acceptance
criteria, changed paths so far, recorded command outcomes, unresolved steering,
and the last owner direction — rather than the old conversation. The archived
transcript stays in state for inspection. The assignment continues; it is not
restarted from zero and its evidence is not discarded.

## Streaming, activity and diagnostics

Shared `session.Event`s drive everything visible:

- `tool_started` / `tool_completed` with tool name and status,
- `text` / `text_delta` for the worker's own narration,
- `status` for turn outcome,
- `usage` for provider-reported turn consumption,
- `context` for working-context occupancy, including compaction invalidation,
- `quota` / `account` for subscription telemetry.

These are persisted as bounded activity entries on the run and exposed through
the existing worker detail API, the dashboard conversation and the assistant's
inspect tool. Private model reasoning, credentials and raw tool payload bodies
are not exposed; tool *names*, *statuses* and bounded sanitized output excerpts
are.

Two accounting corrections come with this:

- Usage is no longer observed only at the terminal result. Each model response
  within a turn carries provider-reported figures, and those are emitted as
  they arrive, marked as per-request observations rather than as the turn's
  accounting. The terminal figure, when the provider supplies a usable one,
  remains the turn's accounting.
- A **failed or interrupted** turn no longer discards what it consumed. Its
  observed per-request figures are retained as evidence. They are not promoted
  to a measurement: a turn whose terminal accounting is absent, zero-valued
  despite having streamed output, or otherwise unusable is recorded as an
  *unknown* call, exactly as before. Unknown is never zero.

Three things that were previously blurred stay distinct:

- **Session reference** — which native conversation this is.
- **Working context occupancy** — how full that conversation's window is, from
  the provider's own context observation, with estimate/measured quality and
  post-compaction invalidation marked.
- **Cumulative usage** — tokens consumed across the assignment.

Failures keep bounded sanitized diagnostics: engine, phase, code, evidence class
and exit code where known. "Unknown" remains a real classification for genuinely
unknown failures and is not the default for everything.

## Time, health and control

- **No fixed deadline on a healthy coding turn**, and no turn, tool or command
  count used as work authority. An assignment continues while its account has
  headroom and, where configured, while its token budget lasts.
- Bounded operations stay bounded and are separated from long-lived work:
  session startup and control requests (interrupt, steer, telemetry reads) have
  short deadlines; individual container commands keep their 60-second limit and
  captured-output bound; artifact collection stays bounded.
- Health is observed, not assumed. The supervisor distinguishes:
  - **exited** — the CLI process ended (terminal, with its transport error),
  - **active** — a stream or tool is currently running,
  - **quiet** — no event for a while, process alive: reported as *unknown*, not
    as stuck, and never used on its own to kill work,
  - **failed** — the provider reported an explicit failure.
  Silence is not proof of being stuck. The daemon's existing stalled-progress
  escalation, which looks at substantive status, summary and evidence across
  check-in windows, remains the thing that notices genuinely stopped work.
- **Pause** interrupts gracefully: it requests a native interrupt, waits for the
  turn's terminal event, checkpoints, and leaves the session resumable. If the
  interrupt cannot be acknowledged the run stays reserved for reconciliation
  rather than being reported as cleanly paused.
- **Stop** contains the whole process tree (the harness's existing process-group
  containment) and confirms container removal before the run is terminal. A
  stopped run cannot resume.
- **Steer** is delivered through the harness's supported mechanism — native
  `turn/steer` on Codex, interrupt-and-continue on Claude — and the receipt says
  which strategy was used. Direction that arrives while no turn is active is
  queued and delivered on the next turn. Delivery is delivery: the worker's
  explicit `acknowledge_steering` receipt remains the only thing that counts as
  having read it, and a receipt is still not evidence of implementation.

## Usage policy under a native loop

A native turn makes **many** internal provider requests. The old guarantee — one
admission per provider request — is not available and is not claimed.

What is implemented:

- **Admission before each turn.** The same `Admit` callback and token-budget
  check run before a turn is started or resumed, so no turn begins outside
  policy.
- **Streamed monitoring during a turn**, where the provider actually streams it.
  Per-request usage observations and quota events are applied as they arrive,
  and a turn that crosses the configured budget or the subscription threshold
  mid-flight is interrupted and held at a checkpoint rather than left to finish.
  This is monitoring, not enforcement, and the distinction is load-bearing: if
  an engine reports usage only at the terminal result, mid-turn enforcement for
  that engine does not exist and is not claimed. The capability is reported per
  engine from what was actually observed, not from what the design hoped for.
- **Explicitly documented granularity.** Overshoot within an in-flight turn is
  possible, bounded by how quickly the provider reports and the interrupt
  settles — and, on an engine with terminal-only reporting, bounded only by the
  turn. Neither gate is a pre-reserved guarantee or a currency limit. This is
  stated in the operator documentation, not only here.
- **Quota rechecks are scoped to the run that made them.** A failed or stale
  telemetry read holds that assignment under the configured unavailable-usage
  policy. It does not become a daemon-wide stop, and it does not hold
  assignments on other profiles or other accounts.
- **Unavailable accounting stays unknown.** A turn that reports no usable usage
  figure increments unknown calls; it is never recorded as zero. A configured
  budget holds on unknown consumption for an owner decision. A reservation is
  written before a turn starts, so a crash records an unmeasured turn as unknown.
- **Holds preserve state and spend no provider retry allowance.** A hold is a
  wait: the session is checkpointed and left resumable, no retry is scheduled,
  no provider classification is attached, and the execution slot is released.
- **The library exposes mechanics; the app owns the decisions.** The library
  reports account, quota, context and usage observations and offers admission
  and interrupt hooks. Budgets, thresholds, retry and hold policy stay in the
  daemon.
- **No layered whole-turn replay.** Provider retry and backoff may happen inside
  the CLI, where the side effects are known. The daemon does not replay a turn
  whose tool effects are uncertain.

## Coordination during native execution

`ask_decision`, `send_message`, `acknowledge_steering` and `finish` remain
daemon-mediated tools, now delivered over MCP inside the native session:

A native turn can issue several tool calls at once, so "this tool ends the turn"
has to be a latch, not a convention. `finish`, `ask_decision` and `send_message`
close the session's tool channel **atomically, inside the call that invokes
them**. Every later call in that same turn — including one already in flight
concurrently — is refused with an explicit closed-channel result and executes
nothing. The daemon then requests a native interrupt and checkpoints. No health
event, turn status or quiet period is treated as a substitute: only an explicit
tool call ends the work, and only collected evidence accepts it.

- `ask_decision` checkpoints the turn and blocks the run with a prepared,
  durable pending decision; the PA or owner answers, and the answer is delivered
  into the **same** native session on resume.
- `send_message` yields until the daemon acknowledges routing; sender identity
  and untrusted-content marking are supplied by the daemon, and peer content
  never grants authority.
- `acknowledge_steering` records cumulative, unique, daemon-validated IDs.
- `finish` supplies an acceptance summary and nothing more. Finishing a native
  turn is **not** acceptance: the daemon still collects the actual patch, the
  changed-path set and the recorded command evidence, and the PA still reviews
  that evidence independently against every acceptance criterion.

## What is removed

The worker-only stateless machinery goes, rather than being maintained beside
the new path: the per-action `Complete` loop, the application-owned worker
context window and its summarizer, the serialized worker tool catalog, the
interrupted-tool transcript repair, the 5-minute completion deadline and the
`ContextBytes`/`ContextCompactions` byte accounting that described it.

Deliberately kept: the constrained `completion` contract for the PA and small
inference jobs; the isolated container executor and its evidence; the
daemon/broker HTTP contract, its idempotency keys, hold kinds and control
capabilities; legacy state compatibility for migration.

No retired dial is reintroduced. `--max-turns` stays retired and still prints a
notice.

## Acceptance criteria

1. A worker completes a multi-step read → edit → test → finish assignment inside
   a single native session, with no application-side summarization and no
   whole-turn deadline, and progresses past the old 5-minute boundary (shown
   with controlled time in tests). The fix is the removal of the deadline and the
   preservation of the native loop, not a larger number.
2. Both engines have a working restricted route; neither is a stub, and neither
   ships with native host tools attached. A build that cannot be restricted
   fails closed with an actionable capability error before launch.
3. Restart resumes the same native session; a crash mid-turn produces exactly
   one worker, not two, and an explicitly recoverable state.
4. Pause checkpoints and resumes; stop contains the process tree and prevents
   later execution; steer is delivered with a truthful strategy receipt.
5. Decisions and peer messages work during native execution, with durable
   pending state and acknowledgement.
6. Quota and budget holds preserve state, spend no retry allowance, and are
   honest about granularity.
7. Legacy completion assignments migrate on explicit owner resume, keep their
   workspace, evidence, authority and usage ledger, and continue.
8. Evidence and independent PA review are unchanged; a native `finish` does not
   accept anything by itself.
9. Failures carry bounded sanitized diagnostics rather than collapsing to
   unknown.
