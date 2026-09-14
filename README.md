# agent-assistant

A personal AI assistant that remembers context, coordinates project agents, follows up on stalled work, and brings its owner prepared decisions. A Go daemon and CLI with an embedded, dark-mode-first dashboard. Private Tailscale access is optional.

## Run it

Requires Go 1.26.4 or newer. Node is only needed when developing or rebuilding the dashboard; its compiled assets are included in the repository.

```sh
make build
./agent-assistant serve --demo --open
```

Demo mode uses fictional data and disables external integrations. Without an explicit `--state`, its temporary state is removed on exit. It is useful for trying projects, decisions, memory, navigation, and the dashboard; it does not simulate an AI response.

For your own assistant:

```sh
./agent-assistant init
./agent-assistant config set assistant.name Quill
./agent-assistant config set model.model '<provider-model-id>'
# Set OPENAI_API_KEY in the daemon environment, or change model.api_key_env.
./agent-assistant doctor
./agent-assistant serve --open
```

The model endpoint is configurable and uses the OpenAI-compatible Chat Completions tool-calling protocol. A local model can use a loopback HTTP endpoint; remote endpoints require HTTPS. Choose a model that supports function tools and strict schemas. No model identifier is silently chosen for you.

Configuration defaults to `~/.config/agent-assistant/config.json`; state defaults to `~/.local/state/agent-assistant/state.db`. XDG overrides and explicit `--config` / `--state` flags are supported. Configuration contains credential **environment variable names**, never secret values. Run `config show` to inspect all effective defaults.

```sh
./agent-assistant chat 'What projects need my attention?'
./agent-assistant chat 'Please coordinate the export project against its acceptance criteria.'
./agent-assistant status
./agent-assistant pause
./agent-assistant resume
./agent-assistant dashboard open
```

Only one daemon can own a state file. `serve --no-dispatch` is a fixed boot-time control for observing without starting or resuming workers. Pause stops new coordination actions; it does not terminate already-running external work. Ctrl-C or SIGTERM shuts down the local daemon; saved worker identities are reconciled on the next start.

## Dashboard

The home screen is an enduring **Overview** of outcomes, decisions, and project progress. Projects expose acceptance criteria, responsible agents, evidence, and activity. Decisions show context, a recommendation, and alternatives. Memory is visible and editable. Settings control the assistant identity, model, connection references, and execution limits.

The assistant's default name is defined once in configuration. UI labels, model instructions, and notifications use the resolved value. The older light concept art and date-based examples in the design journal are historical; they do not define the shipped navigation or theme.

Local access uses a single-use, five-minute pairing code exchanged for an HttpOnly session cookie. `--open` passes that code in a URL fragment, which the browser clears immediately. For another browser, use `dashboard open --print`. Private API reads and writes both require authentication. Treat the host account as trusted: another process running as that account can read local credentials.

## Private Tailscale access

Install and log into Tailscale on the existing daemon host, and enable HTTPS for its tailnet. Configure the exact owner identities permitted to open the dashboard:

```sh
./agent-assistant config set dashboard.allowed_users '["owner@example.test"]'
./agent-assistant serve --tailscale serve --tailscale-port 8443 --open
```

The daemon binds loopback, derives the machine's HTTPS address using the family Tailscale helper, and checks route ownership before changing anything. It refuses occupied routes, preserves other services' configuration, and removes its route on clean shutdown only if the route still matches. Background routes left by a crash are reconciled on restart. Public Funnel is not supported. Tailnet access alone does not grant owner authority; the dashboard checks the configured user allowlist.

To make Serve persistent configuration, set `dashboard.tailscale` to `serve`. Network and Slack connection changes require a daemon restart. Routine preferences and limits can change live through Settings or `config set` against the running daemon. The host must remain running for the assistant to stay available.

## Connect your work

