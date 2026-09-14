# First implementation: local coordination daemon

Date: 2026-09-14. Status: initial implementation. Version: first main-branch build; dependency releases are pinned in go.mod and the dashboard lockfile. This supplements the earlier proposals rather than treating their candidate libraries as already validated decisions.

## Implemented shape

The first build used Go, the family CLI/output libraries, an embedded React/Vite dashboard, and pure-Go SQLite. A bounded Go model loop spoke an OpenAI-compatible function-tool protocol. Slack Socket Mode, scoped Linear GraphQL reads, and an approved HTTP worker-broker protocol sat behind testable adapters. No OpenClaw code was used.

Eino and Restate remained research candidates. For this single-host build, one instance lock and SQLite transactions persisted domain state, dispatch intent, session identity, event receipts, decisions and model-call reservations. It did not require a second server. The model API was narrow enough to implement directly without committing to a framework before measuring a need. This is a simpler initial deployment, not a claim to have validated the proposed Eino/Restate integration.

SQLite currently stores a versioned aggregate snapshot transactionally. It provides restart durability and atomic mutations; it is not an indexed analytical schema or a distributed workflow engine. Growing history and multiple hosts would justify migrations and a re-evaluation of durable workflow infrastructure. Long-lived responsibilities remain records, not a permanently running inference loop.

## Interaction and responsibility

The dashboard starts with Overview, not a date planner. Its name comes from config. A user can record an outcome and acceptance criteria, ask the PA to coordinate, inspect commissioned workers, answer prepared decisions, and correct memory. Dark mode is the primary visual design.

The model may commission direct workers or managers from approved profiles. Go enforces project ownership, parent links, scoped delegation, depth and execution capacity. Waiting managers release capacity for their children. Child progress and questions travel through the parent relationship; the PA handles top-level questions within project-scoped authority before asking the owner.

The local application never provides an implementation shell to the PA. A separate bundled `worker serve` process supports direct coding/review workers inside offline, non-root Docker containers. It requires an owner-supplied, pinned image already present locally and never pulls one. Each run receives a copied nonsecret workspace; the original source stays untouched. The host broker calls the model while file and test tools execute only inside the container. Completion publishes actual content-diff and command artifacts after container shutdown. Its model-call limit is cumulative across resumes and messages. External approved brokers support additional runtimes and manager roles. These broker boundaries enforce isolation and prohibitions outside prompts; development verified the built-in workflow with fake Docker/model services, not a live Docker image or paid provider.

## Recovery and limits

Dispatch intent is persisted before launching work. A timeout or missing run stays uncertain until reconciled. Confirmed interruptions resume the recorded session with a stable operation key and a bounded allowance. Broker timestamps represent progress freshness; polling identical status must not extend a check-in indefinitely.

External messages are durably claimed before sending. If the daemon cannot prove an effect's outcome, it exposes the pending operation instead of replaying it with a new identifier. This is conservative at the crash boundary; it does not claim exactly-once effects from a remote service that lacks that guarantee.

Model requests reserve a durable daily call allowance before invoking the provider, with bounded turns and output tokens. This build does not estimate a bill from calls, claim unknown usage is free, implement monetary budgets, or monitor provider subscription headroom. Workers require their own enforced resource envelopes at the broker boundary.

## Access and verification

The daemon binds loopback and requires identity for private API reads and writes. Local pairing codes are short-lived and single-use. Tailscale Serve uses the family helper with route conflict/ownership guards and an owner identity allowlist. Public Funnel is excluded. Loopback proxy identity assumes a trusted local host account.

Validation covered state restart, scoped authority, capacity and recovery, model tool calls through a fake provider, HTTP project/decision/memory flows, authentication and CSRF, mocked Tailscale ownership, worker protocol behavior, frontend interactions and local browser inspection. Real integration accounts and paid inference were not used for development tests. No production deployment or release tag was created.
