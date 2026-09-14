# Personal assistant: configuration and Tailscale access

Date: 2026-09-14. Status: design proposal; commands and configuration names below
are illustrative, not implemented. Local reference: agent-code-review commit
`b2368459c51964c425fee624cc88fb9cb10c967e`, using `lib-agent-mcp v0.23.1`.
External Tailscale documentation was checked on this date; it is rolling
documentation, not a pinned client-version guarantee.

This supplements the [product proposal](2026-09-14-personal-assistant.md) and
[Go stack research](2026-09-14-personal-assistant-stack.md).

## Recommendation

Configuration should express the owner's intentions, authority, and operating
limits. The PA should turn those into a working organization. Requiring the owner
to configure managers, agent reporting trees, and task-by-task check-ins would
give them back the coordination work the assistant was meant to absorb.

I would offer a short guided setup, everyday preferences in chat and the
dashboard, and advanced settings through the same typed configuration service.
Permissions and cost limits would remain distinct from personality and prompts.

## The useful knobs

| Area | Owner controls | Proposed starting behavior |
| --- | --- | --- |
| Identity | PA name, avatar, voice/tone, language, response length, timezone | Concise, candid; personality may evolve within owner preferences |
| Connections and scope | Slack workspace, Linear account/teams/projects, ownership rules, watched and excluded projects | Show discovered scope before enrollment; no automatic authority over every visible project |
| Standing authority | Which assignments it may start automatically, coordination actions it may take, people/channels it may contact | Coordinate explicitly delegated outcomes; offer new assignments until a standing instruction authorizes starting them |
| Decision preferences | Priority rules, quality expectations, acceptable scope tradeoffs, commitments requiring the owner | Resolve routine choices using recorded decisions; escalate meaningful unresolved tradeoffs with a recommendation |
| Communication | DM destination, digest time, quiet hours, urgency exceptions, follow-up cadence | Meaningful changes and prepared decisions; batch routine information |
| PA model and worker profiles | Approved providers/models, reasoning settings, worker runtimes and environments, permitted fallbacks | One PA profile and one worker profile are enough to start |
| Capacity and usage | Concurrent runs, delegation depth ceiling, project caps, API budgets, subscription headroom reserve | Conservative shared capacity; reserve room for the PA to answer the owner |
| Supervision and recovery | Maximum update gap, grace period, retry/recovery limits, unattended working hours | Negotiate task-specific check-ins, investigate missed commitments, resume safely before replacing |
| Memory and retention | Durable preferences, transcript retention, archived project retention, export/forget controls | Preserve decisions and active commitments; allow inspection and correction |
| Daemon and dashboard | Startup service, local port, Tailscale access, authorized viewers, storage/backup locations | Local authenticated dashboard; opt into private Tailscale access during setup |

These are settings groups, not ten screens of mandatory questions. Most should
start with defaults and acquire exceptions only when the owner expresses one.

## Authority should describe concrete actions

“Make sure it gets done” should authorize planning the coordination, selecting
approved workers, following up, and resolving routine questions for the selected
outcome. It should not require another approval at every handoff. The daemon
would preserve that delegation, its scope, and its constraints across restarts.

Standing instructions could then widen where this applies: “Automatically take
charge of new assignments in these two projects,” or “Prepare briefs for other
projects, but ask before starting workers.” Seeing an assignment and having
authority to act on it would be separate facts.

Useful authority controls would include:

- Eligible teams/projects and permitted tracker changes: create project briefs,
  create issues, update coordination status, post progress summaries.
- Approved worker profiles and environments, including which permit project
  code changes. The PA itself would have no code-writing or project shell tool.
- Communication recipients and actions: contact the owner and delegated agents
  by default; contacting other people or posting into shared channels would
  require a matching standing grant or explicit instruction.
- Decision boundaries: changing promised dates, materially expanding scope,
  cancelling outcomes, or making commitments to others could require the owner.
  Ordinary implementation questions would go to the responsible worker/manager.
