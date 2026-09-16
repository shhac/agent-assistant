import { useEffect, useRef, useState, type FormEvent } from "react";
import {
  api,
  APIError,
  criteriaLines,
  errorText,
  type Project,
  type State,
  type WorkItem,
} from "./api";
import "./work.css";
import { Assignment, needsAttention } from "./Assignment";
import { EvidenceView } from "./EvidenceView";
import { isQueued, orderQueue, startCondition } from "./queue";
import { stateLabel } from "./states";

function when(value: string) {
  const date = new Date(value);
  return Number.isFinite(date.getTime()) ? date.toLocaleString() : "";
}

export function ProjectWork({
  project,
  state,
  refresh,
  onInvestigate,
}: {
  project: Project;
  state: State;
  refresh: () => Promise<void>;
  onInvestigate?: (prompt: string) => void;
}) {
  const items = state.work_items.filter(
    (item) => item.project_id === project.id,
  );
  const hasAttempts = (item: WorkItem) =>
    state.agents.some((agent) => agent.work_item_id === item.id);
  const queued = orderQueue(
    items.filter((item) => isQueued(item, hasAttempts(item))),
  );
  const queuedIds = new Set(queued.map((item) => item.id));
  const current = items.filter((item) => !queuedIds.has(item.id));
  const [adding, setAdding] = useState(false);
  return (
    <section className="section-block project-work" aria-label="Project work">
      <div className="section-heading">
        <h2>
          Work <span>{items.length}</span>
        </h2>
        <button
          type="button"
          className="text-button"
          disabled={state.demo}
          onClick={() => setAdding(!adding)}
        >
          {adding ? "Close outcome form" : "Define an outcome"}
        </button>
      </div>
      <p className="field-hint">
        This project keeps its context across many outcomes. Describe what you
        want in the conversation and your assistant will help define success and
        coordinate the work. You can also record a prepared outcome here.
      </p>
      {adding && (
        <NewOutcome
          project={project}
          items={items}
          refresh={refresh}
          onCreated={() => setAdding(false)}
        />
      )}
      {!items.length && (
        <p className="work-empty">
          No outcomes recorded yet. What would you like to move forward?
        </p>
      )}
      <div className="work-list">
        {current.map((item) => (
          <WorkCard
            key={item.id}
            item={item}
            state={state}
            refresh={refresh}
            onInvestigate={onInvestigate}
          />
        ))}
      </div>
      {!!queued.length && (
        <div className="work-queue" aria-label="Queued outcomes">
          <div className="section-heading">
            <h3>
              Queue <span>{queued.length}</span>
            </h3>
            <span className="section-note">Not started yet</span>
          </div>
          <ol className="queue-list">
            {queued.map((item, index) => (
              <QueuedRow
                key={item.id}
                item={item}
                position={index + 1}
                onInvestigate={onInvestigate}
                predecessor={
                  state.work_items.find(
                    (work) => work.id === item.after_work_item_id,
                  )?.title
                }
                state={state}
                refresh={refresh}
              />
            ))}
          </ol>
        </div>
      )}
    </section>
  );
}

/**
 * A queued outcome is a position in a sequence first. Its full brief, criteria
 * and queue management stay one disclosure away so the order can be read
 * without expanding anything.
 */
function QueuedRow({
  item,
  position,
  predecessor,
  state,
  refresh,
  onInvestigate,
}: {
  item: WorkItem;
  position: number;
  predecessor?: string;
  state: State;
  refresh: () => Promise<void>;
  onInvestigate?: (prompt: string) => void;
}) {
  return (
    <li className="queue-row" aria-label={item.title}>
      <span className="queue-position" aria-hidden="true">
        {position}
      </span>
      <div className="queue-body">
        <div className="queue-heading">
          <strong>{item.title}</strong>
          <span className={`work-status ${item.status}`}>
            {stateLabel(item.status)}
          </span>
        </div>
        <p className="queue-condition">{startCondition(item, predecessor)}</p>
        <details className="queue-detail">
          <summary>Brief and queue management</summary>
          <p className="work-objective">{item.objective}</p>
          <h4>Acceptance criteria</h4>
          {criteriaLines(item.acceptance_criteria).length ? (
            <ul>
              {criteriaLines(item.acceptance_criteria).map((line, i) => (
                <li key={i}>{line}</li>
              ))}
            </ul>
          ) : (
            <p className="muted">No criteria recorded.</p>
          )}
          <WorkCard
            item={item}
            state={state}
            refresh={refresh}
            controlsOnly
          />
          {onInvestigate && (
            <div className="queue-review">
              <button
                type="button"
                className="text-button"
                onClick={() =>
                  onInvestigate(
                    `The queued outcome "${item.title}" may already be satisfied by work done elsewhere. Please review the evidence and tell me whether it still needs doing. Do not accept it or start it on the strength of this question.`,
                  )
                }
              >
                Ask for an evidence review
              </button>
              <p className="field-hint">
                Use this when the outcome may already be handled. It asks your
                assistant to check the evidence; it does not accept the outcome
                or start the work.
              </p>
            </div>
          )}
        </details>
      </div>
    </li>
  );
}

