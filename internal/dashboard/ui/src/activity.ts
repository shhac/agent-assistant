import type { Activity } from "./api";
import { stateLabel } from "./states";

/**
 * Readable descriptions for recorded activity kinds. The daemon also records a
 * dynamic `agent.<status>` family, so unknown kinds fall back to a readable
 * phrase rather than leaking the internal identifier into the workspace.
 */
const labels: Record<string, string> = {
  "agent.blocked": "Worker blocked",
  "agent.cancelled": "Worker stopped",
  "agent.completed": "Worker reported complete",
  "agent.control": "Worker control requested",
  "agent.dispatching": "Worker starting up",
  "agent.interrupted": "Worker interrupted",
  "agent.missed_check_in": "Worker missed a check-in",
  "agent.paused": "Worker paused",
  "agent.pause_requested": "Pause requested",
  "agent.queued": "Worker queued",
  "agent.reconciling": "Checking worker state",
  "agent.recovery": "Worker recovery",
  "agent.retry_wait": "Waiting for the model provider",
  "agent.running": "Worker running",
  "agent.started": "Worker started",
  "agent.stop_requested": "Stop requested",
  "agent.usage_wait": "Waiting for worker resources",
  "agent.waiting": "Worker waiting",
  "assistant.review": "Assistant review",
  "assistant.theme": "Workspace palette changed",
  "assistant.update": "Assistant update",
  "coordination.paused": "Dispatch paused",
  "decision.dismissed": "Decision dismissed",
  "decision.opened": "Decision raised",
  "decision.resolved": "Decision answered",
  "demo.created": "Demo workspace created",
  "memory.corrected": "Memory corrected",
  "memory.created": "Remembered something new",
  "memory.forgotten": "Memory forgotten",
  "memory.updated": "Memory updated",
  "operation.acknowledged": "Interrupted operation inspected",
  "operation.interrupted": "Operation interrupted",
  "project.completed": "Project completed",
  "project.created": "Project added",
  "project.directories_updated": "Project folders updated",
  "project.refined": "Project brief refined",
  "recovery.pending": "Recovery needs attention",
  "work_item.accepted": "Outcome accepted",
  "work_item.created": "Outcome recorded",
  "work_item.queue_coordinated": "Queued outcome commissioned",
  "work_item.queued": "Outcome queued",
  "work_item.review_ready": "Outcome ready for review",
  "work_item.steered": "Direction recorded",
  "work_item.unqueued": "Outcome removed from the queue",
  "worker.prepared": "Worker prepared",
};

/** Kinds that report routine runtime progress rather than a change of state. */
const routine = new Set([
  "agent.dispatching",
  "agent.queued",
  "agent.running",
  "agent.waiting",
  "assistant.update",
]);

export function activityLabel(kind?: string): string {
  if (!kind) return "Workspace";
  const known = labels[kind];
  if (known) return known;
  const separator = kind.indexOf(".");
  const rest = separator < 0 ? "" : kind.slice(separator + 1);
  if (kind.slice(0, separator) === "agent" && rest)
    return "Worker " + stateLabel(rest);
  return kind.replaceAll("_", " ").replaceAll(".", " ");
}

export function isRoutineActivity(kind?: string): boolean {
  return !!kind && routine.has(kind);
}

export interface ActivityGroup {
  entry: Activity;
  label: string;
  count: number;
  routine: boolean;
}

/**
 * Collapses an adjacent run of the same routine kind within one project into a
 * single row. Entries are expected newest first; the newest entry of a run
 * represents it, so the count never implies a fresher event than was recorded.
 */
export function groupActivity(entries: Activity[], limit?: number): ActivityGroup[] {
  const groups: ActivityGroup[] = [];
  for (const entry of entries) {
    const previous = groups[groups.length - 1];
    const collapsible =
      previous &&
      previous.routine &&
      isRoutineActivity(entry.kind) &&
      previous.entry.kind === entry.kind &&
      (previous.entry.project_id || "") === (entry.project_id || "");
    if (collapsible) {
      previous.count += 1;
      continue;
    }
    groups.push({
      entry,
      label: activityLabel(entry.kind),
      count: 1,
      routine: isRoutineActivity(entry.kind),
    });
  }
  return limit ? groups.slice(0, limit) : groups;
}
