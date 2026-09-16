import { useEffect, useState } from "react";
import {
  api,
  errorText,
  pendingDecisions,
  type Agent,
  type Project,
  type State,
} from "./api";
import { stateDetail } from "./states";
import { fullDateLabel, humanStatus, recordedTime } from "./ui";
import { WorkerEditor } from "./WorkerEditor";
import "./workers.css";

export interface ProjectWorker {
  id: string;
  name: string;
  project_id: string;
  workspace: string;
  managed: boolean;
  capabilities: string[];
  model: { engine: string; model: string; effort: string } | null;
  model_status: string;
  settings_editable: boolean;
  detail: string;
}
const terminal = new Set(["completed", "cancelled"]);
function timestamp(value?: string) {
  return fullDateLabel(value) || "Not recorded";
}

export function ProjectWorkers({
  project,
  state,
  refresh,
  revision = 0,
}: {
  project: Project;
  state: State;
  refresh: () => Promise<void>;
  revision?: number;
}) {
  const [workers, setWorkers] = useState<ProjectWorker[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [reload, setReload] = useState(0);
  const [editing, setEditing] = useState<string | null>(null);
  const agents = state.agents.filter(
    (agent) => agent.project_id === project.id,
  );
  const activityKey = agents
    .map((agent) => `${agent.id}:${agent.status}`)
    .join("|");
  useEffect(() => {
    let current = true;
    let pending = false;
    const controller = new AbortController();
    async function load() {
      if (pending) return;
      pending = true;
      try {
        const result = await api<{ workers: ProjectWorker[] }>(
          `/api/projects/${encodeURIComponent(project.id)}/workers`,
          { signal: controller.signal },
        );
        if (current) {
          setWorkers(result.workers || []);
          setError("");
        }
      } catch (err) {
        if (current) setError(errorText(err));
      } finally {
        pending = false;
        if (current) setLoading(false);
      }
    }
    void load();
    const interval = window.setInterval(() => void load(), 30_000);
    return () => {
      current = false;
      controller.abort();
      window.clearInterval(interval);
    };
  }, [project.id, activityKey, revision, reload]);
  async function saved(worker: ProjectWorker) {
    setWorkers((current) =>
      current.map((item) => (item.id === worker.id ? worker : item)),
    );
    setEditing(null);
    setReload((value) => value + 1);
    try {
      await refresh();
    } catch (err) {
      setError(errorText(err));
    }
  }
  return (
    <section
      className="section-block project-workers"
      aria-labelledby="project-workers-title"
    >
      <div className="section-heading">
        <h2 id="project-workers-title">Workers</h2>
        <button
          className="text-button"
          type="button"
          onClick={() => setReload((value) => value + 1)}
        >
          Refresh workers
        </button>
      </div>
      <p className="field-hint">
        Each commissioned agent owns a scoped outcome. Your assistant
        coordinates the work; the daemon manages sessions, messages and
        follow-up.
      </p>
      {error && (
        <div className="error-notice" role="alert">
          {error} Worker settings may be out of date.
        </div>
      )}
      <h3>Prepared workers</h3>
      {loading ? (
        <p role="status">Loading worker settings…</p>
      ) : (
        !workers.length && (
          <p className="muted">
            No worker profiles are prepared for this project.
          </p>
        )
      )}
      <div className="project-worker-list">
        {workers.map((worker) => {
          const runs = agents.filter((agent) => agent.profile_id === worker.id);
          const active = runs.filter((agent) => !terminal.has(agent.status));
          return (
            <article
              className="project-worker-card"
              key={worker.id}
              aria-label={worker.name}
            >
              <div className="project-worker-heading">
                <h4>{worker.name}</h4>
                <span className="worker-state">
                  {active.length
                    ? `${active.length} active assignment${active.length === 1 ? "" : "s"}`
                    : "Prepared · no active assignments"}
                </span>
              </div>
              <dl className="worker-facts">
                <div>
                  <dt>Configured model</dt>
                  <dd>
                    {worker.model
                      ? `${worker.model.engine === "codex" ? "Codex" : worker.model.engine === "claude" ? "Claude Code" : worker.model.engine} · ${worker.model.model || "CLI default"} · ${worker.model.effort || "default"} effort`
                      : "Not reported by this worker"}
                  </dd>
                </div>
                <div>
                  <dt>Model status</dt>
                  <dd>{humanStatus(worker.model_status || "unknown")}</dd>
                </div>
                <div>
                  <dt>Project folder</dt>
                  <dd>{worker.workspace || "Managed externally"}</dd>
                </div>
              </dl>
              {worker.detail && <p className="field-hint">{worker.detail}</p>}
              {worker.managed && (
                <button
                  type="button"
                  className="button secondary"
                  disabled={state.demo || !worker.settings_editable}
                  onClick={() => setEditing(worker.id)}
                >
                  Edit worker settings
                </button>
              )}
              {worker.managed && !worker.settings_editable && (
                <p className="field-hint">
                  Settings cannot be changed while this worker has active or
                  unresolved work.
                </p>
              )}
              {!worker.managed && (
                <p className="field-hint">
                  This worker's model is configured in its external runtime.
                </p>
              )}
              {editing === worker.id && (
                <WorkerEditor
                  worker={worker}
                  projectID={project.id}
                  onCancel={() => setEditing(null)}
                  onSaved={saved}
                  disabled={state.demo || !worker.settings_editable}
                />
              )}
            </article>
          );
        })}
      </div>
      <h3>Commissioned work</h3>
      {!agents.length ? (
        <p className="worker-empty">
          No work has been commissioned. Preparing a worker does not start a
          task. Tell {state.assistant.name || "your assistant"} what you want to
          do next in the conversation.
        </p>
      ) : (
        <div className="project-worker-list">
          {agents.map((agent) => (
            <CommissionedAssignment
              key={agent.id}
              agent={agent}
              worker={workers.find((worker) => worker.id === agent.profile_id)}
              state={state}
            />
          ))}
        </div>
      )}
    </section>
  );
}
function CommissionedAssignment({
  agent,
  worker,
  state,
}: {
  agent: Agent;
  worker?: ProjectWorker;
  state: State;
}) {
  const decisions = pendingDecisions(state.decisions).filter(
    (decision) => decision.agent_id === agent.id,
  );
  const coordinator = agent.parent_id
    ? state.agents.find(
        (peer) =>
          peer.id === agent.parent_id && peer.project_id === agent.project_id,
      )?.name || "Coordinator unavailable"
    : state.assistant.name || "Your assistant";
  const outcome = state.work_items.find(
    (work) => work.id === agent.work_item_id,
  );
  const due = recordedTime(agent.next_check_in);
  const overdue =
    !terminal.has(agent.status) && due && due.getTime() < Date.now();
  return (
    <article
      className="project-worker-card assignment-card"
      aria-label={agent.name || agent.role}
    >
      <div className="project-worker-heading">
        <h4>{agent.name || agent.role}</h4>
        <span className="worker-state">{stateDetail(agent.status)}</span>
      </div>
      <p className="field-hint">
        {humanStatus(agent.role)}
        {worker ? ` · ${worker.name}` : " · Worker settings unavailable"}
      </p>
      {outcome && <p className="field-hint">Outcome: {outcome.title}</p>}
      <p className="worker-task">
        {agent.task || "No task description recorded."}
      </p>
      <p className="field-hint">
        Progress, evidence and worker controls are shown with the outcome above.
        {outcome && (
          <>
            {" "}
            <a href={`#/projects/${encodeURIComponent(agent.project_id)}`}>
              Open {outcome.title}
            </a>
          </>
        )}
      </p>
      {overdue && (
        <p className="worker-health">
          Check-in overdue. Progress needs checking; silence alone does not
          confirm a blocker.
        </p>
      )}
      <dl className="worker-facts">
        <div>
          <dt>Escalates to</dt>
          <dd>{coordinator}</dd>
        </div>
        <div>
          <dt>Last worker report</dt>
          <dd>{timestamp(agent.broker_updated_at)}</dd>
        </div>
        <div>
          <dt>Local coordination update</dt>
          <dd>{timestamp(agent.last_update)}</dd>
        </div>
        <div>
          <dt>Next check-in</dt>
          <dd>
            {terminal.has(agent.status)
              ? "No check-in due"
              : timestamp(agent.next_check_in)}
          </dd>
        </div>
      </dl>
      <p className="field-hint">
        The worker report time comes from its runtime. Local updates can reflect
        supervision without new worker progress.
      </p>
      {!!decisions.length && (
        <div className="worker-decisions">
          <h5>Outstanding decisions</h5>
          {decisions.map((decision) => (
            <div key={decision.id}>
              <strong>{decision.title}</strong>
              <p>{decision.context}</p>
              {decision.recommendation && (
                <p>Recommendation: {decision.recommendation}</p>
              )}
            </div>
          ))}
          <p className="field-hint">
            Answer these in Decisions or discuss them with your assistant.
          </p>
        </div>
      )}
    </article>
  );
}
