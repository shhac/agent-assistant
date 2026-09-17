/**
 * One vocabulary for recorded execution state.
 *
 * The daemon records work-item statuses and worker statuses, and the attention
 * rollup reports whichever is answering for a project — so a single name per
 * state is what keeps one worker from being called three different things on
 * one screen.
 */
const labels: Record<string, string> = {
  // Work item
  ready: "Ready to coordinate",
  queued: "Queued next",
  active: "In progress",
  review: "Ready for acceptance",
  accepted: "Accepted",
  legacy_completed: "Previously completed",
  // Worker
  running: "Running",
  dispatching: "Starting up",
  resuming: "Resuming",
  retry_wait: "Waiting for the model provider",
  usage_wait: "Waiting for worker resources",
  reconciling: "Checking worker state",
  completed: "Reported complete",
  // Shared
  waiting: "Waiting",
  interrupted: "Interrupted",
  blocked: "Blocked",
  paused: "Paused",
  pause_requested: "Pausing",
  stop_requested: "Stopping",
  cancelled: "Stopped",
};

/**
 * The longer form used where there is room to say what the state is waiting
 * for. Only owner-requested controls have one: the distinction between asking
 * for a stop and the worker confirming it is the whole point of those states.
 */
const details: Record<string, string> = {
  pause_requested: "Pause requested · waiting for current operation",
  stop_requested: "Stop requested · waiting for cleanup",
  usage_wait: "Waiting for worker resources · saved work is preserved",
};

export function stateLabel(state: string): string {
  return labels[state] || state.replaceAll("_", " ");
}

export function stateDetail(state: string): string {
  return details[state] || stateLabel(state);
}

/**
 * States where the work is not moving and was not stopped on purpose. This is
 * exactly the set the daemon reports a recovery posture for, so
 * `attention.recovery !== ""` is the same question asked of a rollup row.
 *
 * `usage_wait` is deliberately absent. Waiting for an account allowance that
 * resets on a known schedule is ordinary waiting: nothing failed, and the
 * daemon continues by itself. Only the variant that needs an owner decision
 * counts, which is why `needsAttention` takes the assignment rather than a
 * bare state.
 */
const heldUp = ["blocked", "interrupted", "reconciling", "retry_wait"];

/** Controls the owner asked for; the work stopping is the intended outcome. */
const ownerControl = ["paused", "pause_requested", "stop_requested"];

export function isHeldUp(state: string): boolean {
  return heldUp.includes(state);
}

/**
 * Whether an assignment is worth highlighting and opening by default: either it
 * is held up, or the owner has a control in flight they will want to watch.
 */
export function needsAttention(state: string, ownerDecision = false): boolean {
  return isHeldUp(state) || ownerControl.includes(state) || ownerDecision;
}

/**
 * Whether a rollup row is holding up its outcome. The daemon already decides
 * whether a wait clears by itself, and says so by reporting `recovery: "held"`,
 * so a row is read rather than re-derived from its state name.
 */
export function attentionHeldUp(item: {
  execution: string;
  recovery?: string;
}): boolean {
  // A stop the owner asked for is the intended outcome, never something
  // holding them up, whatever else the row carries.
  if (ownerControl.includes(item.execution)) return false;
  return isHeldUp(item.execution) || item.recovery === "held";
}

/**
 * Whether a resource wait is one the owner has to act on. A budget that has to
 * be raised, or consumption that was never reported, is their decision; a full
 * subscription window that resets on its own is not.
 */
export function awaitsOwnerResources(agent: {
  status: string;
  resource_hold_owner_action?: boolean;
}): boolean {
  return (
    agent.status === "usage_wait" && agent.resource_hold_owner_action === true
  );
}

/**
 * How much an assignment has consumed, phrased so an unmeasured call is never
 * silently counted as nothing. Returns null when there is nothing to report.
 */
export function usageLabel(agent: {
  usage_input_tokens?: number;
  usage_output_tokens?: number;
  usage_unknown_calls?: number;
  token_budget?: number;
}): string | null {
  const used =
    (agent.usage_input_tokens ?? 0) + (agent.usage_output_tokens ?? 0);
  const unknown = agent.usage_unknown_calls ?? 0;
  const budget = agent.token_budget ?? 0;
  if (used === 0 && unknown === 0) return null;
  const amount = used.toLocaleString();
  let label =
    budget > 0
      ? `${amount} of ${budget.toLocaleString()} tokens`
      : `${amount} tokens`;
  if (unknown > 0) {
    label += ` · ${unknown} call${unknown === 1 ? "" : "s"} reported no usage`;
  }
  return label;
}

/** Who acts next, in the daemon's vocabulary. */
export type Actor = "owner" | "assistant" | "worker";

const actors: Record<string, string> = {
  owner: "You",
  assistant: "Your assistant",
  worker: "The worker",
};

export function actorLabel(actor: string): string {
  return actors[actor] || actors.assistant;
}
