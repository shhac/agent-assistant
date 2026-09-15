# Local project ownership — 2026-09-15

As of v0.2.1 (code-internal; no external API version to pin), agent-assistant's durable local state owned the project registry, contracts, decisions and evidence. Linear was an optional resource. A local project could progress from directory registration through approved agent execution and acceptance with no external tracker.

Connecting a CLI account had previously enabled assignment import implicitly. This was corrected: each Linear connection had a separate `import_assignments` switch, off by default. The legacy API path required the same explicit opt-in. Old configurations without the switch became query-only; existing imported projects were preserved. A CLI Linear connection continued to supersede legacy imports, including when the CLI connection was query-only, to avoid falling back to a different account.

The assistant's instructions separated account availability from project relevance. A work account was not context for a personal project unless the owner explicitly linked or requested it. This was model guidance, not a per-project connector access-control mechanism; supported read tools still enforced configured account scope. Linear writes were not supported.

Synthetic regression coverage exercised local creation, refinement and coordination without connector calls, query-only accounts, selective enrollment, disabling imports, and older configuration defaults. No test contacted a real account or started a paid model.