- Separate actions for pause, stop, and revoke authority. A pause would preserve
  responsibilities and pending questions; a stop would explicitly cancel work.

The implementation prohibition would apply to the PA role. An approved worker
could implement code under a delegated work order. The system-wide prohibitions
on deployment, production-data access, and purchases would follow every
delegation. A manager could not escape those restrictions by creating a child.

Those prohibitions should have no “allow anyway” switch in this product. They
would be enforced by available tools, credentials, and runner isolation, not
just a prompt. A worker CLI inheriting an unrestricted shell and the owner's
credentials could not honestly satisfy that promise. Establishing enforceable
worker environments would be a prerequisite for autonomous dispatch.

Inference and agent execution would be permitted operating usage within the
owner's allowance. Buying credits, upgrading subscriptions, renting new
infrastructure, and making other purchases would remain unavailable.

## Preferences should help it answer questions

Priority and decision preferences are more useful than a generic “autonomy” or
“confidence” slider. Examples: deadline order, preferred scope reductions, what
requires independent review, and which past decision applies to this project.
These should be editable records with scope, source, and a last-confirmed date.
Conflicting or stale preferences would prompt a concrete clarification when it
matters, rather than silently becoming permanent policy.

An escalation should contain the decision, why it is needed now, the PA's
recommendation, alternatives and consequences, and what work can continue.
That quality bar would be a product invariant, not a setting the owner must
remember to enable. Chat and dashboard should resolve the same durable decision
record, so answering in one place immediately settles it in the other.

A decision deadline could control reminders and reprioritization. Silence would
not authorize an action requiring consent. Where the owner had already approved
a specific default, the PA could execute it and explain which instruction applied.

## Capacity, billing, and models

Capacity should be accounted across the entire delegation tree. A limit of four
worker runs should mean four active runs total, including manager turns; spawning
managers should not multiply it. Waiting agents would retain responsibility
without occupying an execution slot. The PA would have a separately reserved
slot so it could remain responsive while workers were busy.

A maximum delegation depth would be a safety ceiling, not a desired structure.
The PA would choose whether a manager helps, and reuse an existing one where
appropriate. The runtime would prevent cycles, orphaned responsibilities, and
unbounded recursive delegation.

Approved runner profiles would bind engine, model, credentials reference,
capabilities, environment, maximum turn/run duration, and billing account.
Changing providers would require an approved fallback profile: it can change
both cost and where project information goes. The PA could select among approved
profiles but could not create a more privileged one or raise its own budget.

The usage controls should distinguish:

- API spend: per-work and aggregate period allowances, warning thresholds, and
  reservations for in-flight work. Period boundaries would name their timezone.
- Subscription usage: available headroom and a reserve for the owner's other
  work. Multiple profiles sharing one account would share one allowance.
- Valuation: reported charge, estimated API-equivalent cost, and unknown cost
  would remain distinct. An API-equivalent estimate would not be called a bill.

The default at an exhausted allowance would be to stop starting chargeable work,
preserve state, and report why. Per-run limits would bound already-running work;
a scheduler checking budgets only between runs cannot guarantee a strict dollar
ceiling. Strict caps would require provider-enforced limits or a runner whose
maximum charge is bounded. Unknown-cost behavior would be explicit; a budgeted
API profile should not launch as though unknown meant free. Existing subscription
profiles could instead use their configured concurrency and quota policy.

An exhausted PA allowance must not suppress the notification: the daemon could
send a deterministic explanation without another model call.

## Supervision without notification spam

Suggested initial tuning values below are hypotheses to test, not measured
optimal settings:

| Setting | Initial proposal | Meaning |
| --- | --- | --- |
| Owner digest | Weekdays at 09:00 in configured timezone | Decisions, outcomes, material risks; omit empty digest |
| Quiet hours | Owner-selected during setup | Defer routine messages; urgency exceptions must be explicit |
| Maximum update gap | 30 minutes for active execution | PA may arrange shorter task-specific expectations |
| Missed-update grace | 5 minutes | Avoid treating small transport delays as failure |
| Automatic recovery attempts | 2 per work item per 24 hours | Persistent count, shared across resumed/replacement sessions |
| Worker execution slots | 4, plus PA reserve | Aggregate across projects and descendants |
| Maximum delegation depth | 3 edges below PA | Ceiling only; small tasks can use one worker directly |

