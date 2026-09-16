# Scoped peers coordinated through the daemon

Date: 2026-09-16. Version: code-internal, following v0.4.0.

## Decision

We framed project agents as peers that own narrower outcomes. The personal assistant coordinated the owner's commitments; project coordinators handled a project or workstream; specialists owned bounded implementation, review or research assignments. A small outcome could go straight to a specialist. Roles did not imply a fixed agent hierarchy.

The daemon remained the control plane: it stored assignments and authority, started and recovered sessions, routed messages and decisions, applied quota/capacity checks, and reconciled uncertain effects. Model turns proposed actions through tools. Returning from an assistant turn did not terminate a commissioned assignment.

We separated communication from authority. A peer could send a same-project message through the daemon without becoming the recipient's manager. The daemon supplied sender identity; message content was untrusted evidence or a request, never a permission grant. Privileged coordination instructions retained their narrower responsible-coordinator check. Questions still escalated to the responsible coordinator, then the personal assistant, then the owner when the available context and authority could not resolve them.

## Compatibility and limits

We kept `parent_id` in stored records and existing tool/API payloads. It represented the delegation authority and escalation link, not operating-system process ownership. Its existing depth, capability-inheritance and unfinished-work checks stayed in force. Removing that link would have removed accountability and authority bounds without adding useful peer communication.

We added a separate broker peer-message surface instead of widening the privileged instruction route. This avoided treating a sibling's request as a manager's instruction. The built-in coding broker remained a specialist runtime; this change did not make it a full project-coordinator runtime or give it host access.

The model received a project-scoped roster capped at 64 peers and 8 KiB. Unavailable recipients produced a not-delivered response, so an agent could not hold the only execution slot while waiting for a queued recipient to start. Membership changes became visible when the daemon next started, resumed or messaged a session; there was no new autonomous roster polling loop. This was bounded message delivery through the existing durable event/reconciliation machinery, not an unlimited chat network or a guaranteed exactly-once external effect. A process interruption with an uncertain delivery still required reconciliation.

The dashboard exposed responsibility using names, including the escalation contact. Existing worker profiles still selected execution environments, capabilities and model settings; a profile was not an assignment or proof of running work.
