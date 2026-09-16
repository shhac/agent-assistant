# Durable work, steering and acceptance — 2026-09-16

Status: implementation decision for v0.6. Code-internal behavior; external reference pinned below.

The project originally held both an ongoing area of responsibility and a single execution contract. Completing an assignment could therefore close the whole project, while replacing its agent could strand the owner's direction in a session. We separated these lifetimes.

A project retained its directories, resources and execution configuration. A work item held one requested outcome and measurable acceptance criteria. Existing agent records remained daemon-owned execution assignments, now linked to the work item. The assistant could choose direct specialists or a coordinator without changing that data model. Work-item acceptance left the project open.

Steering became durable work-item data with a caller-supplied idempotency key. The daemon replayed that context for replacements and recovery, and routed new direction to active attempts. Explicit receipts said an agent had considered a message; transport success said only that a broker received it. Neither was implementation evidence. Acceptance required a completed attempt to acknowledge each message, so replacing a cancelled attempt did not leave an impossible receipt requirement.

Acceptance used a digest of the reviewed contract, attempts, recorded evidence, item decisions and steering. The daemon compared the expected digest atomically before writing the acceptance record. Poll timestamps did not change the digest. Readiness still required completed evidence, no unfinished attempts and no applicable unresolved decisions. Existing completed projects were migrated as historical completions without invented acceptance records.

The dashboard placed criteria, attempts, evidence and steering beside each work outcome. The assistant remained the primary way to define and coordinate work. Prepared decisions showed recommendations and alternatives on Overview.

These choices were informed by Arnold's separation of tickets from execution leases, durable steering and exact-revision review gates:

- [Types](https://github.com/tinyorbit-ai/arnold/blob/8c547d324d8367ce16e82557305040b1899e9388/types.ts)
- [Steering](https://github.com/tinyorbit-ai/arnold/blob/8c547d324d8367ce16e82557305040b1899e9388/steering.ts)
- [Workflow](https://github.com/tinyorbit-ai/arnold/blob/8c547d324d8367ce16e82557305040b1899e9388/workflow.ts)
- [Product principles](https://github.com/tinyorbit-ai/arnold/blob/8c547d324d8367ce16e82557305040b1899e9388/PRODUCT.md)

No Arnold code was imported. Its fixed delivery pipeline was not adopted: agent-assistant continued to let the PA resolve routine choices within established authority, use peer communication through the daemon, and escalate only genuine owner decisions. Persistent Slack delivery and richer execution workspaces remained separate future work.

Verification used synthetic stores, brokers, providers and browser tests. No live owner projects, worker sessions, external accounts or production data were changed.
