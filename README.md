# agent-assistant

A personal AI assistant that remembers context, coordinates project agents, follows up on stalled work, and brings its owner prepared decisions. A Go daemon and CLI with an embedded, dark-mode-first dashboard. Private Tailscale access is optional.

## Run it

Building requires Go 1.26.4 or newer. `make build` writes the gitignored `./agent-assistant` binary. Node is only needed when developing or rebuilding the dashboard; its compiled assets are included in the repository. The default model engine requires a compatible Codex CLI installed and logged in on the daemon host.

```sh
make build
./agent-assistant serve --demo --open
```

Demo mode uses fictional data and disables external integrations. Without an explicit `--state`, its temporary state is removed on exit. It is useful for trying projects, decisions, memory, navigation, and the dashboard; it does not simulate an AI response.

For your own assistant:

```sh
export CODEX_HOME="$HOME/.local/state/agent-assistant.paulie.app/codex"
codex login
./agent-assistant init
./agent-assistant config set assistant.name Quill
# Fresh configuration defaults to codex / gpt-6-astra / high.
./agent-assistant doctor
./agent-assistant serve --open
```

Choose engine, model and reasoning effort in **Settings → Assistant and worker models**, independently for the assistant (`model`) and local coding workers (`worker_model`). The assistant defaults to `codex / gpt-6-astra / high`; the built-in downstream worker defaults to `codex / gpt-5.6-terra / high`. External manager brokers own their model selection. Explicit saved profiles are preserved when defaults change.

```sh
./agent-assistant config set model.engine codex
./agent-assistant config set model.model gpt-6-astra
./agent-assistant config set model.effort high
# Worker settings are independent:
./agent-assistant config set worker_model.engine codex
./agent-assistant config set worker_model.model gpt-5.6-terra
./agent-assistant config set worker_model.effort high
```

The `codex` engine uses the configured `codex_bin` and the saved login in the daemon's `CODEX_HOME`. Use a dedicated persistent Codex home as shown above. Keep the same environment when launching the daemon and local worker broker. Codex proposes structured actions with its built-in tools disabled; the Go daemon authorizes and executes the allowed coordination tools. Local coding workers use the same model transport but send their file and command actions through the isolated Docker broker. Account access and usage limits still apply.

Codex currently loads global `AGENTS.md` / `AGENTS.override.md` even when project instructions are disabled. The adapter refuses a Codex home containing nonempty global instruction files before inference. It leaves your usual Codex setup untouched; the dedicated home isolates the PA without weakening its tool boundaries. This is instruction isolation, not a requirement for a second account or subscription: you may sign into the same account there. The home must persist so Codex can manage its own login refresh. We do not copy credentials or modify your normal Codex home.

The `openai-compatible` engine uses Chat Completions with `reasoning_effort`. Configure `base_url`, `api_key_env` and an exact provider model that supports function tools and strict schemas. Remote endpoints require HTTPS; loopback HTTP is allowed for local providers. Astra's native API tool calling requires Responses, so use the Codex engine for this default. Unsupported selections fail rather than silently substituting another model.

Reasoning effort is separate from execution limits. API engines enforce `max_tokens` through `max_completion_tokens`. Codex does not expose a per-request output-token cap here; process time/output bounds and the daemon's model-turn/call limits apply instead. The selected Codex home's saved login selects the account used for inference; API credential references are not required for this engine. This integration never copies login tokens between homes.

Existing configuration with a `model` section but no `engine` keeps the previous API engine and provider. A missing `worker_model` in that legacy configuration inherits its previous assistant API profile once on load. To switch an existing setup, set the engine/model/effort explicitly using the commands above. Changes to the assistant profile apply to subsequent requests; restart a running worker broker to use its changed profile.

Configuration defaults to `~/.config/agent-assistant.paulie.app/config.json`; state defaults to `~/.local/state/agent-assistant.paulie.app/state.db`. XDG overrides and explicit `--config` / `--state` flags are supported. Existing installations keep their legacy `agent-assistant` config/state pair when only that namespace exists; mixed namespaces are reported rather than silently combining or moving files. `config path` shows the selected location. Configuration contains credential **environment variable names**, never secret values. Run `config show` to inspect all effective defaults.

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

