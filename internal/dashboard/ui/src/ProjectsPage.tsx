import { useState } from "react";
import { ProjectDirectories } from "./ProjectForms";
import { WorkerPreparation } from "./WorkerPreparation";
import { ProjectWorkers } from "./ProjectWorkers";
import { ProjectWork } from "./ProjectWork";
import { ActivityList, ProjectRow, WorkerUsageHold } from "./OverviewPage";
import { attentionHeldUp, stateLabel } from "./states";
import {
  CriteriaList,
  Empty,
  ErrorNotice,
  humanStatus,
  Icon,
  PageHeading,
  Status,
} from "./ui";
import { api, errorText, type Project, type State } from "./api";

export function Projects({
  state,
  selected,
  onSelect,
  onNew,
  refresh,
  onInvestigate,
}: {
  state: State;
  selected: string | null;
  onSelect: (id: string | null) => void;
  onNew: () => void;
  refresh: () => Promise<void>;
  onInvestigate?: (prompt: string) => void;
}) {
  const [workersRevision, setWorkersRevision] = useState(0);
  const project = state.projects.find((p) => p.id === selected);
  if (project) {
    const projectState = {
      ...state,
      activity: state.activity.filter((a) => a.project_id === project.id),
    };
    return (
      <section>
        <button
          className="text-button back-link"
          onClick={() => onSelect(null)}
        >
          ← All projects
        </button>
        <PageHeading
          eyebrow="PROJECT CONTEXT"
          title={project.title}
          description={project.description}
          action={
            <span className="project-states">
              {(() => {
                const health = state.attention.find(
                  (a) => a.project_id === project.id,
                );
                return health && attentionHeldUp(health) ? (
                  <Status tone="amber">{stateLabel(health.execution)}</Status>
                ) : null;
              })()}
              <Status>{humanStatus(project.status)}</Status>
            </span>
          }
        />
        <WorkerUsageHold project={project} integrations={state.integrations} />
        <ProjectWork
          key={`work-${project.id}`}
          project={project}
          state={state}
          refresh={refresh}
          onInvestigate={onInvestigate}
        />
        <CoordinateProject
          key={`coordinate-${project.id}`}
          project={project}
          refresh={refresh}
        />
        <details className="project-reference">
          <summary>Brief, folders and worker setup</summary>
          <section className="detail-section">
            <p className="eyebrow">PROJECT GUIDANCE</p>
            <CriteriaList
              criteria={project.acceptance_criteria}
              empty="No acceptance criteria recorded."
              marker
            />
          </section>
          <ProjectDirectories
            key={project.id}
            project={project}
            refresh={refresh}
          />
          <WorkerPreparation
            key={`worker-${project.id}`}
            commissioned={
              state.agents.filter((a) => a.project_id === project.id).length
            }
            project={project}
            demo={state.demo}
            onPrepared={() => {
              setWorkersRevision((n) => n + 1);
              void refresh();
            }}
          />
          <ProjectWorkers
            key={project.id}
            project={project}
            state={state}
            refresh={refresh}
            revision={workersRevision}
          />
          <details className="project-setup-details">
            <summary>Technical identifiers</summary>
            <label htmlFor="project-setup-id">
              Project ID
              <input
                id="project-setup-id"
                value={project.id}
                readOnly
                onFocus={(e) => e.target.select()}
              />
            </label>
            <p className="field-hint">
              For diagnostics and external integrations. Worker setup uses the
              project name automatically.
            </p>
          </details>
        </details>
        <section className="section-block">
          <div className="section-heading">
            <h2>Evidence and activity</h2>
          </div>
          <ActivityList state={projectState} />
        </section>
      </section>
    );
  }
  return (
    <section>
      <PageHeading
        eyebrow="OUTCOMES, WITH OWNERSHIP"
        title="Projects"
        description="What you're moving forward, and what done looks like."
        action={
          <button className="button primary" onClick={onNew}>
            <Icon name="Plus" size={16} />
            Add project
          </button>
        }
      />
      {state.projects.length ? (
        <div className="project-list">
          {state.projects.map((p) => (
            <ProjectRow
              key={p.id}
              project={p}
              state={state}
              onSelect={() => onSelect(p.id)}
            />
          ))}
        </div>
      ) : (
        <Empty
          icon="Projects"
          title="One outcome is a good start"
          action={
            <button className="button primary" onClick={onNew}>
              Add project <Icon name="Arrow" size={15} />
            </button>
          }
        >
          Give the work a name, describe the outcome, and define how you'll know
          it's complete.
        </Empty>
      )}
    </section>
  );
}

function CoordinateProject({
  project,
  refresh,
}: {
  project: Project;
  refresh: () => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [sent, setSent] = useState(false);
  const [next, setNext] = useState("");
  async function coordinate() {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await api(`/api/projects/${encodeURIComponent(project.id)}/coordinate`, {
        method: "POST",
        body: JSON.stringify({ next }),
      });
      setSent(true);
      await refresh();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="coordinate-project">
      <div>
        <label htmlFor="project-next">What would you like to do next?</label>
        <span>
          Describe the change or outcome you want. Your assistant will clarify
          the goal, arrange a worker, and follow through.
        </span>
      </div>
      <textarea
        id="project-next"
        value={next}
        onChange={(e) => {
          setNext(e.target.value);
          setSent(false);
        }}
        placeholder="For example: improve the onboarding, fix a bug, or help me choose the next priority."
        rows={3}
        maxLength={12000}
        disabled={busy}
      />
      <button
        className="button primary"
        disabled={busy}
        onClick={() => void coordinate()}
      >
        {busy
          ? "Thinking it through…"
          : next.trim()
            ? "Work on this with me"
            : "Help me choose"}
        <Icon name="Arrow" size={14} />
      </button>
      <ErrorNotice error={error} />
      {sent && (
        <p className="coordinate-sent" role="status">
          Coordination request recorded. Follow the conversation for the
          response.
        </p>
      )}
    </div>
  );
}
