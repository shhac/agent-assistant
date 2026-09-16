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
export function needsAttention(state: string): boolean {
  return isHeldUp(state) || ownerControl.includes(state);
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
