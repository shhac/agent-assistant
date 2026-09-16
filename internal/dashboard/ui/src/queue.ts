import type { WorkItem } from "./api";

/**
 * Orders queued outcomes so each follows the outcome it waits on. Items whose
 * predecessor is not itself queued come first, since those are the ones that
 * can start next. Any cycle or dangling reference is appended in recorded order
 * rather than dropped: the queue must never hide work it cannot sequence.
 */
export function orderQueue(queued: WorkItem[]): WorkItem[] {
  const remaining = [...queued];
  const queuedIds = new Set(queued.map((item) => item.id));
  const ordered: WorkItem[] = [];
  const placed = new Set<string>();
  while (remaining.length) {
    const index = remaining.findIndex(
      (item) =>
        !item.after_work_item_id ||
        !queuedIds.has(item.after_work_item_id) ||
        placed.has(item.after_work_item_id),
    );
    if (index < 0) break;
    const [next] = remaining.splice(index, 1);
    ordered.push(next);
    placed.add(next.id);
  }
  return [...ordered, ...remaining];
}

/** Whether an outcome is waiting in the queue rather than being worked on. */
export function isQueued(item: WorkItem, hasAttempts: boolean): boolean {
  return item.status === "queued" && !hasAttempts;
}

/**
 * The condition that would let a queued outcome start, in the owner's terms.
 * Commissioning authority and readiness are separate facts, so an authorized
 * item still says what it is waiting for.
 */
export function startCondition(
  item: WorkItem,
  predecessorTitle: string | undefined,
): string {
  if (!item.after_work_item_id)
    return item.commission_requested
      ? "Ready to start; waiting for the assistant to commission it"
      : "Recorded only; arrange a start with your assistant";
  const after = `Waits for ${predecessorTitle || "the preceding outcome"} to be accepted`;
  return item.commission_requested
    ? `${after} · automatic coordination authorized`
    : `${after} · not authorized to start automatically`;
}
