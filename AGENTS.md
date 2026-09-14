# agent-assistant

Go CLI and daemon for personal-assistant coordination. Dashboard is dark-mode-first; home is Overview, not Today/Tomorrow. Assistant display name comes from config; its default is defined once.

## Architecture and boundaries

- Go owns deterministic policy, durable state, retries, scheduling, adapters and APIs. The model chooses coordination actions through constrained tools.
- The PA never writes project code or runs a general shell. Approved workers may implement within an isolated environment. No deployment, production-data access, or purchases, including through descendants.
- All adapters must be testable using injected dependencies. Tests must not contact real Slack/Linear, start real agents, mutate Tailscale routes, or use live owner data.
- Authority is scoped and inherited; retries are idempotent and uncertain external effects are reconciled before repeating. Unknown costs are not free.
- Use lib-agent-cli/lib-agent-output conventions and the family Tailscale helpers when appropriate. Embedded dashboard bundle is built and committed. No separate frontend server needed at runtime.
- Keep names, account IDs, project IDs, prompts, endpoints, and credentials configurable. Synthetic fixtures only. Secrets never appear in logs, config exports, or the UI.

## Working conventions

Commit verified increments directly to main as authorized by the owner. Use git-hunk for staging. Do not commit unrelated work. Use conventional descriptive commit messages. Run Go tests and go vet; build/typecheck frontend after changes and commit generated assets. No release tags or real deployment unless separately requested.

Bounded subagent delegation is authorized. Coordinate file ownership and interfaces before editing shared files. Only the primary agent stages and commits.

Design docs are dated snapshots. See design-docs/README.md.