**Linear:** set `linear.team_ids` to the teams in scope and provide the environment variable named by `linear.api_key_env`. The daemon periodically reads active assignments for the credential's viewer and imports source-linked outcomes. Discovery does not start agents. An issue's update time is not treated as its assignment time. The current adapter reads assignments; it does not create or modify Linear projects or issues.

**Slack:** configure `slack.owner_user_id` and supply the environment variables referenced by `slack.bot_token_env` and `slack.app_token_env`. Use a Slack app with Socket Mode and direct-message events. Only the configured owner's direct messages trigger the PA. The same conversation and decisions appear in the dashboard. See [integration setup and protocols](internal/integrations/README.md) for scopes and delivery semantics.

**Workers:** this repository includes a usable local coding-worker broker as well as the [external broker protocol](internal/integrations/README.md). The PA can commission a direct worker through the built-in broker, or choose a manager from an external approved profile.

Run the separate broker against one dedicated, nonsecret source workspace and an existing assistant project ID:

```sh
./agent-assistant worker serve --workspace /path/to/dedicated-source --project <project-id> --image <locally-installed-image@sha256:digest> --max-turns 24
```

Supply the broker API token through `AGENT_ASSISTANT_WORKER_TOKEN` in both processes. Register a profile pointing to `http://127.0.0.1:8350` with that credential reference, the same `project_id`, and `implement`/`review` capabilities. See the complete [worker setup guide](docs/worker-broker.md) for image requirements, configuration and results.

The image must be pinned and preinstalled locally; it is never pulled. Workers operate in copied workspaces inside non-root Docker containers with no network, no host credentials or sockets, dropped capabilities and resource bounds. The host Go broker makes model requests; the PA never receives coding tools. The original source remains untouched. Completion includes an actual content patch, command results and a bounded broker-generated evidence digest for PA review. Missing or truncated evidence requires further inspection.

`--max-turns` is the exact cumulative model-call cap for a worker session, including resumes and messages. Output tokens, command duration and per-attempt wall time are also bounded. Stable dispatch keys, saved run IDs and durable receipts preserve recovery without blindly repeating uncertain effects. Built-in worker tests use fake Docker and model services; real Docker/image/provider compatibility has not been exercised during development.

External brokers remain trusted enforcement boundaries. They must provide idempotency, truthful progress timestamps, isolated workspaces and role-specific tools. Managers coordinate through the daemon so descendant scope and shared limits remain enforced.

## Operating boundaries

The PA has no shell, code-writing, deployment, production-data, or purchase tool. Worker commissions carry immutable deployment, production-data-access, and purchase prohibitions. A broker must enforce those outside its prompts. Inference and approved worker execution are permitted operating usage.

The current limits bound concurrent execution, delegation depth, recovery attempts, model turns, output tokens, and durable model-call reservations per UTC day. **A call limit is not a dollar budget or a provider subscription meter.** Provider-enforced monetary caps, account headroom polling, quiet hours/digests, transcript-retention controls, avatar generation, WhatsApp, and phone calls remain follow-on work from the design journal.

Completion requires recorded evidence and no unfinished descendants or unresolved project decisions. The PA reviews that evidence against the recorded acceptance criteria; it does not deploy the result. Uncertain outbound actions are retained for inspection rather than silently repeated. Private state and external provider copies have separate lifetimes; deleting a memory does not erase earlier transcripts or remote copies.

## Development

```sh
npm ci --prefix internal/dashboard/ui
make dashboard
make check
make test-race
make build
```

The Vite bundle in `internal/dashboard/assets/` is committed and embedded in Go. After UI changes, rebuild it. CI checks Go tests/races/vet, frontend tests/types, and bundle freshness. Runtime tests use temporary SQLite files, fake providers, and fake worker brokers. No test needs live Slack, Linear, Tailscale, or paid inference.

Design rationale and earlier concepts are in [design-docs](design-docs/README.md). Implementation choices and current limitations are recorded in the [implementation notes](design-docs/2026-09-14-first-implementation.md).

## License

[PolyForm Perimeter 1.0.0](LICENSE), matching the agent CLI family.
