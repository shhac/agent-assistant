# Worker controls, diagnostic evidence and obsolete decisions

Captured 2026-09-16. Code-internal follow-up to v0.7.0.

The dashboard exposed worker lifecycle controls that the assistant could not call.
The assistant tried setup or messaging when it needed to resume an existing
assignment. A broker rejecting a message before delivery could also leave a durable
operation marked uncertain, asking the owner to investigate work that never started.

The change gave the current owner conversation inspect_agent and control_agent
(pause/resume/stop), using the existing daemon admission, cooldown, owner-hold and
terminal-stop rules. Operation identity came from the durable chat turn, assignment
and action. Autonomous reasoning could inspect within its scope but could not invoke
owner lifecycle controls. Preparation remained configuration, never recovery.

Confirmed pre-effect refusals became distinct from transport loss, server errors,
timeouts and unknown idempotency conflicts. A confirmed refusal completed the message
operation as refused and refreshed worker status read-only. Ambiguous outcomes stayed
pending; further instructions required reconciliation.

The shared harness added typed failure phase, allowlisted native error codes and
actual subprocess exit status. The broker saved those fields and cleared them after
successful recovery. The UI did not invent a diagnosis for older generic errors.
Raw stderr, provider text, credentials and model reasoning stayed out of metadata.
Synthetic CLI checks established the transport could work; they could not explain a
historical failure whose diagnostic evidence had been discarded.

Decisions gained custom answers and dismissal with an audit reason. Dismissal had no
Answer and no worker-control side effect. A waiting worker stayed waiting; closing an
obsolete question was not permission to execute. Pending counts and decision history
used the same closed-state rule.
