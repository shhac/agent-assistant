import { explainFailure, ownerLabel } from "./failure";
import { dateLabel, Icon, sinceLabel } from "./ui";
import type { Agent } from "./api";

/**
 * What stopped, what the evidence establishes, what it does not, and who acts
 * next. Rendering this card performs no control action: recovery stays an
 * explicit owner choice routed through the daemon's existing admission rules.
 */
export function WorkerFailureCard({
  agent,
  onInvestigate,
  controls,
}: {
  agent: Agent;
  /** Hands the question to the assistant conversation. Never resumes work. */
  onInvestigate?: (prompt: string) => void;
  controls?: React.ReactNode;
}) {
  const explanation = explainFailure(agent);
  if (!explanation) return null;
  const stoppedAt = agent.last_update || agent.broker_updated_at;
  const since = sinceLabel(stoppedAt);
  return (
    <section
      className={`failure-card ${explanation.owner === "owner" ? "needs-owner" : ""}`}
      aria-label={`What stopped ${agent.name}`}
    >
      <div className="failure-headline">
        <span className="failure-symbol">
          <Icon name={explanation.owner === "owner" ? "Alert" : "Clock"} size={18} />
        </span>
        <div>
          <strong>{explanation.headline}</strong>
          {stoppedAt && !stoppedAt.startsWith("0001-") && (
            <p className="field-hint">
              Last recorded {since || "recently"} ·{" "}
              <time dateTime={stoppedAt}>{dateLabel(stoppedAt)}</time>
            </p>
          )}
        </div>
      </div>

      <dl className="failure-facts">
        <div>
          <dt>What is known</dt>
          <dd>{explanation.known || "The recorded summary is the only account of this attempt."}</dd>
        </div>
        {explanation.unknown && (
          <div>
            <dt>What is not known</dt>
            <dd>{explanation.unknown}</dd>
          </div>
        )}
        <div>
          <dt>Progress preserved</dt>
          <dd>
            {agent.evidence?.length
              ? `${agent.evidence.length} evidence ${agent.evidence.length === 1 ? "record" : "records"} kept with the assignment.`
              : "The isolated workspace is preserved for inspection."}
            {!!agent.context_compactions &&
              ` ${agent.context_compactions} context checkpoint${agent.context_compactions === 1 ? "" : "s"} saved.`}
          </dd>
        </div>
        <div>
          <dt>Recovery</dt>
          <dd>
            {explanation.recovery}
            {agent.retry_at && !agent.retry_at.startsWith("0001-") && (
              <>
                {" "}
                Next attempt after{" "}
                <time dateTime={agent.retry_at}>
                  {dateLabel(agent.retry_at)}
                </time>
                .
              </>
            )}
          </dd>
        </div>
        <div>
          <dt>Who acts next</dt>
          <dd>
            <strong>{ownerLabel(explanation.owner)}</strong> · {explanation.nextStep}
          </dd>
        </div>
      </dl>

      {agent.summary && (
        <div className="failure-summary">
          <span className="field-hint">Recorded summary</span>
          <pre>{agent.summary}</pre>
        </div>
      )}

      {controls}

      {onInvestigate && (
        <button
          type="button"
          className="button secondary"
          onClick={() =>
            onInvestigate(
              `${agent.name} stopped and I want to understand why before deciding anything. Please inspect the preserved assignment and tell me what the evidence shows.`,
            )
          }
        >
          Ask assistant to investigate
        </button>
      )}

      <details className="failure-technical">
        <summary>Technical details</summary>
        <dl>
          {agent.provider_failure_kind && (
            <div>
              <dt>Provider classification</dt>
              <dd>{agent.provider_failure_kind.replaceAll("_", " ")}</dd>
            </div>
          )}
          {agent.model_failure_engine && (
            <div>
              <dt>Engine</dt>
              <dd>{agent.model_failure_engine}</dd>
            </div>
          )}
          {agent.model_failure_phase && (
            <div>
              <dt>Phase</dt>
              <dd>{agent.model_failure_phase}</dd>
            </div>
          )}
          {agent.model_failure_code && (
            <div>
              <dt>Provider code</dt>
              <dd>
                <code>{agent.model_failure_code}</code>
              </dd>
            </div>
          )}
          {agent.model_exit_code !== undefined && (
            <div>
              <dt>CLI exit code</dt>
              <dd>{agent.model_exit_code}</dd>
            </div>
          )}
          <div>
            <dt>Recovery attempts</dt>
            <dd>{agent.recoveries ?? 0}</dd>
          </div>
          {!!agent.provider_failures && (
            <div>
              <dt>Consecutive provider failures</dt>
              <dd>{agent.provider_failures}</dd>
            </div>
          )}
        </dl>
      </details>
    </section>
  );
}
