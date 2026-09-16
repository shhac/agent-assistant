import { ProjectLink } from "./ProjectLink";
import { WorkerPreparation } from "./WorkerPreparation";
import { ProjectWorkers } from "./ProjectWorkers";
import { ProjectWork } from "./ProjectWork";
import { ChatPanel } from "./ChatPanel";
import { NewProject, ProjectDirectories } from "./ProjectForms";
import { Avatar, ThemePicker, validTheme } from "./Identity";
import { AssistantSetup } from "./AssistantSetup";
import { ConnectionsSettings } from "./ConnectionsSettings";
import { ModelSettings } from "./ModelSettings";
import { ChatSettings } from "./ChatSettings";
import { WorkerUsageSettings } from "./WorkerUsageSettings";
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";
import {
  api,
  APIError,
  bootstrapSession,
  criteriaLines,
  errorText,
  normalizeState,
  pendingDecisions,
  type Config,
  type Decision,
  type Project,
  type State,
} from "./api";

import { WorkerSettings } from "./WorkerSettings";
import { PendingOperations } from "./PendingOperations";
import { DecisionHistory } from "./DecisionHistory";
import { AttentionSummary } from "./AttentionSummary";
import { isHeldUp, stateLabel } from "./states";
import { groupActivity } from "./activity";
import {
  dateLabel,
  Empty,
  ErrorNotice,
  humanStatus,
  Icon,
  Mark,
  PageHeading,
  Status,
} from "./ui";

