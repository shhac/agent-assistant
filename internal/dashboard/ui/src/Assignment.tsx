import { WorkerConversation, workerStateLabel } from "./WorkerConversation";
import { WorkerFailureCard } from "./WorkerFailureCard";
import { sinceLabel, Status } from "./ui";
import type { Agent } from "./api";

/** Worker states that hold the outcome up and deserve to be open by default. */
export const attentionStates = [
  "interrupted",
  "retry_wait",
  "blocked",
  "reconciling",
  "pause_requested",
  "paused",
  "stop_requested",
];

export function needsAttention(agent: Agent): boolean {
  return attentionStates.includes(agent.status);
}

/**
 * The one place an assignment is inspected and controlled. Other views link
 * here rather than restating the same worker: two copies meant two independent
 * conversation polls and two different vocabularies for one status.
 */
export function Assignment({
  agent,
  outcomeTitle,
  demo,
  refresh,
  onInvestigate,
}: {
  agent: Agent;
  outcomeTitle?: string;
  demo: boolean;
  refresh: () => Promise<void>;
  onInvestigate?: (prompt: string) => void;
}) {
  const attention = needsAttention(agent);
  const progress = sinceLabel(agent.last_progress_at || agent.last_update);
  return (
    <article
      className={`assignment ${attention ? "needs-attention" : ""}`}
      aria-label={`Assignment for ${agent.name}`}
    >
      <div className="assignment-heading">
        <strong>{agent.name}</strong>
        <Status tone={attention ? "amber" : agent.status === "completed" ? "green" : ""}>
          {workerStateLabel(agent.status)}
        </Status>
      </div>
      <p className="assignment-meta">
        {progress ? `Last progress ${progress}` : "No progress recorded yet"}
        {agent.recoveries ? ` · ${agent.recoveries} recovery attempts recorded` : ""}
      </p>

      {attention ? (
        <WorkerFailureCard agent={agent} onInvestigate={onInvestigate} />
      ) : (
        agent.summary && (
          <pre className="assignment-summary">{agent.summary}</pre>
        )
      )}

      <WorkerConversation
        agent={agent}
        refresh={refresh}
        demo={demo}
        outcomeTitle={outcomeTitle}
      />
    </article>
  );
}
