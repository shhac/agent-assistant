# Personal assistant: September 2026 stack research

Research date: 2026-09-14. Status: recommendation from primary documentation,
not a benchmark or an implementation decision. Sources were checked on this
date; rolling documentation is not a pinned release contract. Exact Go,
SDK, and server versions would be pinned after the integration spike below.

This supplements [the product proposal](2026-09-14-personal-assistant.md).
It incorporates dynamic delegation, escalation through agents, and proactive
supervision of quiet teams. Owner constraints: our own codebase, Go as the main
language, and no cloning or extending OpenClaw. Dashboard TypeScript is acceptable.

## Recommendation

I would build **our own Go daemon using Cobra and the family libraries, evaluate
Eino for the PA's model/tool loop, use Restate's Go SDK for durable coordination,
SQLite for queryable product records, and a React/Vite dashboard**. Slack, Linear,
policy, supervision, and runner integration would live in Go. Eino and Restate
are the first spike candidates, not dependencies already proven together.

This is a fit judgment, not a claim that these tools universally outperform the
alternatives. The important split is between the model deciding how to organize
work and the runtime remembering responsibility, delivering messages, enforcing
authority, and recovering interrupted operations.

The PA's delegation tree should be data created during execution. A project
manager would be an agent with a coordination brief and appropriate capabilities,
not a mandatory layer or a dedicated subsystem. The runtime would support
delegation to an agent, that agent delegating further, and questions travelling
back through the responsibility chain. The PA could send a small project task
straight to a worker. The PA's own prohibition on implementation would still apply.

## Go owns the application

The reviewer demonstrated useful daemon behavior, but its main task was to launch
a CLI, collect a verdict, and record it. This PA would also need interactive
agent turns, concurrent conversations, structured questions, timers that survive
restarts, and supervision across sessions and providers.

Go-native agent libraries and durable-runtime SDKs support this direction. The
reviewer's CLI conventions, diagnostics, process lifecycle, and engine adapters
would remain useful starting points. Domain behavior would be our own Go code.
The existence of more TypeScript agent examples is not a reason to add a Node
backend to this product.

| Layer | Proposed choice | Reason for this product |
| --- | --- | --- |
| Application and CLI | Go, Cobra, lib-agent-cli, lib-agent-output, net/http | Reuse family conventions and own the application logic |
| PA reasoning | Eino behind a narrow Go engine interface, subject to spike | Go-native model/tool execution with streaming and interruption facilities |
| Durable coordination | Restate Go SDK and a self-hosted server | Preserve waits, follow-ups, session execution, and recovery |
| Product storage | SQLite and migrations | Query project bindings, preferences, briefs, decisions, and dashboard views locally |
| Dashboard | React, Vite, TanStack Query | Conventional client application for chat, decisions, activity, and live state |
| Slack | slack-go/slack, initially Socket Mode | A Go connector for private messages and interactive responses |
| Tracker | Typed Linear GraphQL operations from Go | Fetch assignments and project state without giving the PA arbitrary API access |
| Project workers | Adapters to configured agent runtimes | Allow different models, billing arrangements, and execution environments |

The frontend choice is less consequential than the runtime. Svelte remains a
reasonable option and has AI SDK support; the research did not establish that
React is faster or more capable for this dashboard. I slightly prefer React for
a new application composed around chat and data components. Keep Vite and a
static SPA; a separate SSR application would not improve the initial owner-only
dashboard. Go would embed and serve the built assets, following the reviewer's
committed-bundle pattern. Node would be a frontend development/build dependency,
not a required backend runtime.

## Go-native agent-runtime shortlist

### Eino: first framework candidate

