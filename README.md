# agent-assistant

A personal AI assistant that remembers context, coordinates project agents, follows up on stalled work, and brings its owner prepared decisions. A Go daemon and CLI with an embedded, dark-mode-first dashboard. Private Tailscale access is optional.

## Install

```sh
brew install shhac/tap/agent-assistant
```

Homebrew installs Bash, Zsh, and Fish completions automatically. Standalone binaries for macOS, Linux, and Windows are available on the [releases page](https://github.com/shhac/agent-assistant/releases).

For a source build or standalone binary, enable completions in your shell:

```sh
# Bash: add to ~/.bashrc (requires bash-completion).
source <(agent-assistant completion bash)

# Zsh: after compinit in ~/.zshrc.
source <(agent-assistant completion zsh)

# Fish: run once.
agent-assistant completion fish > ~/.config/fish/completions/agent-assistant.fish
```

Create Fish's completions directory first if needed. PowerShell scripts are also available with `agent-assistant completion powershell`. Completions suggest config keys and supported values, assistant/worker login profiles, and configured worker projects/models. They only read local configuration; they never contact the daemon, integrations, or model providers.

## Run it

Building requires Go 1.26.4 or newer. `make build` writes the gitignored `./agent-assistant` binary. Node is only needed when developing or rebuilding the dashboard; its compiled assets are included in the repository. The default model engine requires a compatible Codex CLI installed and logged in on the daemon host.

```sh
make build
./agent-assistant serve --demo --open
```

Demo mode uses fictional data and disables external integrations. Without an explicit `--state`, its temporary state is removed on exit. It is useful for trying projects, decisions, memory, navigation, and the dashboard; it does not simulate an AI response.

For your own assistant:

```sh
./agent-assistant init
./agent-assistant model login
./agent-assistant config set assistant.name Quill
# Fresh configuration defaults to codex / gpt-6-astra / high.
./agent-assistant doctor
./agent-assistant serve --open
```

Choose engine, model and reasoning effort from the installed CLI’s live catalog in **Settings → Assistant and worker models**, independently for the assistant (`model`) and local coding workers (`worker_model`). The assistant defaults to `codex / gpt-6-astra / high`; the built-in downstream worker defaults to `codex / gpt-5.6-terra / high`. External manager brokers own their model selection. Explicit saved profiles are preserved when defaults change.

```sh
./agent-assistant config set model.engine codex
./agent-assistant config set model.model gpt-6-astra
./agent-assistant config set model.effort high
# Worker settings are independent:
./agent-assistant config set worker_model.engine codex
./agent-assistant config set worker_model.model gpt-5.6-terra
./agent-assistant config set worker_model.effort high
```

The `codex` engine uses each profile's `codex_bin` and `codex_home`. Both homes default to `~/.local/state/agent-assistant.paulie.app/codex` (respecting `XDG_STATE_HOME`). Set `model.codex_home` or `worker_model.codex_home` to an absolute path to use an existing dedicated login or separate accounts. Explicit configuration wins over the shell's `CODEX_HOME`: the daemon sets that variable only in each child process, never globally. You do not need to export it before starting the daemon or broker.

`agent-assistant model login` creates the configured directory privately and runs the selected CLI’s own interactive login. Use `--profile worker` for a separate worker home. Codex owns credential storage and refresh; this command never copies tokens. The same account and subscription can be used for both profiles. Login is an owner-invoked CLI command, not a model tool.

Codex currently loads global `AGENTS.md` / `AGENTS.override.md` even when project instructions are disabled. The adapter refuses a home containing those instructions before inference. This is instruction isolation, not a requirement for a second account. Codex proposes structured actions with its built-in tools disabled; Go authorizes and executes permitted coordination tools. Workers send implementation actions through the isolated Docker broker.

If you previously exported `CODEX_HOME`, point both configuration fields at that existing directory to retain its login. Configuration files written before these fields existed receive the app-owned default; the daemon does not silently adopt a shell's unrelated Codex setup.

The `claude` engine uses `claude_bin` and `claude_home`, defaults to the native `~/.claude` login, and keeps Claude’s credential storage and refresh in the CLI. Both assistant and worker profiles use this same home by default. A custom home selects a separate login; `model login --profile worker` uses Claude’s subscription login flow when that profile selects Claude. Ambient API keys are not forwarded to either CLI. Configure a managed worker’s optional `model_profile` only when it needs its own complete model/account profile; otherwise all managed workers use `worker_model`.

Normal operation uses local Codex or Claude CLI authentication and its billing arrangement. Separate per-project homes are unnecessary: project code runs in containers that cannot access the host CLI homes. No credentials are copied. API providers remain an explicit advanced option and are never selected automatically after a CLI error.

The `openai-compatible` engine uses Chat Completions with `reasoning_effort`. Configure `base_url`, `api_key_env` and an exact provider model that supports function tools and strict schemas. Remote endpoints require HTTPS; loopback HTTP is allowed for local providers. Astra's native API tool calling requires Responses, so use the Codex engine for this default. Unsupported selections fail rather than silently substituting another model.

Reasoning effort is separate from execution limits. API engines enforce `max_tokens` through `max_completion_tokens`. Codex does not expose a per-request output-token cap here; process time/output bounds and the daemon's model-turn/call limits apply instead. The configured Codex home's saved login selects the account used for inference; API credential references are not required for this engine. This integration never copies login tokens between homes.

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

## Add an existing project

In **Projects → Add project**, choose **Existing folders**, browse the daemon host, and select one or more directories. You can also paste an absolute host path. The project name starts from the folder name and remains editable. An objective or acceptance criteria can be added later; registering a folder does not start agents. Before commissioning implementation, the assistant must establish a measurable acceptance contract.

Project details show the linked directories and let you change them. The same project can cover several repositories or folders. Paths are validated as existing directories, resolved to their canonical absolute locations, and retained across restarts. Registering a project never creates assistant files inside its source directories or grants a worker new filesystem permissions. Approved worker brokers still enforce their own workspace scope.

Each project has a private scratch directory under `<state-directory>/projects/<project-id>`. Assistant model invocations use temporary directories under `<state-directory>/model-runs`, removed after the invocation. Both follow the selected `--state` location. Linked source directories remain project references, never the assistant's working directory.

The reusable file/folder picker browses the **daemon host**, including over Tailscale. It supports single or multiple files/folders, keyboard navigation, hidden entries, and direct path entry. The server returns one directory level as paginated metadata; the browser renders only visible rows. It does not upload files or read file contents. Browsing requires the same owner authentication as the dashboard and is unavailable in demo mode.

## Dashboard

The home screen is an enduring **Overview** of outcomes, decisions, and project progress. Projects expose acceptance criteria, responsible agents, evidence, and activity. Decisions show context, a recommendation, and alternatives. Memory is visible and editable. Settings control the assistant identity, model, connection references, and execution limits.

The default palette is Graphite + sage, with Ink + soft blue and Charcoal + warm amber alternatives in Settings. Expand chat to give the conversation most of the workspace; pending agent/tool work has an animated indicator that respects reduced-motion preferences.

Use **Settings → Assistant setup** for a short conversation about working style and visual preferences. The model recommends a name, personality, theme, and geometric vector avatar. Preview the recommendation before applying it; setup never changes project permissions or connects accounts. The interview survives a daemon restart. This avatar is generated from a constrained shape and palette, not an image-model illustration.

The assistant's default name is defined once in configuration. UI labels, model instructions, and notifications use the resolved value. The older light concept art and date-based examples in the design journal are historical; they do not define the shipped navigation or theme.

Local access uses a single-use, five-minute pairing code exchanged for an HttpOnly session cookie. `--open` passes that code in a URL fragment, which the browser clears immediately. For another browser, use `dashboard open --print`. Private API reads and writes both require authentication. Treat the host account as trusted: another process running as that account can read local credentials.

### Conversation queue and activity

Send another message while the assistant is working to queue it. Accepted messages are stored by the daemon and run in order, even if the browser closes. Queued messages can be cancelled before they start. The conversation displays each turn’s tool activity as it begins and finishes, using friendly labels without raw arguments or results. A daemon interruption marks the active turn as interrupted instead of replaying its actions; messages that had not started remain queued.

**Settings → Conversation** controls personalized loading phrases. By default, one small local CLI request uses the current message and the previous dialogue message: Luna with low effort for Codex, or Haiku for Claude, using the assistant’s configured login. Where the CLI reports no effort dial, the model’s native default applies. A different model can be selected from the CLI catalog. Captions have no tools, receive bounded context, and count toward the existing daily model-call limit. They are decorative text; tool cards carry actual action status. Generation is cancelled when the turn ends and failures keep the static loading message. API-only assistant configurations use static loading text.

The config keys are `chat.loading_phrases.enabled`, `chat.loading_phrases.model` (empty selects the small default), and `chat.loading_phrases.effort`. Disable captions to avoid the extra model request. Message delivery retries reuse a client-generated ID, so retrying an unconfirmed submission cannot enqueue it twice.

## Private Tailscale access

Install and log into Tailscale on the existing daemon host, and enable HTTPS for its tailnet. Configure the exact owner identities permitted to open the dashboard:

```sh
./agent-assistant model login # Once, if not already signed in.
./agent-assistant config set dashboard.allowed_users '["owner@example.test"]'
./agent-assistant serve --http 127.0.0.1:8340 --tailscale serve --tailscale-port 8443 --open
```

Port **8443 is the private Tailscale HTTPS port**; local HTTP remains on **127.0.0.1:8340**. Use your actual Tailscale login in `allowed_users`. The daemon prints the resulting `https://<machine>.<tailnet>.ts.net:8443` address.

The daemon binds loopback, derives the machine's HTTPS address using the family Tailscale helper, and checks route ownership before changing anything. It refuses occupied routes, preserves other services' configuration, and removes its route on clean shutdown only if the route still matches. Background routes left by a crash are reconciled on restart. Public Funnel is not supported. Tailnet access alone does not grant owner authority; the dashboard checks the configured user allowlist.

To make Serve persistent configuration, set `dashboard.tailscale` to `serve`. Network and Slack connection changes require a daemon restart. Routine preferences and limits can change live through Settings or `config set` against the running daemon. The host must remain running for the assistant to stay available.

## Connect your work

Projects live in agent-assistant's local state. Linear, Notion, Slack and other connections are optional resources, not the project registry. To manage a local codebase, choose **Add project → Existing folder**, select its directory, and ask the assistant to coordinate it. No Linear account, issue, or project is required. Worker execution still requires an approved broker. A connected work account does not make it relevant to a personal project; the assistant should use only resources you requested or linked to that project's context.

**Optional Linear imports:** connecting `lin` enables read-only queries without importing projects. Enable **Import assigned issues as projects** on a specific connection only if you want that account's assignments enrolled automatically (`"import_assignments": true`; default `false`). Split work and personal accounts into separate named connections when only one should import. Disabling import stops future polling/imports and preserves projects already recorded. Explicit assignment queries never enroll projects by themselves.

**CLI connections:** select existing accounts from `lin`, `agent-slack`, and `agent-fathom` in Settings. Each connection has a name and an explicit list of allowed accounts. Slack uses the workspace aliases from `agent-slack auth list`, passed to queries through `--workspace`; Fathom uses profiles. The assistant names the connection and selected account for each query. Credentials stay with the CLI. The daemon provides bounded read operations rather than arbitrary command execution. Slack search can use the broader access of your existing `agent-slack` account independently of the bot.

**Notion:** add an `agent-notion` connection without choosing a profile (`"profiles": []`, or omit the field). Search, page reads, and block reads use the CLI's current default account and native authentication, including its native environment credentials. The assistant never switches that default. Changing the default in `agent-notion` changes the account this connection reads. Named Notion profiles are rejected because the CLI cannot select them per call. Account discovery reads local metadata, not project or message content. Slack aliases with unavailable stored credentials remain visible with a re-authentication hint; configure credentials through the CLI on the daemon host.

**Slack bot:** configure `slack.owner_user_id` and supply the environment variables referenced by `slack.bot_token_env` and `slack.app_token_env`. Use a Slack app with Socket Mode and direct-message events. Only the configured owner's direct messages trigger the PA. The same conversation and decisions appear in the dashboard. The bot remains a separate connection from CLI querying. See [integration setup and protocols](internal/integrations/README.md) for scopes and delivery semantics.

**Legacy Linear API:** set `linear.import_assignments` to `true` as well as explicit `linear.team_ids` and `linear.api_key_env` to enable assignment discovery. Omitted import flags stay off on upgrade, including older configurations. New setups should use the `lin` connection; configuring one supersedes legacy API discovery. Discovery does not start agents and an issue's update time is not its assignment time.

**Workers:** add an existing folder to a project and describe what you want to do next, in chat or on the project page. The assistant can prepare its worker, register the private connection, commission the agreed outcome, and follow progress. Preparation creates no project run by itself. The project page also offers **Prepare worker**; external broker fields are under advanced settings.

On macOS, automatic setup uses an existing local container runtime or a dedicated Colima instance. With Homebrew available it installs the free runtime tools as needed, without changing your current Docker context. On Linux, a working local Docker daemon is required. Missing host prerequisites produce a specific blocker; setup never purchases services or creates paid cloud resources. Windows can use an external broker.

The daemon builds a fixed Go/Node toolchain without project source as its build context and records the resulting image digest. Public Go and npm dependencies can be prepared from sanitized manifests without project scripts or host credentials. Private registries, local/git dependencies, npm workspaces, and install scripts need an explicitly prepared environment; automatic setup reports these limitations. Other languages need a custom toolchain through an external broker.

Workers use copied project workspaces inside non-root containers with no network, no host credentials or sockets, dropped capabilities and resource bounds. Git-ignored files and common secret files are excluded. The host Go broker invokes the configured local model CLI; the PA never receives coding tools. The original source remains untouched. Completion includes a patch, command results, and a bounded evidence digest for PA review. Missing or truncated evidence requires further inspection.

Managed state and recovery records live under the daemon’s state directory. The private loopback endpoint and its random token are owned by the daemon; users do not enter or share them. All managed workers share the configured worker CLI login by default. Existing external services remain supported through the [worker broker guide](docs/worker-broker.md). Worker model changes require a daemon restart after a broker has started.

`--max-turns` on a manually operated broker is its cumulative model-call cap, including resumes and messages. Output, command duration and per-attempt wall time are bounded. Stable dispatch keys and durable receipts prevent blind retries of uncertain effects. Tests use fake CLIs, runtimes, providers, and brokers; a full real-runtime installation and paid worker run have not been exercised during development.

External brokers remain trusted enforcement boundaries. They must provide idempotency, truthful progress timestamps, isolated workspaces and role-specific tools. Managers coordinate through the daemon so descendant scope and shared limits remain enforced.

## Worker subscription limits

**Settings → Capacity and supervision → Worker subscription usage** controls when to hold new worker starts, resumes, and follow-up instructions. Codex and Claude default to **90% consumed**: reaching the threshold in any applicable short or weekly quota window holds new work, while existing workers continue. Queued work is retried when usage falls below the threshold. The gate uses the worker’s configured CLI login, including any per-worker profile, rather than assuming it shares the assistant’s account.

The equivalent configuration is:

```json
{
  "limits": {
    "worker_usage": {
      "codex_max_used_percent": 90,
      "claude_max_used_percent": 90,
      "on_unavailable": "allow"
    }
  }
}
```

Set an engine’s threshold to `0` to disable its gate. `on_unavailable` defaults to `allow`, so a missing or failed meter does not stop work; choose `pause` to hold new work until usage can be checked. External broker accounts cannot be inspected locally and follow that unavailable-usage policy. Usage is cached for up to one minute; the provider may also cache its report. These are admission limits, not a hard cap: already-running workers and assistant chat can continue consuming usage.

## Operating boundaries

The PA has no shell, code-writing, deployment, production-data, or purchase tool. Worker commissions carry immutable deployment, production-data-access, and purchase prohibitions. A broker must enforce those outside its prompts. Inference and approved worker execution are permitted operating usage.

The current limits bound concurrent execution, delegation depth, recovery attempts, model turns and durable model-call reservations per UTC day. API output-token caps and Codex process bounds are described above. **A call limit is not a dollar budget or a provider subscription meter.** Provider-enforced monetary caps, quiet hours/digests, transcript-retention controls, raster avatar generation, WhatsApp, and phone calls remain follow-on work from the design journal.

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

## Releasing

Commit the implementation and rebuilt dashboard bundle, then run:

```sh
make release VERSION=vX.Y.Z
git tag vX.Y.Z
git push origin main vX.Y.Z
```

The release check requires a clean tree and an unused local and remote tag, rebuilds the dashboard, and runs Go tests/races/vet and frontend tests/types. The tag workflow repeats CI before invoking the shared Homebrew-tap release workflow. CI builds the standalone binaries, publishes their SHA-256 checksums and GitHub release, and updates the formula with completions. Do not package or upload release artifacts manually.

The tap's write deploy key is the `TAP_DEPLOY_KEY` secret in this repository's `homebrew-tap` GitHub environment. That environment allows only tags matching `v*`; branch and PR jobs do not use it. The release workflow also checks the version format before entering the shared release job. Keep the key out of repository-level secrets, local config, and checked-in files. Its temporary provisioning files were deleted after upload.

## License

[PolyForm Perimeter 1.0.0](LICENSE), matching the agent CLI family.
