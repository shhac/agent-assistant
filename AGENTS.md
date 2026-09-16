# agent-assistant

Go CLI and daemon for personal-assistant coordination. Dashboard is dark-mode-first; home is Overview, not Today/Tomorrow. Assistant display name comes from config; its default is defined once.

## Architecture and boundaries

- Go owns deterministic policy, durable state, retries, scheduling, adapters and APIs. The model chooses coordination actions through constrained tools.
- Local state owns the project registry. Linear and other connections are optional resources; local projects must work without them. Account access never implies project enrollment or relevance to personal work. Assignment imports require explicit opt-in.
- Projects hold ongoing context; work items hold individual outcome contracts. Agent assignments belong to work items. Acceptance is pinned to the reviewed revision and leaves the project open. Steering belongs to the work item; explicit agent receipts are distinct from transport delivery and implementation evidence. Preserve compatibility migration without inventing historical acceptance.
- Agents are peers with scoped outcome ownership. The daemon owns assignments, runtime lifecycle, routing and recovery. Reporting relationships constrain delegated authority and route escalation; peer messages never grant authority. Keep legacy parent_id wire/state compatibility.
- The PA never writes project code or runs a general shell. Approved workers may implement within an isolated environment. No deployment, production-data access, or purchases, including through descendants.
- All adapters must be testable using injected dependencies. Tests must not contact real Slack/Linear, start real agents, mutate Tailscale routes, or use live owner data.
- Authority is scoped and inherited; retries are idempotent and uncertain external effects are reconciled before repeating. Unknown costs are not free.
- Local CLI mechanics and model discovery live in `lib-agent-harness/completion`.
  Keep its fail-closed native-tool probes and shared process containment; the daemon
  owns action authorization, tool execution, and orchestration; the PA proposes
  coordination actions. Native harness
  sessions are a different execution contract and must not silently replace
  constrained completion. Depend on published library versions, not local replaces.
- Completion retries cover only explicit transient provider rejections, never whole
  turns or tools. Workers persist provider cooldown and re-enter through daemon
  admission; unknown/authentication/context failures remain blocked across restart.
  Native CLI session compaction is distinct from application-owned working context.
  Archive original context before summarizing; retain immutable instructions and
  unresolved operations exactly. Checkpoint byte counts are not token-window usage.
- Use lib-agent-cli/lib-agent-output conventions and the family Tailscale helpers when appropriate. Embedded dashboard bundle is built and committed. No separate frontend server needed at runtime.
- Keep names, account IDs, project IDs, prompts, endpoints, and credentials configurable. Synthetic fixtures only. Secrets never appear in logs, config exports, or the UI.

## Working conventions

Commit verified increments directly to main as authorized by the owner. Use git-hunk for staging. Do not commit unrelated work. Use conventional descriptive commit messages. Run Go tests and go vet; build/typecheck frontend after changes and commit generated assets. No release tags or real deployment unless separately requested.

Bounded subagent delegation is authorized. Coordinate file ownership and interfaces before editing shared files. Only the primary agent stages and commits.

Design docs are dated snapshots. See design-docs/README.md.