type Page = "Overview" | "Projects" | "Decisions" | "Memory" | "Settings";
const pages: Page[] = [
  "Overview",
  "Projects",
  "Decisions",
  "Memory",
  "Settings",
];
export function App() {
  const [state, setState] = useState<State | null>(null);
  const [page, setPage] = useState<Page>("Overview");
  const [selectedProject, setSelectedProject] = useState<string | null>(null);
  const [authRequired, setAuthRequired] = useState(false);
  const [connectionError, setConnectionError] = useState("");
  const [newProject, setNewProject] = useState(false);
  const [chatOpen, setChatOpen] = useState(false);
  const [chatExpanded, setChatExpanded] = useState(false);
  const [chatPrefill, setChatPrefill] = useState<{
    text: string;
    nonce: number;
  } | null>(null);
  const [controlBusy, setControlBusy] = useState(false);
  const [controlError, setControlError] = useState("");
  const request = useRef(0);
  const conversation = useRef<HTMLElement>(null);
  const refresh = useCallback(async () => {
    const generation = ++request.current;
    try {
      const value = await api<State>("/api/state");
      if (generation !== request.current) return;
      setState(normalizeState(value));
      setAuthRequired(false);
      setConnectionError("");
    } catch (error) {
      if (generation !== request.current) return;
      if (error instanceof APIError && error.status === 401) {
        setAuthRequired(true);
        setState(null);
      } else setConnectionError(errorText(error));
    }
  }, []);
  useEffect(() => {
    void bootstrapSession()
      .then(refresh)
      .catch((error) => {
        setAuthRequired(true);
        setConnectionError(errorText(error));
      });
    const timer = setInterval(() => {
      if (!document.hidden) void refresh();
    }, 5000);
    return () => {
      clearInterval(timer);
      request.current++;
    };
  }, [refresh]);
  useEffect(() => {
    document.title = state?.assistant.name
      ? `${page} · ${state.assistant.name}`
      : "Agent Assistant";
  }, [page, state?.assistant.name]);
  useEffect(() => {
    if (!chatOpen) return;
    const prior = document.activeElement as HTMLElement | null;
    conversation.current
      ?.querySelector<HTMLButtonElement>(".mobile-close")
      ?.focus();
    const keydown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        setChatOpen(false);
      }
      if (event.key !== "Tab") return;
      const focusable = Array.from(
        conversation.current?.querySelectorAll<HTMLElement>(
          "button:not([disabled]),textarea:not([disabled]),input:not([disabled]),a[href]",
        ) || [],
      ).filter((el) => el.getClientRects().length > 0);
      const first = focusable[0],
        last = focusable[focusable.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last?.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first?.focus();
      }
    };
    document.addEventListener("keydown", keydown);
    return () => {
      document.removeEventListener("keydown", keydown);
      prior?.focus();
    };
  }, [chatOpen]);
  useEffect(() => {
    if (!window.matchMedia) return;
    const desktop = window.matchMedia("(min-width: 1001px)");
    const changed = () => {
      if (desktop.matches) setChatOpen(false);
      else setChatExpanded(false);
    };
    desktop.addEventListener("change", changed);
    return () => desktop.removeEventListener("change", changed);
  }, []);
  useEffect(() => {
    document.documentElement.dataset.theme = validTheme(state?.assistant.theme);
  }, [state?.assistant.theme]);
  useEffect(() => {
    const follow = () => {
      const match = /^#\/projects\/([^/]+)$/.exec(window.location.hash);
      if (match) {
        try {
          setSelectedProject(decodeURIComponent(match[1]));
          setPage("Projects");
          setChatExpanded(false);
          setChatOpen(false);
        } catch {
          /* Ignore malformed bookmarks. */
        }
      }
    };
    follow();
    window.addEventListener("hashchange", follow);
    return () => window.removeEventListener("hashchange", follow);
  }, []);
  function openProject(id: string) {
    window.location.hash = `/projects/${encodeURIComponent(id)}`;
    setSelectedProject(id);
    setPage("Projects");
    setChatExpanded(false);
    setChatOpen(false);
  }
  function navigate(next: Page) {
    window.history.replaceState(
      window.history.state,
      "",
      window.location.pathname + window.location.search,
    );
    setChatExpanded(false);
    setPage(next);
    setSelectedProject(null);
  }
  // Hands a question to the conversation. It proposes a message and never
  // resumes, retries or otherwise touches the worker.
  function askAssistant(text: string) {
    setChatPrefill({ text, nonce: Date.now() });
    setChatOpen(true);
  }
  async function togglePause() {
    if (!state) return;
    setControlBusy(true);
    setControlError("");
    try {
      await api("/api/control", {
        method: "POST",
        body: JSON.stringify({ paused: !state.paused }),
      });
      await refresh();
    } catch (error) {
      setControlError(errorText(error));
    } finally {
      setControlBusy(false);
    }
  }
  if (authRequired)
    return <Login onSuccess={refresh} initialError={connectionError} />;
  if (!state)
    return (
      <div className="loading-screen">
        <Mark />
        <h1>Agent Assistant</h1>
        <p role="status">
          {connectionError
            ? "The daemon is unavailable."
            : "Connecting to your assistant…"}
        </p>
        <ErrorNotice error={connectionError} />
        {connectionError && (
          <button className="button primary" onClick={() => void refresh()}>
            Try again
          </button>
        )}
      </div>
    );
  const decisions = pendingDecisions(state.decisions);
  const name = state.assistant.name || "Assistant";
  return (
    <div className={`app-shell ${chatExpanded ? "chat-expanded" : ""}`}>
      <a
        className="skip-link"
        href={chatExpanded ? "#chat-message" : "#main-content"}
      >
        Skip to content
      </a>
      <aside className="sidebar" inert={chatOpen}>
        <button
          className="brand"
          onClick={() => navigate("Overview")}
          aria-label={`${name} overview`}
        >
          <Avatar avatar={state.assistant.avatar} small />
          <span>
            {name}
            <small>PERSONAL ASSISTANT</small>
          </span>
        </button>
        <div className="sidebar-rule" />
        <nav aria-label="Main navigation">
          {pages.map((item) => (
            <button
              key={item}
              className={`nav-item ${page === item ? "active" : ""}`}
              aria-current={page === item ? "page" : undefined}
              onClick={() => navigate(item)}
            >
              <Icon name={item} />
              <span>{item}</span>
              {item === "Decisions" &&
                decisions.length + state.pending_operations.length > 0 && (
                  <span className="nav-count">
                    {decisions.length + state.pending_operations.length}
                  </span>
                )}
            </button>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="private-label">
            <Icon name="Lock" size={14} /> Your private workspace
          </div>
          <div className="daemon-card">
            <span
              className={`status-dot ${connectionError ? "offline" : state.paused ? "paused" : ""}`}
            />
            <div>
              {connectionError
                ? "Connection interrupted"
                : state.paused
                  ? "Dispatch paused"
                  : "Daemon connected"}
              <small>
                {connectionError
                  ? "Showing last received state"
                  : state.paused
                    ? "Existing work may continue"
                    : "Coordination, with context"}
              </small>
            </div>
          </div>
          <button
            className="text-button pause-button"
            disabled={controlBusy || !!connectionError}
            onClick={() => void togglePause()}
          >
            <Icon name={state.paused ? "Play" : "Pause"} size={13} />
            {controlBusy
              ? "Updating…"
              : state.paused
                ? "Resume dispatch"
                : "Pause new work"}
          </button>
          <ErrorNotice error={controlError} />
        </div>
      </aside>
      <div className="workspace" inert={chatOpen || chatExpanded}>
        <header className="topbar">
          <div className="breadcrumbs">
            Workspace <span>/</span> <strong>{page}</strong>
          </div>
          <div className="topbar-actions">
            <span className="local-label">
              <span className="status-dot" />{" "}
              {state.demo ? "Demo workspace" : "Personal workspace"}
            </span>
            <button
              className="icon-button chat-toggle"
              aria-label="Open conversation"
              onClick={() => setChatOpen(true)}
            >
              <Icon name="Message" />
            </button>
            <button
              className="owner-avatar"
              aria-label="Open settings"
              onClick={() => navigate("Settings")}
            >
              You
            </button>
          </div>
        </header>
        {state.demo && (
          <div className="demo-banner">
            Preview mode · Sample projects and agents. No live work is running.
          </div>
        )}
        {connectionError && (
          <div className="connection-banner" role="status">
            Connection interrupted. These are the last received updates.{" "}
            <button onClick={() => void refresh()}>Reconnect</button>
          </div>
        )}
        <main id="main-content" className="main-content">
          {page === "Overview" && (
            <Overview
              state={state}
              onNew={() => setNewProject(true)}
              onNavigate={navigate}
              onProject={openProject}
              refresh={refresh}
            />
          )}
          {page === "Projects" && (
            <Projects
              onInvestigate={askAssistant}
              state={state}
              selected={selectedProject}
              onSelect={(id) => (id ? openProject(id) : navigate("Projects"))}
              onNew={() => setNewProject(true)}
              refresh={refresh}
            />
          )}
          {page === "Decisions" && (
            <section>
              <PageHeading
                eyebrow="YOUR JUDGMENT, WHERE IT COUNTS"
                title="Decisions"
                description="The context, the trade-off, and a recommendation. Make the call and the work can move."
              />
              <PendingOperations
                operations={state.pending_operations}
                projects={state.projects}
                refresh={refresh}
              />
              {decisions.length ? (
                <div className="decision-list">
                  {decisions.map((d) => (
                    <DecisionCard
                      key={d.id}
                      decision={d}
                      projects={state.projects}
                      refresh={refresh}
                    />
                  ))}
                </div>
              ) : !state.pending_operations.length ? (
                <Empty
                  icon="Check"
                  title="Nothing needs your decision"
                  action={
                    <button
                      className="button secondary"
                      onClick={() => navigate("Projects")}
                    >
                      View projects <Icon name="Arrow" size={15} />
                    </button>
                  }
                >
                  When your assistant needs your judgment, it will bring a clear
                  recommendation here.
                </Empty>
              ) : null}
              <DecisionHistory
                decisions={state.decisions.filter((d) => !decisions.includes(d))}
                projects={state.projects}
              />
            </section>
          )}
          {page === "Memory" && <MemoryView state={state} refresh={refresh} />}
          {page === "Settings" && (
            <Settings
              state={state}
              refresh={refresh}
              control={
                <>
                  <button
                    className="button secondary"
                    disabled={controlBusy || !!connectionError}
                    onClick={() => void togglePause()}
                  >
                    <Icon name={state.paused ? "Play" : "Pause"} size={14} />
                    {controlBusy
                      ? "Updating…"
                      : state.paused
                        ? "Resume dispatch"
                        : "Pause new work"}
                  </button>
                  <ErrorNotice error={controlError} />
                </>
              }
            />
          )}
        </main>
      </div>
      <aside
        ref={conversation}
        role={chatOpen ? "dialog" : undefined}
        aria-modal={chatOpen || undefined}
        className={`conversation ${chatOpen ? "mobile-open" : ""}`}
        aria-label={`Conversation with ${name}`}
      >
        <ChatPanel
          prefill={chatPrefill}
          onProjectOpen={openProject}
          state={state}
          refresh={refresh}
          onClose={() => setChatOpen(false)}
          expanded={chatExpanded}
          onExpand={() => {
            setChatExpanded(!chatExpanded);
            requestAnimationFrame(() =>
              document.getElementById("chat-message")?.focus(),
            );
          }}
        />
      </aside>
      {newProject && (
        <NewProject
          onClose={() => setNewProject(false)}
          onCreated={async () => {
            setNewProject(false);
            navigate("Projects");
            await refresh();
          }}
        />
      )}
    </div>
  );
}
function Overview({
  state,
  onNew,
  onNavigate,
  onProject,
  refresh,
}: {
  state: State;
  onNew: () => void;
  onNavigate: (p: Page) => void;
  onProject: (id: string) => void;
  refresh: () => Promise<void>;
}) {
  const [fullActivity, setFullActivity] = useState(false);
  const decisions = pendingDecisions(state.decisions);
  const active = state.projects.filter(
    (p) => !["completed", "cancelled", "archived"].includes(p.status),
  );
  return (
    <section>
      <PageHeading
        eyebrow="ROOM TO FOCUS"
        title="A clear view of the work."
        description={
          state.projects.length
            ? "The outcomes you care about. The decisions that need you."
            : "Give your assistant an outcome. Keep your attention for the decisions that matter."
        }
        action={
          <button className="button primary" onClick={onNew}>
            <Icon name="Plus" size={16} />
            Add project
          </button>
        }
      />
      <AttentionSummary
        attention={state.attention}
        projects={state.projects}
        decisionCount={decisions.length}
        onOpen={onProject}
        onReview={() => onNavigate("Decisions")}
        hasProjects={state.projects.length > 0}
      />
      {decisions.length > 0 && (
        <div className="overview-decision">
          {decisions.slice(0, 3).map((decision) => (
            <DecisionCard
              key={decision.id}
              decision={decision}
              projects={state.projects}
              refresh={refresh}
              compact
            />
          ))}
        </div>
      )}
      <section className="section-block">
        <div className="section-heading">
          <h2>
            Projects in motion <span>{active.length}</span>
          </h2>
          {state.projects.length > 0 && (
            <button
              className="text-button"
              onClick={() => onNavigate("Projects")}
            >
              All projects <Icon name="Arrow" size={14} />
            </button>
          )}
        </div>
        {active.length ? (
          <div className="project-list">
            {active.slice(0, 5).map((p) => (
              <ProjectRow
                project={p}
                state={state}
                key={p.id}
                onSelect={() => onProject(p.id)}
              />
            ))}
          </div>
        ) : (
          <Empty
            icon="Projects"
            title={
              state.projects.length
                ? "Room for your next outcome"
                : "Start with the outcome"
            }
            action={
              <button className="button secondary" onClick={onNew}>
                Set up a project <Icon name="Arrow" size={15} />
              </button>
            }
          >
            {state.projects.length
              ? "Your active project list is clear. Completed work remains in Projects."
              : "Describe what should be achieved and what success looks like. Your assistant keeps the context together."}
          </Empty>
        )}
      </section>
      <section className="section-block">
        <div className="section-heading">
          <h2>Behind the scenes</h2>
          <span className="section-note">The work around the work</span>
        </div>
        <ActivityList state={state} limit={fullActivity ? undefined : 5} />
        {state.activity.length > 5 && (
          <button
            type="button"
            className="text-button"
            aria-expanded={fullActivity}
            onClick={() => setFullActivity((open) => !open)}
          >
            {fullActivity ? "Show recent activity" : "View activity"}
          </button>
        )}
      </section>
    </section>
  );
}
/**
 * A capacity hold is the reason work is not moving, so it belongs beside the
 * work rather than only in Settings. Unrelated configuration stays out of this
 * flow, and no account identifier or credential is shown.
 */
function WorkerUsageHold({
  project,
  integrations,
}: {
  project: Project;
  integrations: State["integrations"];
}) {
  // Managed worker profiles are identified by their project; see the daemon's
  // worker preparation, which names them "managed-<project id>".
  const usage = integrations.find(
    (i) => i.id === `worker-usage:managed-${project.id}`,
  );
  if (!usage || !["paused", "unavailable"].includes(usage.status)) return null;
  return (
    <div className="usage-hold" role="status">
      <Icon name="Clock" size={16} />
      <div>
        <strong>
          {usage.status === "paused"
            ? "New work is held by a usage limit"
            : "Subscription usage could not be read"}
        </strong>
        <p>{usage.detail || "No further detail was recorded."}</p>
      </div>
    </div>
  );
}
function ProjectRow({
  project,
  state,
  onSelect,
}: {
  project: Project;
  state: State;
  onSelect: () => void;
}) {
  const agents = state.agents.filter((a) => a.project_id === project.id);
  const decisions = pendingDecisions(state.decisions).filter(
    (d) => d.project_id === project.id,
  );
  // Lifecycle and execution health are separate facts: an active project can
  // hold blocked work, and only the second is a reason to look now.
  const health = state.attention.find((a) => a.project_id === project.id);
  const heldUp = health && isHeldUp(health.execution);
  return (
    <button className="project-row" onClick={onSelect}>
      <span className="project-symbol">
        <Icon name="Projects" size={19} />
      </span>
      <span className="project-info">
        <strong>{project.title}</strong>
        <span>
          {project.description ||
            "Open to review the desired outcome and acceptance criteria."}
        </span>
        <span className="project-meta">
          {agents.length
            ? `${agents.length} ${agents.length === 1 ? "agent" : "agents"}`
            : "No agents assigned"}
          {decisions.length > 0 && (
            <span className="decision-meta">
              {" "}
              · {decisions.length} decision{decisions.length !== 1 ? "s" : ""}
            </span>
          )}
        </span>
      </span>
      <span className="project-states">
        {heldUp && (
          <Status tone="amber">{stateLabel(health.execution)}</Status>
        )}
        <Status
          tone={
            decisions.length
              ? "amber"
              : project.status === "completed"
                ? "green"
                : ""
          }
        >
          {humanStatus(project.status)}
        </Status>
      </span>
      <Icon name="Chevron" size={15} />
    </button>
  );
}
function Projects({
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
                return health && isHeldUp(health.execution) ? (
                  <Status tone="amber">
                    {stateLabel(health.execution)}
                  </Status>
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
            {criteriaLines(project.acceptance_criteria).length ? (
              <ul className="criteria-list">
                {criteriaLines(project.acceptance_criteria).map((line, i) => (
                  <li key={i}>
                    <span className="criteria-dot" />
                    {line}
                  </li>
                ))}
              </ul>
            ) : (
              <p className="muted">No acceptance criteria recorded.</p>
            )}
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
function DecisionCard({
  decision,
  projects,
  refresh,
  compact = false,
}: {
  decision: Decision;
  projects: Project[];
  refresh: () => Promise<void>;
  compact?: boolean;
}) {
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [mode, setMode] = useState<"answer" | "dismiss" | "">("");
  const [draft, setDraft] = useState("");
  async function choose(
    choice: string,
    action: "choice" | "answer" | "dismiss" = "choice",
  ) {
    setBusy(action === "choice" ? choice : action);
    setError("");
    try {
      await api(
        `/api/decisions/${encodeURIComponent(decision.id)}/${action === "dismiss" ? "dismiss" : "resolve"}`,
        {
          method: "POST",
          body: JSON.stringify(
            action === "dismiss"
              ? { reason: choice }
              : action === "answer"
                ? { answer: choice }
                : { choice },
          ),
        },
      );
      await refresh();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy("");
    }
  }
  const project = projects.find((p) => p.id === decision.project_id);
  return (
    <article className={`decision-card ${compact ? "compact" : ""}`}>
      <div className="decision-topline">
        <span className="eyebrow">YOUR DECISION</span>
        {project && <ProjectLink project={project} />}
      </div>
      <h3>{decision.title}</h3>
      <p className="decision-context">{decision.context}</p>
      {decision.recommendation && (
        <div className="recommendation">
          <Icon name="Arrow" size={17} />
          <div>
            <strong>Recommendation</strong>
            <p>{decision.recommendation}</p>
          </div>
        </div>
      )}
      <div className="decision-actions">
        {(decision.choices || []).map((choice, index) => (
          <button
            className={`button ${index === 0 ? "warm" : "secondary"}`}
            disabled={!!busy}
            key={choice}
            onClick={() => void choose(choice)}
          >
            {busy === choice ? "Recording…" : choice}
            {index === 0 && <Icon name="Arrow" size={14} />}
          </button>
        ))}
        {!decision.choices?.length && (
          <p className="muted">
            Reply in the conversation to discuss this decision.
          </p>
        )}
      </div>
      <div className="decision-actions">
        <button
          className="button secondary"
          disabled={!!busy}
          onClick={() => {
            setMode("answer");
            setDraft("");
            setError("");
          }}
        >
          Give a different answer
        </button>
        <button
          className="button secondary"
          disabled={!!busy}
          onClick={() => {
            setMode("dismiss");
            setDraft("");
            setError("");
          }}
        >
          No longer needed
        </button>
      </div>
      {mode && (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (draft.trim() && !busy) void choose(draft.trim(), mode);
          }}
        >
          <label>
            {mode === "dismiss"
              ? "Why is this no longer needed?"
              : "Your answer"}
            <textarea
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              maxLength={mode === "dismiss" ? 4096 : 16384}
              disabled={!!busy}
              required
              rows={3}
            />
          </label>
          {mode === "dismiss" && (
            <p className="muted">
              Closes this question and records your reason. It does not approve
              work or resume a waiting worker.
            </p>
          )}
          <div className="decision-actions">
            <button
              className="button warm"
              disabled={!!busy || !draft.trim()}
              type="submit"
            >
              {busy
                ? "Recording…"
                : mode === "dismiss"
                  ? "Dismiss decision"
                  : "Record answer"}
            </button>
            <button
              className="button secondary"
              disabled={!!busy}
              type="button"
              onClick={() => setMode("")}
            >
              Cancel
            </button>
          </div>
        </form>
      )}
      <ErrorNotice error={error} />
    </article>
  );
}
function ActivityList({ state, limit }: { state: State; limit?: number }) {
  const groups = groupActivity(state.activity, limit);
  if (!groups.length)
    return (
      <div className="quiet-activity">
        <span className="activity-line" />
        <p>
          No activity yet.
          <span>Updates and completion evidence will appear here.</span>
        </p>
      </div>
    );
  return (
    <ol className="activity-list">
      {groups.map(({ entry, label, count }) => (
        <li key={entry.id}>
          <span className="activity-dot" />
          <div>
            <p>{entry.summary}</p>
            <span>
              {state.projects.some((p) => p.id === entry.project_id) ? (
                <ProjectLink
                  project={state.projects.find(
                    (p) => p.id === entry.project_id,
                  )!}
                />
              ) : (
                label
              )}
              {count > 1 && ` · ${count} updates`}
              {entry.created_at && (
                <>
                  {" "}
                  ·{" "}
                  <time dateTime={entry.created_at}>
                    {dateLabel(entry.created_at)}
                  </time>
                </>
              )}
            </span>
          </div>
        </li>
      ))}
    </ol>
  );
}
function MemoryView({
  state,
  refresh,
}: {
  state: State;
  refresh: () => Promise<void>;
}) {
  const [content, setContent] = useState("");
  const [kind, setKind] = useState("preference");
  const [busy, setBusy] = useState(false);
  // Keyed per memory: acting on one card must not disable every other card.
  const [pending, setPending] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [confirm, setConfirm] = useState<string | null>(null);
  const [correcting, setCorrecting] = useState<string | null>(null);
  const [correction, setCorrection] = useState("");
  async function add(e: FormEvent) {
    e.preventDefault();
    if (!content.trim()) return;
    setBusy(true);
    setError("");
    try {
      await api("/api/memories", {
        method: "POST",
        body: JSON.stringify({ content: content.trim(), kind }),
      });
      setContent("");
      await refresh();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }
  async function correct(id: string) {
    if (!correction.trim()) return;
    setPending(id);
    setError("");
    try {
      await api(`/api/memories/${encodeURIComponent(id)}/correct`, {
        method: "POST",
        body: JSON.stringify({ content: correction.trim() }),
      });
      setCorrecting(null);
      setCorrection("");
      await refresh();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setPending(null);
    }
  }
  async function remove(id: string) {
    setPending(id);
    setError("");
    try {
      await api(`/api/memories/${encodeURIComponent(id)}`, {
        method: "DELETE",
      });
      setConfirm(null);
      await refresh();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setPending(null);
    }
  }
  return (
    <section>
      <PageHeading
        eyebrow="LESS REPEATING YOURSELF"
        title="Memory"
        description="Preferences and context your assistant can carry forward. You can inspect or forget any of it."
      />
      <form className="memory-form" onSubmit={add}>
        <label htmlFor="memory">Something worth remembering</label>
        <textarea
          id="memory"
          rows={3}
          value={content}
          onChange={(e) => setContent(e.target.value)}
          placeholder="For example: bring me a recommendation with each decision, and keep updates brief."
          maxLength={10000}
          required
        />
        <label htmlFor="memory-kind" className="memory-kind">
          What kind of thing is this?
          <select
            id="memory-kind"
            value={kind}
            onChange={(e) => setKind(e.target.value)}
          >
            <option value="preference">
              A standing preference — how you want things done
            </option>
            <option value="observation">
              An observation — true when recorded, may go stale
            </option>
          </select>
        </label>
        <div className="form-bottom">
          <span>
            Saved context informs decisions. It never grants new permissions.
          </span>
          <button className="button primary" disabled={busy || !content.trim()}>
            Remember this
          </button>
        </div>
      </form>
      <ErrorNotice error={error} />
      <section className="section-block">
        <div className="section-heading">
          <h2>
            Remembered context <span>{state.memories.length}</span>
          </h2>
        </div>
        {state.memories.length ? (
          <div className="memories">
            {state.memories.map((m) => {
              const superseded = !!m.superseded_at;
              const replaces = m.supersedes
                ? state.memories.find((other) => other.id === m.supersedes)
                : undefined;
              return (
                <article
                  key={m.id}
                  className={`memory-card ${superseded ? "superseded" : ""}`}
                >
                  <Icon name="Memory" size={17} />
                  <div>
                    <p>{m.content}</p>
                    <small className="memory-facts">
                      {m.kind === "preference"
                        ? "Standing preference"
                        : m.kind === "observation"
                          ? "Observation, true when recorded"
                          : "Uncategorized"}
                      {" · "}
                      {m.source === "owner"
                        ? "From you"
                        : m.source === "assistant"
                          ? "From your assistant"
                          : "Source not recorded"}
                      {" · "}
                      {superseded ? "Corrected " : "Last updated "}
                      {dateLabel(m.superseded_at || m.updated_at)}
                    </small>
                    {replaces && (
                      <small className="memory-facts">
                        Replaces an earlier note kept below.
                      </small>
                    )}
                    {correcting === m.id && (
                      <div className="memory-correction">
                        <label htmlFor={`correct-${m.id}`}>
                          What should it say instead?
                          <textarea
                            id={`correct-${m.id}`}
                            rows={2}
                            maxLength={10000}
                            value={correction}
                            onChange={(e) => setCorrection(e.target.value)}
                          />
                        </label>
                        <p className="field-hint">
                          The original is kept and marked corrected. Nothing is
                          rewritten in place.
                        </p>
                        <div className="forget-actions">
                          <button
                            className="text-button"
                            disabled={pending === m.id || !correction.trim()}
                            onClick={() => void correct(m.id)}
                          >
                            Save correction
                          </button>
                          <button
                            className="text-button"
                            onClick={() => {
                              setCorrecting(null);
                              setCorrection("");
                            }}
                          >
                            Cancel
                          </button>
                        </div>
                      </div>
                    )}
                  </div>
                  {confirm === m.id ? (
                    <div className="forget-actions">
                      <button
                        className="text-button danger"
                        disabled={pending === m.id}
                        onClick={() => void remove(m.id)}
                      >
                        Confirm forget
                      </button>
                      <button
                        className="text-button"
                        onClick={() => setConfirm(null)}
                      >
                        Cancel
                      </button>
                    </div>
                  ) : (
                    !superseded &&
                    correcting !== m.id && (
                      <div className="forget-actions">
                        <button
                          className="text-button"
                          onClick={() => {
                            setCorrecting(m.id);
                            setCorrection(m.content);
                          }}
                        >
                          Correct
                        </button>
                        <button
                          className="text-button"
                          onClick={() => setConfirm(m.id)}
                        >
                          Forget
                        </button>
                      </div>
                    )
                  )}
                </article>
              );
            })}
          </div>
        ) : (
          <Empty icon="Memory" title="A fresh start">
            Add a preference here, or tell your assistant something you'd like
            it to remember.
          </Empty>
        )}
      </section>
    </section>
  );
}
function Settings({
  state,
  refresh,
  control,
}: {
  state: State;
  refresh: () => Promise<void>;
  control: ReactNode;
}) {
  const [config, setConfig] = useState<Config | null>(null);
  const [name, setName] = useState(state.assistant.name);
  const [personality, setPersonality] = useState(state.assistant.personality);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let alive = true;
    api<Config>("/api/config")
      .then((value) => {
        if (alive) setConfig(value);
      })
      .catch((e) => {
        if (alive) setError(errorText(e));
      });
    return () => {
      alive = false;
    };
  }, []);
  async function save(e: FormEvent) {
    e.preventDefault();
    if (!config) return;
    setBusy(true);
    setError("");
    setSaved(false);
    try {
      const next = {
        ...config,
        assistant: { ...config.assistant, name: name.trim(), personality },
      };
      await api("/api/config", { method: "PUT", body: JSON.stringify(next) });
      setConfig(next);
      setSaved(true);
      await refresh();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }
  return (
    <section>
      <PageHeading
        eyebrow="MAKE IT YOURS"
        title="Settings"
        description="A familiar voice, a clear remit, and connections you control."
      />
      <ErrorNotice error={error} />
      <AssistantSetup
        currentName={state.assistant.name}
        demo={state.demo}
        onApplied={async (assistant) => {
          setName(assistant.name || state.assistant.name);
          setPersonality(assistant.personality || "");
          setConfig(await api<Config>("/api/config"));
          await refresh();
        }}
      />
      <form className="settings-form" onSubmit={save}>
        <div className="settings-section-title">
          <Mark small />
          <div>
            <h2>Your assistant</h2>
            <p>Choose the name and character you want to work with.</p>
          </div>
        </div>
        <label htmlFor="assistant-name">
          Name
          <input
            id="assistant-name"
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              setSaved(false);
            }}
            maxLength={80}
            required
          />
        </label>
        <label htmlFor="personality">
          Personality
          <textarea
            id="personality"
            value={personality}
            onChange={(e) => {
              setPersonality(e.target.value);
              setSaved(false);
            }}
            rows={4}
            maxLength={10000}
            placeholder="Calm, direct, curious. Bring a recommendation, not just a question."
          />
        </label>
        <p className="field-hint">
          Personality changes how your assistant communicates. Authority is
          configured separately.
        </p>
        {config && (
          <ThemePicker
            value={config.assistant?.theme}
            onChange={(theme) => {
              setConfig({
                ...config,
                assistant: { ...config.assistant, theme },
              });
              setSaved(false);
            }}
          />
        )}
        {config && (
          <ConnectionsSettings
            connections={config.connections || []}
            onChange={(connections) => {
              setConfig({ ...config, connections });
              setSaved(false);
            }}
          />
        )}
        {config && (
          <ChatSettings
            config={config}
            onChange={(next) => {
              setConfig(next);
              setSaved(false);
            }}
          />
        )}
        {config && (
          <ConfigurationFields
            projects={state.projects}
            config={config}
            onChange={(next) => {
              setConfig(next);
              setSaved(false);
            }}
          />
        )}
        {config && (
          <WorkerSettings
            projects={state.projects}
            workers={config.workers || []}
            onChange={(workers) => {
              setConfig({ ...config, workers });
              setSaved(false);
            }}
          />
        )}
        <div className="settings-save">
          <span role="status">{saved ? "Preferences saved." : ""}</span>
          <button
            className="button primary"
            disabled={busy || !config || !name.trim()}
          >
            {busy ? "Saving…" : "Save preferences"}
          </button>
        </div>
      </form>
      <section className="section-block">
        <div className="section-heading">
          <h2>Connection status</h2>
        </div>
        <p className="section-description">
          Connection credentials stay outside the dashboard. Configure
          credential references through the CLI.
        </p>
        <div className="integrations">
          {state.integrations.length ? (
            state.integrations.map((i) => (
              <div className="integration-row" key={i.id}>
                <span className="integration-symbol">{i.name.slice(0, 1)}</span>
                <div>
                  <strong>{i.name}</strong>
                  <p>{i.detail || "No additional connection details"}</p>
                </div>
                <Status
                  tone={
                    ["connected", "ready", "configured"].includes(i.status)
                      ? "green"
                      : "amber"
                  }
                >
                  {humanStatus(i.status)}
                </Status>
              </div>
            ))
          ) : (
            <div className="integration-empty">
              <Icon name="Settings" />
              <p>
                No connections configured. Run{" "}
                <code>agent-assistant doctor</code> to check setup.
              </p>
            </div>
          )}
        </div>
      </section>
      <section className="section-block">
        <div className="section-heading">
          <h2>Dispatch control</h2>
        </div>
        <p className="section-description">
          Pausing stops new work from being dispatched. Existing agent runs may
          continue.
        </p>
        {control}
      </section>
      <section className="settings-boundaries">
        <Icon name="Lock" size={20} />
        <div>
          <h3>Built-in boundaries</h3>
          <p>
            The assistant coordinates approved agents. It cannot write project
            code, deploy, access production data, or buy things. A personality
            change cannot override these boundaries.
          </p>
        </div>
      </section>
    </section>
  );
}
function Login({
  onSuccess,
  initialError = "",
}: {
  onSuccess: () => Promise<void>;
  initialError?: string;
}) {
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(initialError);
  async function login(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api("/api/session", {
        method: "POST",
        body: JSON.stringify({ token: token.trim() }),
      });
      setToken("");
      await onSuccess();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="login-screen">
      <div className="login-art" aria-hidden="true">
        <div className="orbit one" />
        <div className="orbit two" />
        <div className="orbit three" />
        <Mark />
      </div>
      <main className="login-card">
        <p className="eyebrow">YOUR PRIVATE WORKSPACE</p>
        <h1>A little less to carry.</h1>
        <p>
          Connect to your assistant to see the work clearly and keep it moving.
        </p>
        <form onSubmit={login}>
          <label htmlFor="login-token">
            Dashboard access code
            <input
              id="login-token"
              type="password"
              autoComplete="off"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              autoFocus
              required
            />
          </label>
          <div className="field-hint login-help">
            <p>
              Run this on the computer hosting your assistant to get a code:
            </p>
            <pre>
              <code>agent-assistant dashboard open --print</code>
            </pre>
            <p>
              The code expires after five minutes and works once. It is issued
              on demand, not stored for you to look up.
            </p>
          </div>
          <ErrorNotice error={error} />
          <button
            type="submit"
            className="button primary"
            disabled={busy || !token.trim()}
          >
            {busy ? "Connecting…" : "Open workspace"}
            <Icon name="Arrow" size={16} />
          </button>
        </form>
        <span className="login-private">
          <Icon name="Lock" size={13} /> Agent Assistant · Owner access
        </span>
      </main>
    </div>
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
/**
 * The global setting is a default for workers created later; a project worker
 * can carry its own model. Showing both prevents the settings page and a
 * project page from looking as though they disagree.
 */
function WorkerModelOverrides({
  config,
  projects,
}: {
  config: Config;
  projects: Project[];
}) {
  const fallback = (config.worker_model || {}) as Record<string, unknown>;
  const overridden = (config.workers || []).filter(
    (worker) => worker.managed === true && !!worker.model_profile,
  );
  if (!overridden.length) return null;
  const describe = (model: Record<string, unknown>) =>
    [model.engine, model.model, model.effort && `${model.effort} effort`]
      .filter(Boolean)
      .join(" · ");
  return (
    <div className="worker-overrides">
      <h3>Project workers with their own model</h3>
      <p className="field-hint">
        These projects do not use the default above. The effective model is what
        their next assignment will run.
      </p>
      <dl>
        {overridden.map((worker) => (
          <div key={worker.id}>
            <dt>
              {projects.find((p) => p.id === worker.project_id)?.title ||
                worker.name ||
                worker.id}
            </dt>
            <dd>
              {describe(worker.model_profile as Record<string, unknown>) ||
                "Not recorded"}
            </dd>
          </div>
        ))}
        <div>
          <dt>Default for new workers</dt>
          <dd>{describe(fallback) || "Not recorded"}</dd>
        </div>
      </dl>
    </div>
  );
}
function ConfigurationFields({
  config,
  onChange,
  projects,
}: {
  config: Config;
  onChange: (value: Config) => void;
  projects: Project[];
}) {
  const [listDrafts, setListDrafts] = useState<Record<string, string>>({});
  const linear = (config.linear || {}) as Record<string, unknown>;
  function field(
    group: string,
    key: string,
    label: string,
    options: {
      type?: string;
      hint?: string;
      min?: number;
      max?: number;
      env?: boolean;
      list?: boolean;
    } = {},
  ) {
    const object = (config[group] || {}) as Record<string, unknown>;
    const raw = object[key];
    const value =
      options.list && listDrafts[`${group}.${key}`] !== undefined
        ? listDrafts[`${group}.${key}`]
        : Array.isArray(raw)
          ? raw.join(", ")
          : typeof raw === "string" || typeof raw === "number"
            ? raw
            : "";
    return (
      <label key={`${group}.${key}`} htmlFor={`${group}-${key}`}>
        {label}
        <input
          id={`${group}-${key}`}
          type={options.type || "text"}
          value={value}
          min={options.min}
          max={options.max}
          pattern={options.env ? "[A-Za-z_][A-Za-z0-9_]*" : undefined}
          autoComplete="off"
          onChange={(e) => {
            const text = e.target.value;
            if (options.list)
              setListDrafts({ ...listDrafts, [`${group}.${key}`]: text });
            onChange({
              ...config,
              [group]: {
                ...object,
                [key]: options.list
                  ? text
                      .split(",")
                      .map((x) => x.trim())
                      .filter(Boolean)
                  : options.type === "number"
                    ? Number(text)
                    : text,
              },
            });
          }}
        />
        {options.hint && <span className="field-hint">{options.hint}</span>}
      </label>
    );
  }
  return (
    <div className="configuration-fields">
      <details className="settings-group" open>
        <summary>Models and worker defaults</summary>
        <p className="field-hint">
          Enter environment variable names for credentials. Never paste a token
          or API key. Connection changes may require restarting the daemon.
        </p>
        <ModelSettings
          config={config}
          onChange={onChange}
          group="model"
          title="Assistant"
        />
        <ModelSettings
          config={config}
          onChange={onChange}
          group="worker_model"
          title="Worker"
          legend="Default model for new workers"
        />
        <WorkerModelOverrides config={config} projects={projects} />
      </details>
      <details className="settings-group">
        <summary>Advanced</summary>
        <details className="advanced-connection">
          <summary>Advanced: Slack bot and direct Linear API</summary>
          <p className="field-hint">
            Optional integrations for a dedicated bot identity or direct API
            access. Named CLI connections above are the simpler starting point.
            Bot connection changes require a daemon restart.
          </p>
          <div className="config-field-group">
            <h3>Slack bot</h3>
            {field("slack", "owner_user_id", "Your Slack user ID")}
            {field("slack", "bot_token_env", "Bot token environment variable", {
              env: true,
            })}
            {field("slack", "app_token_env", "App token environment variable", {
              env: true,
            })}
          </div>
          <div className="config-field-group">
            <h3>Linear</h3>
            <label className="profile-choice">
              <input
                type="checkbox"
                checked={linear.import_assignments === true}
                onChange={(e) =>
                  onChange({
                    ...config,
                    linear: {
                      ...linear,
                      import_assignments: e.target.checked,
                    },
                  })
                }
                aria-describedby="linear-import-hint"
              />
              <span>Import assigned issues as projects</span>
            </label>
            <p className="field-hint" id="linear-import-hint">
              Optional. Import your assigned issues from the teams below using
              the direct API. Projects in agent-assistant do not require Linear;
              keep this off unless you want automatic imports from this account.
              Configured Linear CLI connections take precedence; enable imports
              on those connections instead.
            </p>
            {field("linear", "api_key_env", "API key environment variable", {
              env: true,
            })}
            {field("linear", "team_ids", "Watched team IDs", {
              list: true,
              hint: "Separate IDs with commas. Configure only the teams you want the assistant to access.",
            })}
          </div>
        </details>
      </details>
      <details className="settings-group">
        <summary>Capacity and recovery</summary>
        <p className="field-hint">
          These limits apply across coordinated work. Model call counts are an
          operating limit, not a dollar budget.
        </p>
        <div className="config-field-group">
          {field("limits", "max_agents", "Maximum active agents", {
            type: "number",
            min: 1,
            max: 64,
          })}
          {field("limits", "max_depth", "Maximum delegation depth", {
            type: "number",
            min: 1,
            max: 10,
          })}
          {field(
            "limits",
            "max_model_calls_per_day",
            "Maximum model calls per day",
            { type: "number", min: 1, max: 100000 },
          )}
          {field(
            "limits",
            "max_model_turns",
            "Maximum model turns per request",
            { type: "number", min: 1, max: 32 },
          )}
          {field(
            "limits",
            "check_in_minutes",
            "Expected agent check-in (minutes)",
            { type: "number", min: 1, max: 1440 },
          )}
          {field("limits", "max_recoveries", "Maximum recovery attempts", {
            type: "number",
            min: 0,
            max: 10,
          })}
        </div>
        <WorkerUsageSettings config={config} onChange={onChange} />
      </details>
    </div>
  );
}