function NewOutcome({
  project,
  items,
  refresh,
  onCreated,
}: {
  project: Project;
  items: WorkItem[];
  refresh: () => Promise<void>;
  onCreated: () => void;
}) {
  const [title, setTitle] = useState("");
  const [objective, setObjective] = useState("");
  const [criteria, setCriteria] = useState("");
  const [after, setAfter] = useState("");
  const predecessor = items.find((item) => item.id === after);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [created, setCreated] = useState(false);
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      if (!created) {
        await api(
          `/api/projects/${encodeURIComponent(project.id)}/work-items${after ? "/queue" : ""}`,
          {
            method: "POST",
            body: JSON.stringify({
              title: title.trim(),
              objective: objective.trim(),
              acceptance_criteria: criteria.trim(),
              ...(after ? { after_work_item_id: after } : {}),
            }),
          },
        );
        setCreated(true);
      }
      await refresh();
      onCreated();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }
  return (
    <form className="work-form" onSubmit={(event) => void submit(event)}>
      <h3>Next outcome</h3>
      <label>
        Outcome name
        <input
          required
          maxLength={300}
          value={title}
          disabled={busy || created}
          onChange={(e) => setTitle(e.target.value)}
        />
      </label>
      <label>
        What should change?
        <textarea
          required
          maxLength={12000}
          rows={3}
          value={objective}
          disabled={busy || created}
          onChange={(e) => setObjective(e.target.value)}
        />
      </label>
      <label>
        How will we know it is done?
        <textarea
          required
          maxLength={12000}
          rows={3}
          value={criteria}
          disabled={busy || created}
          onChange={(e) => setCriteria(e.target.value)}
        />
      </label>
      {!!items.length && (
        <label>
          When should it start?
          <select
            value={after}
            disabled={busy || created}
            onChange={(event) => setAfter(event.target.value)}
          >
            <option value="">
              Record only — arrange a start with the assistant
            </option>
            {items.map((item) => (
              <option value={item.id} key={item.id}>
                After {item.title} is accepted
              </option>
            ))}
          </select>
        </label>
      )}
      <p className="field-hint">
        {predecessor
          ? `Queueing authorizes the assistant to commission this work automatically after “${predecessor.title}” is accepted. No additional kickoff is needed.`
          : "Record the outcome, then discuss it with your assistant to begin coordination. This keeps the project available for the next piece of work."}
      </p>
      {error && (
        <p className="error-notice" role="alert">
          {created ? "Outcome recorded, but the view could not refresh. " : ""}
          {error}
        </p>
      )}
      <button
        className="button primary"
        disabled={
          busy || !title.trim() || !objective.trim() || !criteria.trim()
        }
      >
        {busy
          ? "Recording…"
          : created
            ? "Refresh recorded outcome"
            : predecessor
              ? `Queue after ${predecessor.title}`
              : "Record outcome"}
      </button>
    </form>
  );
}