The default palette is Graphite + sage, with Ink + soft blue and Charcoal + warm amber alternatives in Settings. Expand chat to give the conversation most of the workspace; pending agent/tool work has an animated indicator that respects reduced-motion preferences.

Use **Settings → Assistant setup** for a short conversation about working style and visual preferences. The model recommends a name, personality, theme, and geometric vector avatar. Preview the recommendation before applying it; setup never changes project permissions or connects accounts. The interview survives a daemon restart. This avatar is generated from a constrained shape and palette, not an image-model illustration.

The assistant's default name is defined once in configuration. UI labels, model instructions, and notifications use the resolved value. The older light concept art and date-based examples in the design journal are historical; they do not define the shipped navigation or theme.

Local access uses a single-use, five-minute pairing code exchanged for an HttpOnly session cookie. `--open` passes that code in a URL fragment, which the browser clears immediately. For another browser, use `dashboard open --print`. Private API reads and writes both require authentication. Treat the host account as trusted: another process running as that account can read local credentials.

## Private Tailscale access

Install and log into Tailscale on the existing daemon host, and enable HTTPS for its tailnet. Configure the exact owner identities permitted to open the dashboard:

```sh
export CODEX_HOME="$HOME/.local/state/agent-assistant.paulie.app/codex"
# Run codex login with this CODEX_HOME once, if not already signed in.
./agent-assistant config set dashboard.allowed_users '["owner@example.test"]'
./agent-assistant serve --http 127.0.0.1:8340 --tailscale serve --tailscale-port 8443 --open
```

Port **8443 is the private Tailscale HTTPS port**; local HTTP remains on **127.0.0.1:8340**. Use your actual Tailscale login in `allowed_users`. The daemon prints the resulting `https://<machine>.<tailnet>.ts.net:8443` address.

The daemon binds loopback, derives the machine's HTTPS address using the family Tailscale helper, and checks route ownership before changing anything. It refuses occupied routes, preserves other services' configuration, and removes its route on clean shutdown only if the route still matches. Background routes left by a crash are reconciled on restart. Public Funnel is not supported. Tailnet access alone does not grant owner authority; the dashboard checks the configured user allowlist.

To make Serve persistent configuration, set `dashboard.tailscale` to `serve`. Network and Slack connection changes require a daemon restart. Routine preferences and limits can change live through Settings or `config set` against the running daemon. The host must remain running for the assistant to stay available.

## Connect your work

**CLI connections:** select existing accounts from `lin`, `agent-slack`, `agent-notion`, and `agent-fathom` in Settings. Each connection has a name and an explicit list of allowed profiles; the assistant names the connection and profile for every query. Credentials stay with the CLI. The daemon provides bounded read operations rather than arbitrary command execution. Slack search can use the broader access of your existing `agent-slack` account independently of the bot.

The installed `agent-notion` supports workspace discovery but lacks per-command workspace selection. Notion queries are unavailable until the CLI can select the requested workspace reliably; the assistant never switches the CLI's global default. Other connectors can use multiple explicitly selected profiles. Account discovery is a local credentials-metadata read, not a request for project or message content.

**Slack bot:** configure `slack.owner_user_id` and supply the environment variables referenced by `slack.bot_token_env` and `slack.app_token_env`. Use a Slack app with Socket Mode and direct-message events. Only the configured owner's direct messages trigger the PA. The same conversation and decisions appear in the dashboard. The bot remains a separate connection from CLI querying. See [integration setup and protocols](internal/integrations/README.md) for scopes and delivery semantics.

**Legacy Linear API:** existing explicit `linear.team_ids` and `linear.api_key_env` configurations remain supported for assignment discovery. New setups should use the `lin` connection; configuring one supersedes legacy API discovery. Discovery does not start agents and an issue's update time is not its assignment time.

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

The current limits bound concurrent execution, delegation depth, recovery attempts, model turns and durable model-call reservations per UTC day. API output-token caps and Codex process bounds are described above. **A call limit is not a dollar budget or a provider subscription meter.** Provider-enforced monetary caps, account headroom polling, quiet hours/digests, transcript-retention controls, raster avatar generation, WhatsApp, and phone calls remain follow-on work from the design journal.

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