An expected update would include the next meaningful milestone, blocker or
waiting reason, and next expected contact. A heartbeat saying “alive” would not
establish progress. A known wait for owner input, CI, or an external dependency
would have its own expectation; it should not trigger blind restarts every
30 minutes. The PA would challenge repeated updates with no changed evidence.

On silence, it would inspect runner/session state and completion evidence, then
ask the responsible agent a bounded status question if appropriate. Resume would
precede replacement. A timeout would not prove that the previous worker had
stopped; ambiguous dispatch or session state would require reconciliation before
another worker could act on the same work. Repeated recovery failure would
produce an owner decision with the attempted remedies already summarized.

Owner quiet hours, permission to contact other humans, and agent working hours
would be independent. Workers could continue overnight while routine updates
waited. Blocking decisions would remain visible in the dashboard and next
eligible notification, even when routine notifications were batched.

## Memory and personality

The PA could propose or choose its name, conversational style, and avatar during
onboarding when invited. Identity changes would not alter authentication,
permissions, or which account represents the owner. Avatar generation could use
an approved inference profile and its allowance; buying assets would be excluded.

“Be brief,” “remember this preference,” and “send my digest at ten” should work
through conversation with an immediate acknowledgement of the saved change.
Permission increases, credential changes, budget increases, and network exposure
would require explicit authenticated owner intent and a validated operation.
Imported project text and messages from other agents could not issue those changes.

Retention controls would separate transcripts, summarized context, decisions,
and active responsibilities. Expiring a transcript should not lose an unresolved
decision or a promise to follow up. Forget/export operations would cover local
indexes, durable-runtime payloads, and backup retention where applicable; they
must accurately report copies still held by external services. Secrets would
live in the credential store, referenced by config and redacted from exports.

## Tailscale: reuse the family launch pattern

The reviewer reference was `internal/cli/serve.go`, with identity handling in
`internal/dashboard/identity.go`. It called `tailscale.Wire` from the family
library with mode, HTTPS port, local address, and optional public URL. The helper
derived the machine's MagicDNS URL, invoked the installed Tailscale CLI, and
returned a shutdown function. There was no automatic browser-open step in that
flow.

For the PA, I would propose this first-run UX, using a placeholder binary name:

```sh
agent-assistant init
agent-assistant serve --tailscale serve --tailscale-port 8443 --open
```

Setup would save the connection and dashboard preferences. Later starts could
just use `agent-assistant serve`; `agent-assistant dashboard open` would open an
already-running instance. Browser opening would be opt-in for interactive starts,
never an attempt to open a browser from a background service. Optional service
installation would use the host's launchd/systemd facilities on the existing host.

