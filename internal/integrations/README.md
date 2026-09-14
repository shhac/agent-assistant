# Integration contracts

These adapters contact external services only when the operator configures and starts them. Tests use synthetic fixtures and local HTTP servers. Credential values are read from environment variables and never returned in adapter errors. Configure services before starting the daemon; the application owns configuration, retries, durable receipts and authorization.

## PA model

`engine.New` accepts a full HTTPS Chat Completions endpoint (for example the configured base URL followed by `/chat/completions`), model identifier, credential environment reference, assistant name and personality. Loopback HTTP supports local compatible providers. The provider must support Chat Completions function tools and `max_completion_tokens`; model compatibility is checked by the first requested conversation, not by spending inference during setup.

The engine sends only coordination tools. The application bridge validates every argument and checks current authority. A model response cannot introduce new tools or increase allowances. `BeforeRequest` reserves a durable model-call allowance before each request. Per-call output tokens, model turns and serialized context size are bounded. The implementation does not automatically retry model HTTP requests: a lost response can still represent billable inference. Usage reports are token counts with an explicit `known` flag, not dollar charges or subscription remaining capacity. The model-call cap is not a dollar budget.

History is server-owned user/assistant dialogue. Tool results and current records are supplied as evidence, not policy. The prompt prohibits deployment, production data access and purchases through all descendants. These constraints also need enforcement by the worker broker, not just prompts.

Reference: [OpenAI function calling](https://developers.openai.com/api/docs/guides/function-calling). This adapter uses the compatible Chat Completions protocol deliberately; it does not require the OpenAI agent runtime or give a model arbitrary built-in tools.

## Linear

Set `linear.api_key_env` to an environment variable containing a Linear personal API key and configure explicit `linear.team_ids`. `Assigned` discovers the key owner's `viewer.id`, then reads active assigned issues in those teams with cursor pagination. OAuth access tokens require the value's `Bearer ` prefix; personal API keys use their raw value. No write scope or write operation is used. The adapter returns an error on partial GraphQL errors or incomplete pagination; callers must not interpret a failed read as an empty assignment list.

`updatedAt` indicates record freshness. It does **not** establish when an issue was assigned. Assignment-time queries require actual assignment history, which this adapter does not yet fetch. Current discovery is assigned issues, not lead-owned projects with no assigned issue.

Reference: [Linear GraphQL API](https://linear.app/developers/graphql).

## Slack

Create a Slack app, enable Socket Mode, create an app-level token with `connections:write`, and grant its bot `chat:write`, `im:history`, and `im:write`. Subscribe to the `message.im` bot event, install the app in the intended workspace, then set the configured bot-token and app-token environment variables. Set `slack.owner_user_id` to the owner's Slack user ID. Reinstall the Slack app after adding scopes.

Only original messages from that owner in a DM enter the PA. Channel messages, bot messages, edits and shared-channel events are ignored. Replies stay in the initiating thread. The SDK manages Socket Mode reconnects; no public webhook ingress is needed. SDK debug logging is disabled. The application must persist event claims before acknowledgement, record completion and surface pending claims after a crash. Pending requests must not be automatically re-run when prior coordination effects are uncertain. Notification calls target only the configured owner; external recipients are not part of this adapter.

Reference: [Slack Socket Mode](https://docs.slack.dev/apis/events-api/using-socket-mode/).

## Approved worker broker

A configured worker endpoint is an **operator-trusted execution broker**, not a public arbitrary webhook. Register only a broker that enforces the assigned scope, runs isolated workers without production credentials, and forbids deployment, production-data access and purchases. This adapter cannot sandbox an independently operated server. The PA itself has no subprocess execution surface.

The broker implements these JSON endpoints under its configured base URL:

| Request | Meaning |
| --- | --- |
| `POST /runs` | Start using a previously persisted dispatch key. |
| `GET /runs?dispatch_key=...` | Reconcile the exact original start. Return one run or 404. |
| `GET /runs/{id}` | Fetch current state and any pending coordination request. |
| `POST /runs/{id}/resume` | Resume the existing run with `{ "instruction": "..." }`. |
| `POST /runs/{id}/messages` | Deliver `{ "message": "..." }` to the run. |
| `POST /runs/{id}/cancel` | Stop the run and descendants; return its resulting state. |

All POST requests carry an `Idempotency-Key`. The broker must durably associate it with the operation before launching or messaging an agent, return the same result for identical retries, and reject reuse with changed contents. `GET` by dispatch key must establish whether the original operation exists. A 404 must never mean merely that the broker is still eventually indexing an already-started run. A transport failure, malformed success or 5xx from a mutation is an uncertain outcome, not permission to launch another agent.

The start JSON includes `dispatch_key`, `agent_id`, `project_id`, optional `parent_id`, `role`, `task`, `acceptance_criteria`, `capabilities`, immutable `prohibitions` and `check_in_deadline`. Managers receive only `coordinate` in executable `capabilities`; `delegation_capabilities` records their allowed descendant scope. A manager must send delegation requests to the daemon rather than start hidden children itself, so aggregate limits and ancestry remain enforced.

Responses are one run object:

```json
{
  "id": "broker-run-123",
  "dispatch_key": "persisted-dispatch-key",
  "status": "running",
  "summary": "Review is underway.",
  "evidence": [],
  "updated_at": "2026-09-14T12:00:00Z"
}
```

Recognized statuses are `queued`, `running`, `waiting`, `blocked`, `interrupted`, `completed`, `failed` and `cancelled`. `updated_at` must advance on substantive progress, not every status read. `completed` is a worker report requiring evidence and application acceptance, not a declaration that the entire project is accepted.

A run can additionally return:

- `decision`: `{request_id, question, recommendation, options: [], why, evidence: []}` for its actual parent to resolve.
- `delegation`: `{request_id, worker_profile, role, task, acceptance_criteria, capabilities: []}` for a manager to commission a child through the daemon.
- `instruction`: `{request_id, target_agent_id, message}` for a manager to address its direct child. The daemon validates ancestry; IDs are not authority.

Request IDs must remain stable across status reads until resolved. They are separate from operation idempotency keys and run IDs. The application deduplicates these requests and delivers decisions with the messages endpoint. Any required request that cannot be resolved within existing authority goes to the PA and finally the owner.
