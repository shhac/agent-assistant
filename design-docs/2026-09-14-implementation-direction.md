# Implementation direction

Date: 2026-09-14. Status: owner-approved direction. Nothing to pin: pre-implementation.

The CLI, Go module, and GitHub repository are named `agent-assistant`. The dashboard is dark-mode-first. Its home is an enduring overview of commitments, decisions, and project progress, not a Today/Tomorrow planner. Date-based summaries answer explicit questions.

The assistant name is configurable. A shipped default may exist in exactly one configuration default; UI, prompts, notifications, and worker briefs obtain the resolved name. Historical concept art uses Milo as a placeholder, not a product constant.

The earlier concepts and research remain dated explorations. The visual implementation follows this update where they differ. Go owns deterministic policy, persistence, scheduling, adapters, and HTTP. The PA model makes fuzzy coordination decisions through constrained tools. It never implements project work. Delegation is dynamic, carries authority and shared resource limits, and must preserve accountability through interruption and recovery.

Development proceeds in verified commits on main. Initial delivery should work locally without external accounts, with explicit setup for models, Slack, Linear, and private Tailscale access. Demo data must never be passed off as live integrations. No public network route or live project worker is started by development tests.