Tailscale Serve provides tailnet access and HTTPS; enabling HTTPS in the tailnet
is a prerequisite. The proposed dashboard would bind `127.0.0.1:8340` and use
the machine's Tailscale hostname with the selected HTTPS port. A PA display name
would not create a DNS name. [Tailscale Serve documentation](https://tailscale.com/docs/features/tailscale-serve)

The reviewer defaulted to local port 8330 and Tailscale port 443. Choosing 8443
in this example would allow coexistence if that port was free. The family helper
accepted 443, 8443, and 10000; that was a wrapper restriction, not a claim that
Tailscale Serve itself only supported those ports.

The following changes would be necessary before reusing the helper for the PA:

1. **Claim local resources before changing routes.** Acquire the instance lock,
   bind the HTTP listener, and establish authentication before exposing it or
   dispatching workers. The reviewer called `Wire` before binding its listener.
2. **Detect route conflicts.** Inspect the existing Serve configuration and
   persist which route this installation owns. Refuse an occupied/unowned route
   with a precise remedy. The inspected helper used `--bg --yes` without checking
   ownership; its shutdown disabled the selected HTTPS port without checking
   whether the mapping had changed.
3. **Recover and clean up narrowly.** Reconcile a previously owned route on boot;
   remove it on clean shutdown only if its configuration still matches ours.
   Serialize our own route operations and skip destructive cleanup when state is
   uncertain. Do not use a global Serve reset. An external administrator can
   still change Tailscale configuration concurrently; our ownership record is
   not an atomic lock over their tools.
4. **Expose only the dashboard.** Internal coordination/runtime administration
   endpoints would stay separate. A future public webhook receiver would have a
   separate, signature-verified ingress, not expose the dashboard alongside it.

The CLI supports inspecting Serve configuration as JSON. Its background mode
persists across restarts, so a process crash cannot be assumed to remove a route.
That is why startup reconciliation matters even with a deferred cleanup function.
[Tailscale Serve CLI reference](https://tailscale.com/docs/reference/tailscale-cli/serve)

The initial access choices should be local or private Tailscale Serve. I would
leave public Funnel out of the initial dashboard settings. Networking controls
should be owner-only and changing them should not silently alter tailnet policy.
Doctor would report missing CLI/login, HTTPS setup, route conflict, or identity
mapping, and provide a concrete remedy. If explicitly requested Tailscale setup
failed at boot, fail before dispatch. A later network outage should preserve
local access and ongoing work while reporting degraded reachability.

## Dashboard authorization

The PA would hold private messages, memories, and authority to dispatch agents.
Every sensitive read and mutation should require a recognized identity. The
reviewer's allowance for anonymous browsing should not carry across.

Serve supplies user identity headers, strips incoming spoofed copies, and omits
user headers for traffic from tagged devices. The application still needs an
owner/viewer allowlist. Only accept those headers through the intended local
proxy boundary. Loopback alone does not distinguish Tailscale from another local
process; this deployment assumes a trusted host account. Stronger multi-user
host isolation would need a separate authentication boundary.
[Tailscale identity-header behavior](https://tailscale.com/docs/features/tailscale-serve)

For local access without Serve, an OS-local CLI could issue a short-lived,
single-use login code exchanged for a protected session cookie. Do not put a
permanent dashboard secret into a URL. Mutations would also need origin/CSRF
protection. Missing Tailscale identity would deny access rather than become a
privileged anonymous session. Tailnet grants would limit network reachability;
application authorization would decide what a recognized person could do.

## Configuration behavior and delivery

Use one versioned schema behind the CLI and dashboard: `config init`, `show`,
`get`, `set`, `unset`, `validate`, and an effective-value view explaining defaults
and overrides. Writes would be validated, atomic, revision-checked, and audited.
Unknown keys would be errors. Secrets would never appear in `config show`.

Ordinary preferences could override defaults at project scope. Permission grants
and resource limits would combine restrictively: a child could not widen its
parent's scope, increase the aggregate allowance, or revive revoked authority.
The effective configuration for a decision would be snapshotted, while authority
would be checked again immediately before an external action.

Tone, digests, and routine cadence changes could apply live at the next relevant
operation. Listener/storage changes would require restart; connection changes
would require controlled reconnect. Lowering capacity would prevent new starts
until running work fell within it; revocation would block pending actions and
cancel affected runs as supported, reporting actions already completed or not
yet confirmed stopped. A boot-time `--no-dispatch` must stay in force despite
live config edits.

I would deliver setup for identity, scope, connections, one PA/worker profile,
usage allowance, communication, and optional Tailscale first. Advanced overrides
could follow as real usage justified them. I would not initially expose manager
counts, agent-tree templates, arbitrary model-editable tool definitions, prompt
temperature, or a slider that combined authority with personality.

Validation before implementation acceptance would include configuration boundary
tests, inherited budget/authority tests, fake-CLI route conflict/crash tests,
dashboard authentication/CSRF tests, and a two-daemon coexistence check on an
explicitly configured development tailnet. No daemon or Tailscale configuration
was changed while writing this proposal.
