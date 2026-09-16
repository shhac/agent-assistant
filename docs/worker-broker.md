# Run a separate coding worker (advanced)

For normal local use, add a project folder and ask the assistant to prepare a worker. It manages setup, names and private authentication; see the [managed workflow](../README.md#connect-your-work). This guide is for operating a separate broker with a custom toolchain.

The assistant coordinates work; the separate `worker serve` process performs implementation inside an offline Docker container. This repository includes both components. You do not need to write your own execution service to try direct worker delegation. Manager agents can still use an external approved broker; the built-in broker accepts direct `worker` roles with `implement` or `review` capabilities only.

## Prepare the boundary

Use macOS or Linux with a local Docker daemon. Prepare a **dedicated, stable, nonsecret source workspace** and a locally installed Linux image containing `/bin/sh`, basic Unix utilities, the required compilers and preinstalled dependencies. The image must support running as your numeric user ID. Network access is disabled during work, so dependencies must already be in the image or approved source snapshot. The broker never builds or pulls an image, installs dependencies, logs in to a registry, or buys anything.

Supply an immutable image identifier: `sha256:<64 hex digits>` or `repository@sha256:<64 hex digits>`. Startup inspects the local image and resolves it to its immutable image ID. The broker uses `--pull=never` for every container start.

Known sensitive files and directories are excluded from the source copy: `.env*`, `.git`, credential/config directories, key/certificate files, `.npmrc`, `.netrc`, and similar names. Symlinks, device files and sockets are not copied. Source reads use `os.OpenRoot` confinement and do not follow final symlinks. File-name exclusions cannot recognize a secret embedded in ordinary source code; prepare the dedicated source accordingly and keep it stable while it is copied. The original workspace is never mounted or modified.

## Start the broker

Configure the independent `worker_model` profile in Settings or with `config set worker_model.<field>`. Fresh worker profiles use `codex / gpt-5.6-terra / high` with the login in `worker_model.codex_home`. `--engine`, `--model` and `--effort` override that profile for this broker process. The API engine uses `worker_model.base_url` and `worker_model.api_key_env`; the Codex engine uses `worker_model.codex_bin`; Claude uses `worker_model.claude_bin` and `worker_model.claude_home` with its native CLI login. Changing the PA's model does not change a running worker broker.

Set an independently generated broker API token in the environment variable `AGENT_ASSISTANT_WORKER_TOKEN` in both the broker and assistant processes. For the API engine, set the configured model credential variable in the broker process as well. For Codex, configure `worker_model.codex_home` and run `agent-assistant model login --profile worker`. Its default is the same app-owned home as the PA; no environment variable export is needed. Homes containing global AGENTS instruction files are rejected; credentials are never copied from your usual Codex home. Keep actual tokens out of configuration files, command arguments and version control.

```sh
agent-assistant worker serve \
  --workspace /path/to/dedicated-source \
  --project <assistant-project-id> \
  --image <locally-installed-image@sha256:digest> \
  --http 127.0.0.1:8350 \
  --max-turns 24 \
  --max-output-tokens 4096 \
  --max-concurrent 1
```

`--docker-socket` selects a local Unix socket when Docker is not available at `/var/run/docker.sock`. Remote TCP Docker daemons are not supported. `--worker-state` chooses the private state/artifact directory, which must be outside the dedicated source tree. Its default is beside the assistant state file.

Register the endpoint as a worker profile in the assistant configuration, preserving existing profiles:

```json
{
  "id": "local-builder",
  "name": "Isolated builder",
  "project_id": "<assistant-project-id>",
  "endpoint": "http://127.0.0.1:8350",
  "api_key_env": "AGENT_ASSISTANT_WORKER_TOKEN",
  "capabilities": ["implement", "review"]
}
```

The profile lives in the configuration's `workers` array. Its broker accepts only the exact `--project` ID. Use a separate broker/state directory for another project. `agent-assistant status` includes project IDs. The PA can then commission a direct worker when you ask it to coordinate that project.

## What is isolated

Each run gets its own copied workspace. The container has:

- No network, host sockets, host home directory, provider credentials or cloud credentials.
- A read-only root filesystem, dropped Linux capabilities and `no-new-privileges`.
- A non-root numeric user, 1 GiB memory limit, two CPUs and 128-process limit.
- One copied workspace mount and a bounded 256 MiB temporary filesystem. Review-only workers receive a read-only workspace mount; managed npm workers also have a writable per-run dependency mount for temporary test bundles.

Model requests happen on the host through the selected engine. With Codex or Claude, a tool-disabled CLI proposes structured actions that the Go broker checks and executes; it cannot run commands itself. The model sees narrowly defined worker tools: `read_file`, `write_file`, `run_command`, `send_message`, `ask_decision`, and `finish`. File writes and test commands are sent through `docker exec`; command strings are interpreted only by the container's shell. The Docker CLI receives a clean environment without inherited credentials. The PA's model does not receive these implementation tools. `send_message` requests daemon-mediated communication with another active agent in the same project. It yields until the daemon acknowledges delivery, then the sender can continue. The daemon supplies sender identity and marks peer content as untrusted; it never grants new authority. The project roster arrives in the task and subsequent daemon messages. Requests outside the project are rejected.

The owner-supplied image and local Docker daemon are trusted infrastructure. This is container isolation, not a claim of a hardware security boundary against kernel or Docker vulnerabilities.

## Bounds and recovery

`--max-turns` is the **cumulative model-request limit for a worker session**, including resumes, delivered messages and broker restarts. It is never reset by a resume. API output tokens are capped per request. Codex has no per-request token cap here; its process time and output bytes are bounded separately. Reasoning effort is independent of these limits. Each execution attempt also has a 30-minute wall-clock bound; individual commands have a 60-second bound, with at most 128 commands per session. Source/artifact collection is limited to 10,000 source files, 64 MiB total and 2 MiB per file. Model context is limited to 128 KiB and command output to 64 KiB.

These are resource bounds, not a dollar estimate or a promise of free model inference. Model calls can cost money under the configured provider. No paid model call is automatically retried after an uncertain response.

Launch and control requests use durable idempotency keys. Interrupted execution requires the explicit resume endpoint; an ordinary message cannot bypass recovery limits. Resuming keeps the existing copied workspace and baseline. On broker restart, recorded containers must be verified as owned and stopped before any run becomes recoverable. Incomplete tool acknowledgements become explicit uncertainty results, not repeated tool executions. An exhausted cumulative model allowance requires an explicit higher limit from the owner before further model work.

## Review results

A worker's `completed` report is published only after its container has stopped and actual artifacts have been collected under:

```text
<worker-state>/runs/<run-id>/
  workspace/                 # resulting isolated copy
  artifacts/changes.patch    # content changes against the initial copy
  artifacts/commands.json    # commands, success flags and captured output
  artifacts/summary.txt      # changed paths and command count
```

Evidence includes these concrete paths plus a broker-generated digest: changed paths, actual command success/failure counts, failure-first command/output excerpts, and a content-patch hash and excerpt. The PA evaluates those supplied excerpts without receiving arbitrary host-file access. Truncation and omissions are explicit; when an acceptance criterion needs omitted information, the PA must obtain that evidence or escalate for inspection rather than infer success. The patch records text content changes; binary changes are identified by content hashes. It is not a full Git commit and does not encode permission-bit or symlink changes. The resulting workspace remains available for inspection and manual transfer. Nothing is committed, pushed, merged or deployed automatically.

An explicit `finish` tool call supplies the worker's acceptance summary. The PA still compares it with the actual evidence before accepting the project. A successful command is recorded as such; missing tools, failed checks and unresolved questions must remain visible rather than being described as passing tests.

The broker persists at most 1,000 runs in one state directory. Preserve or archive completed state deliberately; automatic artifact deletion is not currently implemented.
