import { Icon, sinceLabel, Status } from "./ui";
import type { Project, ProjectAttention } from "./api";

const executionLabels: Record<string, string> = {
  blocked: "Blocked",
  interrupted: "Interrupted",
  reconciling: "Checking worker state",
  review: "Ready for your review",
  paused: "Paused",
  pause_requested: "Pausing",
  stop_requested: "Stopping",
  retry_wait: "Waiting for the model provider",
  waiting: "Waiting",
  queued: "Queued",
  ready: "Ready",
  dispatching: "Starting up",
  resuming: "Resuming",
  running: "Running",
  active: "Running",
};

const nextActionLabels: Record<string, string> = {
  owner: "You",
  assistant: "Your assistant",
  worker: "The worker",
  none: "Nobody",
};

const recoveryLabels: Record<string, string> = {
  scheduled: "Retry scheduled",
  checking: "Checking before any retry",
  held: "Held until you decide",
};

export function executionLabel(execution: string): string {
  return executionLabels[execution] || execution.replaceAll("_", " ");
}

/** Execution states that hold the work up rather than merely describing it. */
export function isHeldUp(execution: string): boolean {
  return ["blocked", "interrupted", "reconciling", "retry_wait"].includes(
    execution,
  );
}

export function attentionTone(item: ProjectAttention): string {
  if (item.next_action === "owner" && isHeldUp(item.execution)) return "amber";
  if (item.execution === "review") return "amber";
  return "";
}

/**
 * Execution health, open decisions and uninspected interruptions are three
 * separate facts. A quiet decision queue is never presented as evidence that
 * the work is healthy.
 */
export function AttentionSummary({
  attention,
  projects,
  decisionCount,
  onOpen,
  onReview,
  hasProjects,
}: {
  attention: ProjectAttention[];
  projects: Project[];
  decisionCount: number;
  onOpen: (projectId: string, workItemId?: string) => void;
  onReview: () => void;
  hasProjects: boolean;
}) {
  const heldUp = attention.filter(
    (a) => isHeldUp(a.execution) || a.execution === "review",
  );
  const needsOwner = heldUp.filter((a) => a.next_action === "owner");
  const calm = !decisionCount && !heldUp.length;

  return (
    <>
      <div
        className={`attention-banner ${decisionCount || needsOwner.length ? "needs-attention" : ""}`}
      >
        <span className="attention-symbol">
          <Icon
            name={
              needsOwner.length ? "Alert" : decisionCount ? "Decisions" : "Check"
            }
            size={23}
          />
        </span>
        <div>
          <h2>{headline(decisionCount, needsOwner.length, heldUp.length)}</h2>
          <p>{body(decisionCount, heldUp.length, calm, hasProjects)}</p>
        </div>
        {decisionCount > 0 && (
          <button className="text-button" onClick={onReview}>
            Review <Icon name="Arrow" size={16} />
          </button>
        )}
      </div>

      {heldUp.length > 0 && (
        <section
          className="section-block attention-work"
          aria-label="Work needing attention"
        >
          <div className="section-heading">
            <h2>Work needing attention</h2>
            <span className="section-note">
              Separate from decisions waiting on you
            </span>
          </div>
          <ul className="attention-list">
            {heldUp.map((item) => {
              const project = projects.find((p) => p.id === item.project_id);
              const progress = sinceLabel(item.last_progress_at);
              return (
                <li key={item.project_id}>
                  <button
                    type="button"
                    className="attention-row"
                    onClick={() => onOpen(item.project_id, item.work_item_id)}
                  >
                    <span className="attention-row-main">
                      <strong>{project?.title || "Project"}</strong>
                      <span className="attention-detail">
                        {item.agent_name ? `${item.agent_name} · ` : ""}
                        {item.reason || executionLabel(item.execution)}
                      </span>
                      <span className="attention-meta">
                        {progress
                          ? `Last progress ${progress}`
                          : "No progress recorded yet"}
                        {" · "}
                        {nextActionLabels[item.next_action] || "Your assistant"}{" "}
                        {item.next_action === "owner" ? "act next" : "acts next"}
                        {item.recovery
                          ? ` · ${recoveryLabels[item.recovery] || item.recovery}`
                          : ""}
                      </span>
                    </span>
                    <span className="attention-row-state">
                      <Status tone={attentionTone(item)}>
                        {executionLabel(item.execution)}
                      </Status>
                      <Icon name="Chevron" size={15} />
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        </section>
      )}
    </>
  );
}

function headline(decisions: number, needsOwner: number, heldUp: number) {
  if (decisions && heldUp)
    return `${decisions} ${decisions === 1 ? "decision" : "decisions"} and ${heldUp} ${heldUp === 1 ? "outcome" : "outcomes"} need you`;
  if (decisions)
    return `${decisions} ${decisions === 1 ? "decision needs" : "decisions need"} your judgment`;
  if (needsOwner)
    return `${needsOwner} ${needsOwner === 1 ? "outcome is" : "outcomes are"} waiting on you`;
  if (heldUp)
    return `${heldUp} ${heldUp === 1 ? "outcome is" : "outcomes are"} not moving`;
  return "No decisions waiting on you";
}

function body(
  decisions: number,
  heldUp: number,
  calm: boolean,
  hasProjects: boolean,
) {
  if (heldUp)
    return "Work has stopped or is waiting. Open it to see the blocker and who acts next.";
  if (decisions)
    return "Your assistant has gathered the context. You make the call.";
  if (!calm) return "Your assistant will bring you anything that needs a decision.";
  return hasProjects
    ? "No work is reported blocked, and nothing needs your judgment."
    : "Start with one meaningful outcome. The coordination happens from there.";
}