CloudWeGo's Eino provides a Go agent development kit, model/tool components,
streaming, and multi-agent composition. Its Runner documents checkpoint storage,
interrupt/resume, and a multi-turn runtime. This is enough to justify a focused
integration spike without building all model-loop machinery ourselves.
[Eino overview](https://www.cloudwego.io/docs/eino/overview/),
[Runner and recovery](https://www.cloudwego.io/docs/eino/core_modules/eino_adk/agent_extension/).

I would use a minimal ChatModelAgent with our explicit coordination tools. I
would not adopt a prebuilt coding/deep-agent configuration, filesystem middleware,
or mandatory planner/manager pattern. Our work orders, permissions, and decision
records would stay outside Eino-specific state.

Eino checkpoints and Restate journals solve overlapping parts of recovery; simply
wrapping an entire Eino run in one retried function is insufficient. The spike
must establish model/tool boundaries, interrupt restoration, and upgrade behavior.
Restate would own durable work lifecycles and timers; the agent adapter would own
recovering a bounded model turn. A deliberate pause checkpoint is not proof of
recovery from a crash at any arbitrary instruction.

### Grafana AI SDK for Go: lighter alternative

Grafana's Go SDK documents multi-step tool execution, structured output, several
providers, and compatibility with React chat hooks. It is a distinct Go library,
not the Vercel TypeScript runtime. This is a promising smaller layer if Eino
introduces too much framework state. I have not validated its provider feature
coverage or release compatibility for our selected models.
[Grafana AI SDK](https://github.com/grafana/ai-sdk).

### Google ADK Go: another credible framework

Google maintains a Go ADK and a Go quickstart. It belongs in the shortlist for
agent composition, but the actual providers, persistence implementation, and
dynamic session behavior we need should be checked in Go rather than inferred
from Python or TypeScript examples. I would not add a second agent framework
unless the first candidate failed a specific requirement.
[ADK Go repository](https://github.com/google/adk-go),
[Go quickstart](https://adk.dev/get-started/go/).

Direct provider SDKs behind our own small model/tool loop remain a fallback if
neither library gives sufficiently clear recovery boundaries. That fallback
would own more streaming and provider-specific semantics, which is why I would
try a focused Go library first. Existing CLI harnesses remain separate runner
adapters and can retain their configured subscription or API billing models.

## Cross-language options examined

### Vercel AI SDK: useful comparison, not the Go backend

The official repository describes a provider-independent TypeScript toolkit,
direct provider packages, structured output, a `ToolLoopAgent`, and UI bindings
including React and Svelte. Vercel hosting or its model gateway is not required:
direct provider adapters are documented. I would configure those explicitly so
provider routing and billing remain visible.
[AI SDK repository and examples](https://github.com/vercel/ai).

Its narrow model/tool abstraction is a useful comparison for the Go libraries.
The owner's language constraint excludes using it as our application runtime;
frontend UI components do not imply adopting its Node backend.

### OpenAI Agents SDK and Agents API: viable, distinct choices

The Agents SDK documentation supports application-owned tools, storage, runtime
behavior, and orchestration in TypeScript or Python. Those are the SDK languages
documented by this source; we should not describe it as a first-party Go Agents
SDK. It remains a comparison point rather than the chosen Go runtime.
[OpenAI Agents SDK](https://developers.openai.com/api/docs/guides/agents/sdk).

The current Agents API is a separate managed-harness option: OpenAI documents
managed sessions, compaction, orchestration, and recovery. Its multi-agent surface
supplies create/message/wait/interrupt tools, including an example with no
execution environment. It deserves consideration for an OpenAI-centric build;
account access, tool restrictions, retention, and fit with independently billed
external workers would need verification. I would not make this provider-specific
session system the sole record of cross-provider project responsibility.
[Agents API](https://developers.openai.com/api/docs/guides/agents-api/overview),
[multi-agent support](https://developers.openai.com/api/docs/guides/agents-api/multi-agent).

### Claude Agent SDK: strong worker adapter, possible PA runtime

The current documentation explicitly supports subagents that spawn their own
subagents. It documents depth, concurrency, and aggregate query-spend limits for
recent SDK releases, as well as separate contexts and tool configuration. Advice
that Claude subagents cannot nest would be stale for the documented versions.
[Claude Agent SDK subagents](https://code.claude.com/docs/en/agent-sdk/subagents).

This is attractive when we want an existing agent harness and its session
behavior. I would initially use it behind a worker adapter and test a restricted
PA profile as an alternative. Tool allow rules are not equivalent to removing
tools, and a model's nested calls do not automatically give the application
cross-provider responsibility tracking or independent project lifetimes. Our
PA remains a coordinator even if the underlying harness can edit code.

### LangGraph: capable of dynamic orchestration

LangGraph's JavaScript documentation describes dynamic orchestrator/worker
behavior through `Send`, alongside persistence and interruption facilities.
It does not require a predeclared manager for every project.
[Dynamic workflows and agents](https://docs.langchain.com/oss/javascript/langgraph/workflows-agents),
[LangGraph overview](https://docs.langchain.com/oss/javascript/langgraph/overview).

The JavaScript framework is not a fit for the Go application constraint. Its
dynamic-worker support demonstrates that a framework need not dictate a fixed
hierarchy. We should preserve that property in our Go design.

### Mastra: convenient integration, check the durability boundary

Mastra's June 26, 2026 durable-agents announcement distinguishes reconnectable
streams from execution that survives backend crashes. It describes a cached-stream
wrapper and an Inngest-backed path for crash recovery. That distinction matters
here: reconnecting a dashboard must not be confused with recovering a PA that
owed somebody an answer overnight.
[Mastra durable agents](https://mastra.ai/blog/introducing-durable-agents).

Mastra's TypeScript runtime is outside the chosen backend stack. The distinction
between a resumable stream and a recoverable operation remains a useful test for
the Go candidates too.

## Durable runtime: why Restate leads my shortlist

Restate documents durable sessions, external-event waits, timers, and a native
Go SDK. These map directly to
“ask the manager, keep the PA responsive, and continue when the answer arrives.”
[Restate agent patterns](https://docs.restate.dev/ai),
[Restate Go SDK](https://docs.restate.dev/develop/go/services),
[workflows and waits](https://docs.restate.dev/tour/workflows).

Its server is available as a single binary, so the first installation could run
our Go service and one local Restate process, without a managed cloud dependency.
This sacrifices the earlier single-Go-binary packaging idea. A single-node
installation still depends on its persistent disk and backups; recovery from a
process crash is not recovery from losing the machine.
[Self-hosted Restate](https://docs.restate.dev/server/overview).

Temporal is the strongest alternative I would carry into the spike. Its
Go API documents workflow messaging through signals, queries, and updates;
its workflow model also supports child workflows and durable
timers: enough expressive power for this design. I would favor it if we already
operated Temporal or needed an organization-wide workflow platform. For a personal
installation, Restate's deployment shape makes it my initial
preference. This research did not benchmark either system's latency or resource
use.
[Temporal Go workflow messaging](https://docs.temporal.io/develop/go/workflows/message-passing).

We must still design replay-safe external actions. A durable runtime cannot
magically make an arbitrary Slack send or worker launch exactly once. Persist
operation IDs, use provider idempotency where supported, and reconcile ambiguous
outcomes before retrying. Model/tool results must be recorded at the correct
durable boundaries so a replay does not blindly repeat effects.

There would be one authority for each class of state: Restate for execution
and waits; SQLite for product records and queryable projections. Changes crossing
that boundary would use stable IDs and idempotent operations. A dashboard status
projection must never independently launch recovery. Completed or abandoned
executions would have retention and archival rules.

## OpenClaw is a reference, not the product definition

OpenClaw already documents an always-on gateway, messaging integrations, session
tools, and configurable nested subagents. It is a useful comparator for how much
plumbing already exists. The nesting documentation also exposes practical limits
on depth and child concurrency.
[OpenClaw overview](https://docs.openclaw.ai/help/faq/what-is-openclaw),
[nested subagents](https://docs.openclaw.ai/tools/subagents/nesting).

The owner's goal is a focused assistant that carries responsibility and brings
prepared decisions. I would not copy OpenClaw's feature catalogue, navigation,
plugin ecosystem, or general-purpose execution experience. Its security guidance
also makes the gateway's trust boundary explicit; per-agent configuration is not
a reason to assume independently trusted environments.
[OpenClaw security model](https://docs.openclaw.ai/gateway/security).

The owner explicitly wants our own code. We would not clone OpenClaw, extend it
as the foundation, or ship a reskinned distribution. Only design lessons about
sessions, channel reconnection, and recovery inform this proposal.

## Runtime primitives, without a predefined management hierarchy

The application would expose a compact, typed vocabulary:

| Primitive | What it records or does |
| --- | --- |
| Session | An agent's identity, context, runtime reference, and current execution |
| Work order | An outcome, scope, accountable session, dependencies, acceptance evidence, and authority |
| Delegate | Create or reuse a suitable session; transfer a scoped work order and its context |
| Ask decision | Address a structured question to the accountable parent and wait durably |
| Answer decision | Record a scoped answer and notify affected sessions |
| Progress report | Record evidence, blockers, and the next expected update |
| Follow-up | Wake supervision when an update or decision is due |
| Complete | Propose completion with evidence against the agreed outcome |

“Manager” would be a role described in a session's brief. The PA could decide
that a complex project needs one, that an existing manager should own a new task,
or that a small task needs only one worker. Roles would not grant permissions.
The runtime would select from owner-approved capability profiles and enforce
limits on depth, active agents, time, and aggregate usage across the whole tree.

A PA allowed to commission code work could delegate to an approved coding-worker
profile while having no coding tools itself. The delegation authority would come
from the owner's grant, not from the PA inventing a more powerful role. No layer
could grant deployments, production access, or purchasing. Hitting a nesting
limit must not cause a coordination-only agent to do the implementation itself.

Each work order would have one accountable parent, with cross-project dependencies
represented separately. This prevents cyclic chains of responsibility. If a
manager disappeared, its parent could adopt or reassign outstanding questions
and work orders without losing their IDs or answers.

The normal escalation path would be worker → its accountable agent → PA → owner,
with intermediate layers omitted when absent. Each recipient would try to answer
from scope, prior decisions, and available evidence before escalating. Reaching
the owner would mean presenting a recommendation and consequences, not forwarding
a raw worker question. Permissions would be enforced on the answer as well as
the original dispatch.

## “What's been assigned to me today? Please make sure it gets done.”

The PA would resolve “today” in the owner's timezone and query assignment events
or recorded assignment changes. Currently assigned, created today, updated today,
and newly assigned today are different facts. If historical assignment events
were unavailable, the PA would state the coverage limit rather than substitute
another timestamp silently.

Once the owner says “make sure it gets done,” the PA would create durable
responsibility for those outcomes. It would check existing owners and active work
before dispatching, reuse appropriate agents, communicate necessary context, and
set expected follow-up times. The model would choose the delegation structure.
Future tracker polling must not interpret a new assignment as permission to start
unrelated work unless the owner established that standing instruction.

The record would preserve what the owner meant by done. A code task might finish
at a reviewed PR ready for the owner; a project whose outcome requires deployment
would remain pending that external step. The PA would not redefine completion to
fit its own permissions or claim a deployed outcome it could not establish.

## Supervision is part of accepting responsibility

Every active work order would carry an expected next update, last contact time,
last substantive progress, current blocker, and evidence references. A manager
could maintain its children's detail while the PA watches the manager's promises.
The PA could inspect descendants when the manager stopped responding. It would
not ask every agent for a full report on every timer tick.

Durable timers would identify missed commitments cheaply. The model would be
invoked when the situation needed interpretation or action. A responsive process,
a fresh “working” message, and meaningful progress are separate signals.

When a team went quiet, the PA's investigation would distinguish these cases:

| Evidence | Proposed action |
| --- | --- |
| Report delivery failed but work continues | Reconnect or reconcile status; keep the work running |
| Agent is alive with specific progress | Agree a revised check-in; avoid unnecessary interruption |
| Agent requests information already decided | Relay the existing decision and track acknowledgement |
| Agent needs technical investigation | Assign that investigation to an appropriate worker |
| Agent was interrupted | Query the runner and resume the known session when supported |
| Agent cannot be recovered | Establish that the old run stopped, then reassign with preserved context and a new attempt ID |
| Work is reported complete | Obtain acceptance evidence, resolve outstanding dependencies, and report the precise completed outcome |
| Scope or priority needs the owner | Prepare the decision, recommendation, and consequences |
| Runner status is unavailable | Record uncertainty and retry with backoff; do not create a duplicate worker |

Recovery would be bounded and auditable. One durable recovery owner and a
generation/version check would prevent the PA and a manager restarting the same
worker simultaneously. A missing heartbeat would not authorize duplicate work.
Pause, cancellation, and revoked scope would be checked before a recovery action.
Repeated failures would eventually become a prepared escalation rather than an
infinite restart loop.

Completion would require the agreed acceptance evidence and disposition of open
work, blockers, and required decisions. The PA could ask a reviewer or manager
to verify an artifact; it would not inspect code or run project tests itself.
After completion, it would retire follow-ups and send one concise closure report
with links and any remaining external action. A tracker label or silent team
alone would not prove the project was complete.

## Dashboard and connectors

The home page would remain a decision inbox. It would also show which outcomes
the PA had accepted, what it was handling now, overdue promises being investigated,
and completed outcomes. An optional agent tree would explain the organization
the PA chose; the owner would not have to design that tree in a workflow editor.

Chat, decisions, recovery history, and operating usage would share stable IDs
across Slack and the dashboard. Tool activity would explain actions and evidence;
it would not promise access to a model's private reasoning. SSE with reconnect
and snapshot recovery would be sufficient initially.

Slack's Socket Mode avoids a public inbound HTTP endpoint, and the community
slack-go library includes Socket Mode support. We would use that Go connector
with explicit identity mapping and restricted operations. Linear would use
typed GraphQL queries and mutations from Go, not a Node service solely to access
its TypeScript SDK.
[Slack Socket Mode](https://docs.slack.dev/apis/events-api/using-socket-mode/),
[slack-go](https://github.com/slack-go/slack),
[Linear API](https://linear.app/developers/graphql).

## The spike that should decide the stack

Before building all views or connectors, I would test the proposed stack with a
fake tracker and two fake worker adapters, one modelling a persistent session and
one an asynchronous remote job. A selected real PA model would choose whether to
delegate directly or introduce a coordination agent. The test would not prescribe
that choice.

It would have to demonstrate:

1. A remembered owner preference resolves a worker question without disturbing
   the owner, across a PA process restart.
2. An unknown tradeoff escalates through the actual responsibility chain and
   arrives as a prepared decision; the answer reaches the original worker.
3. A deliberately silent team is investigated, then resumed, reassigned, or
   accurately reported as complete according to the injected evidence.
4. A crash after an accepted worker launch does not cause a second launch.
5. The PA remains responsive while a manager waits for children or a decision.
6. Scope revocation and tree-wide usage limits prevent further dispatch; no
   agent obtains prohibited tools by changing its role or nesting further.

The model tests would measure behavior, not demand identical delegation trees.
The durable runtime tests would use deterministic fakes and forced failures.
If Restate's replay model or local operation proved awkward in this slice,
Temporal's Go SDK would be the next durability candidate. If Eino's checkpoint
model complicated recovery, compare Grafana's Go SDK or a small provider-SDK loop
without replacing the work-order and decision contracts. We would not introduce
a TypeScript backend to resolve that choice.

No providers were configured, agents launched, or packages installed during this
research. The recommendation rests on documented capabilities; restart behavior,
permission enforcement, package compatibility, and operational overhead still
need the spike.
