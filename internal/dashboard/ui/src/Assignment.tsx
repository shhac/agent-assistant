import { WorkerConversation } from "./WorkerConversation";
import { WorkerFailureCard } from "./WorkerFailureCard";
import { sinceLabel, Status } from "./ui";
import { needsAttention as attentionState, stateDetail } from "./states";
import { explainFailure } from "./failure";
import type { Agent } from "./api";

export function needsAttention(agent: Agent): boolean {
  return attentionState(agent.status);
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
  // The failure card is the only judge of whether it has an account to give;
  // anything it cannot explain still shows its recorded summary.
  const explained = explainFailure(agent) !== null;
  const progress = sinceLabel(agent.last_progress_at || agent.last_update);
  return (
    <article
      className={`assignment ${attention ? "needs-attention" : ""}`}
      aria-label={`Assignment for ${agent.name}`}
    >
      <div className="assignment-heading">
        <strong>{agent.name}</strong>
        <Status
          tone={
            attention ? "amber" : agent.status === "completed" ? "green" : ""
          }
        >
          {stateDetail(agent.status)}
        </Status>
      </div>
      <p className="assignment-meta">
        {progress ? `Last progress ${progress}` : "No progress recorded yet"}
        {agent.recoveries
          ? ` · ${agent.recoveries} recovery attempts recorded`
          : ""}
      </p>

      {explained ? (
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