function WorkCard({
  item,
  state,
  refresh,
  onInvestigate,
  controlsOnly = false,
}: {
  item: WorkItem;
  state: State;
  refresh: () => Promise<void>;
  onInvestigate?: (prompt: string) => void;
  /** Renders only queue management, for an outcome shown as a queue row. */
  controlsOnly?: boolean;
}) {
  const agents = state.agents.filter((agent) => agent.work_item_id === item.id);
  const predecessor = state.work_items.find(
    (work) => work.id === item.after_work_item_id,
  );
  const attention = agents.filter(needsAttention);
  const settled = agents.filter((agent) => !needsAttention(agent));
  const evidence = [
    ...new Set(agents.flatMap((agent) => agent.evidence || [])),
  ];
  const acceptanceCurrent =
    item.status === "accepted" &&
    item.acceptance?.revision === item.review_revision;
  const displayedEvidence = acceptanceCurrent
    ? item.acceptance!.evidence
    : evidence;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [staleRevision, setStaleRevision] = useState<string | null>(null);
  const [recordedRevision, setRecordedRevision] = useState<string | null>(null);
  useEffect(() => {
    setError("");
    setStaleRevision(null);
    setRecordedRevision(null);
  }, [item.review_revision]);
  const awaitingDirection = state.steering.some(
    (message) =>
      message.work_item_id === item.id &&
      !state.steering_receipts.some(
        (receipt) =>
          receipt.message_id === message.id &&
          agents.some(
            (agent) =>
              agent.id === receipt.agent_id && agent.status === "completed",
          ),
      ),
  );
  const stale = staleRevision === item.review_revision;
  const recorded = recordedRevision === item.review_revision;
  async function accept() {
    if (busy || stale || recorded) return;
    setBusy(true);
    setError("");
    const revision = item.review_revision;
    try {
      await api(`/api/work-items/${encodeURIComponent(item.id)}/accept`, {
        method: "POST",
        body: JSON.stringify({ review_revision: revision, evidence }),
      });
      setRecordedRevision(revision);
      await refresh();
    } catch (err) {
      setError(errorText(err));
      if (err instanceof APIError && err.status === 409)
        setStaleRevision(revision);
    } finally {
      setBusy(false);
    }
  }
  async function withdraw() {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await api(`/api/work-items/${encodeURIComponent(item.id)}/queue`, {
        method: "DELETE",
      });
      await refresh();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }
  async function reload() {
    setBusy(true);
    setError("");
    try {
      await refresh();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }
  const queueControls = item.commission_requested && !agents.length && (
    <div className="work-queue-controls">
      <button
        type="button"
        className="button secondary"
        disabled={state.demo || busy}
        onClick={() => void withdraw()}
      >
        {busy ? "Updating queue…" : "Remove from queue"}
      </button>
      <p className="field-hint">
        Keep the outcome as a draft and withdraw permission to start
        automatically.
      </p>
      {error && (
        <p className="error-notice" role="alert">
          {error}
        </p>
      )}
    </div>
  );
  if (controlsOnly)
    return queueControls || (
      <p className="field-hint">
        This outcome is not authorized to start automatically.
      </p>
    );
  return (
    <article className="work-card" aria-label={item.title}>
      <div className="work-heading">
        <h3>{item.title}</h3>
        <span className={`work-status ${item.status}`}>
          {stateLabel(item.status)}
        </span>
      </div>
      <p className="work-objective">{item.objective}</p>
      {item.after_work_item_id && (
        <p className="work-queue-order">
          After <strong>{predecessor?.title || "the preceding outcome"}</strong>{" "}
          is accepted
          {item.commission_requested
            ? " · automatic coordination authorized"
            : ""}
        </p>
      )}
      {queueControls}
      {item.status_reason && (
        <p className="work-state-reason" role="status">
          {item.status_reason}
        </p>
      )}
      {!!attention.length && (
        <div className="work-attention" aria-label="Worker attention">
          {attention.map((agent) => (
            <Assignment
              key={agent.id}
              agent={agent}
              outcomeTitle={item.title}
              demo={state.demo}
              refresh={refresh}
              onInvestigate={onInvestigate}
            />
          ))}
        </div>
      )}
      <div className="work-review-grid">
        <div>
          <h4>Acceptance criteria</h4>
          {criteriaLines(item.acceptance_criteria).length ? (
            <ul>
              {criteriaLines(item.acceptance_criteria).map((line, i) => (
                <li key={i}>{line}</li>
              ))}
            </ul>
          ) : (
            <p className="muted">No criteria recorded.</p>
          )}
        </div>
        <div>
          <h4>
            {acceptanceCurrent ? "Accepted evidence" : "Reported evidence"}
          </h4>
          <EvidenceView
            evidence={displayedEvidence}
            accepted={!!acceptanceCurrent}
          />
        </div>
      </div>
      {acceptanceCurrent && item.acceptance && (
        <p className="work-acceptance">
          Accepted {when(item.acceptance.accepted_at)}. Acceptance records the
          evidence for this reviewed version.
        </p>
      )}
      {item.acceptance && !acceptanceCurrent && (
        <details className="work-attempts">
          <summary>Previous acceptance · historical evidence</summary>
          <p className="field-hint">
            The acceptance recorded {when(item.acceptance.accepted_at)} does not
            cover this outcome’s current review. Review the reported evidence
            above before accepting again.
          </p>
          <ul>
            {item.acceptance.evidence.map((line, i) => (
              <li key={i}>{line}</li>
            ))}
          </ul>
        </details>
      )}
      {item.status === "legacy_completed" && (
        <p className="field-hint">
          Imported completion history. This is not a new acceptance of reviewed
          evidence.
        </p>
      )}
      {item.status === "review" && (
        <div className="work-acceptance-controls">
          <p className="field-hint">
            Check this evidence against the criteria. Acceptance applies to the
            reviewed version of this outcome; the project remains open for
            future work.
          </p>
          {awaitingDirection && (
            <p className="field-hint">
              A completed worker must acknowledge the recorded direction before
              this outcome can be accepted.
            </p>
          )}
          <button
            type="button"
            className="button primary"
            disabled={
              state.demo ||
              busy ||
              stale ||
              recorded ||
              !item.review_revision ||
              !evidence.length
            }
            onClick={() => void accept()}
          >
            {busy
              ? "Recording…"
              : recorded
                ? "Acceptance recorded"
                : "Accept this outcome"}
          </button>
          {(stale || recorded) && (
            <button
              type="button"
              className="button secondary"
              disabled={busy}
              onClick={() => void reload()}
            >
              Refresh evidence
            </button>
          )}
          {stale && (
            <p role="status">
              This outcome changed. Refresh and review the latest evidence
              before accepting.
            </p>
          )}
          {error && (
            <p className="error-notice" role="alert">
              {error}
            </p>
          )}
        </div>
      )}
      {!!settled.length && (
        <details className="work-attempts">
          <summary>
            Other assignments · {settled.length}
          </summary>
          {settled.map((agent) => (
            <Assignment
              key={agent.id}
              agent={agent}
              outcomeTitle={item.title}
              demo={state.demo}
              refresh={refresh}
              onInvestigate={onInvestigate}
            />
          ))}
        </details>
      )}
      <WorkSteering item={item} state={state} refresh={refresh} />
    </article>
  );
}

function WorkSteering({
  item,
  state,
  refresh,
}: {
  item: WorkItem;
  state: State;
  refresh: () => Promise<void>;
}) {
  const messages = state.steering.filter(
    (message) => message.work_item_id === item.id,
  );
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [status, setStatus] = useState("");
  const attempt = useRef<{ message_id: string; message: string } | null>(null);
  const closed =
    item.status === "accepted" || item.status === "legacy_completed";
  async function send(event: FormEvent) {
    event.preventDefault();
    if (busy || !draft.trim()) return;
    setBusy(true);
    setError("");
    setStatus("");
    attempt.current ||= {
      message_id: crypto.randomUUID(),
      message: draft.trim(),
    };
    try {
      await api(`/api/work-items/${encodeURIComponent(item.id)}/steering`, {
        method: "POST",
        body: JSON.stringify(attempt.current),
      });
      attempt.current = null;
      setDraft("");
      setStatus(
        "Direction recorded. Awaiting worker acknowledgment; acknowledgment does not mean the change is complete.",
      );
      try {
        await refresh();
      } catch {
        setStatus(
          "Direction recorded. Refresh the project to see worker acknowledgments.",
        );
      }
    } catch (err) {
      if (err instanceof APIError && err.status >= 400 && err.status < 500)
        attempt.current = null;
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }
  return (
    <details className="work-steering">
      <summary>
        Direction and acknowledgments
        {messages.length ? ` · ${messages.length}` : ""}
      </summary>
      <p className="field-hint">
        Leave a correction or change of direction for this outcome.
        Acknowledgment means the agent says it has read and considered the
        direction, not merely that its runtime received a message. It does not
        mean the change is complete; acceptance of the outcome is separate.
      </p>
      {messages.map((message) => {
        const receipts = state.steering_receipts.filter(
          (receipt) => receipt.message_id === message.id,
        );
        return (
          <div className="work-steering-message" key={message.id}>
            <p>{message.content}</p>
            <small>{when(message.created_at)}</small>
            {receipts.length ? (
              <ul>
                {receipts.map((receipt) => (
                  <li key={receipt.agent_id}>
                    Acknowledged by{" "}
                    {state.agents.find((agent) => agent.id === receipt.agent_id)
                      ?.name || "a worker"}
                    {receipt.acknowledged_at
                      ? ` · ${when(receipt.acknowledged_at)}`
                      : ""}
                  </li>
                ))}
              </ul>
            ) : (
              <p className="field-hint">
                Recorded · awaiting worker acknowledgment
              </p>
            )}
          </div>
        );
      })}
      {!closed && (
        <form className="work-form" onSubmit={(event) => void send(event)}>
          <label>
            Direction for {item.title}
            <textarea
              rows={2}
              maxLength={12000}
              value={draft}
              readOnly={busy || attempt.current !== null}
              disabled={state.demo}
              onChange={(event) => setDraft(event.target.value)}
              placeholder="For example: keep the existing keyboard shortcuts."
            />
          </label>
          <button
            className="button secondary"
            disabled={state.demo || busy || !draft.trim()}
          >
            {busy
              ? "Recording…"
              : attempt.current
                ? "Retry same direction"
                : "Send direction"}
          </button>
          {error && (
            <p className="error-notice" role="alert">
              {error}
              {attempt.current
                ? " Delivery is unconfirmed. Retrying uses the same message, without creating a duplicate."
                : " Direction was not recorded. You can edit it and try again."}
            </p>
          )}
          {status && <p role="status">{status}</p>}
        </form>
      )}
    </details>
  );
}
