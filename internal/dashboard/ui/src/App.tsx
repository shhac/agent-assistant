import { NewProject, ProjectDirectories } from "./ProjectForms";
import { Avatar, Waiting, ThemePicker, validTheme } from "./Identity";
import { AssistantSetup } from "./AssistantSetup";
import { ConnectionsSettings } from "./ConnectionsSettings";
import { ModelSettings } from "./ModelSettings";
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

type Page = "Overview" | "Projects" | "Decisions" | "Memory" | "Settings";
const pages: Page[] = [
  "Overview",
  "Projects",
  "Decisions",
  "Memory",
  "Settings",
];
const icons: Record<string, string> = {
  Overview: "M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h7v7h-7z",
  Projects: "M3 7h7l2-3h9v16H3z",
  Decisions: "M12 3l9 9-9 9-9-9zM12 8v5M12 16h.01",
  Memory: "M6 3h12v18l-6-4-6 4z",
  Settings: "M4 7h16M4 17h16M8 4v6M16 14v6",
  Arrow: "M5 12h14M13 6l6 6-6 6",
  Plus: "M12 5v14M5 12h14",
  Close: "M6 6l12 12M18 6L6 18",
  Send: "M12 19V5M6 11l6-6 6 6",
  Check: "M5 12l4 4L19 6",
  Pause: "M8 5v14M16 5v14",
  Play: "M7 4l14 8-14 8z",
  Message: "M4 4h16v13H9l-5 4z",
  Lock: "M6 10h12v11H6zM8 10V6a4 4 0 018 0v4",
  Chevron: "M9 5l7 7-7 7",
  Expand:
    "M8 3H3v5M16 3h5v5M3 16v5h5M21 16v5h-5M3 3l6 6M21 3l-6 6M3 21l6-6M21 21l-6-6",
  Shrink: "M3 8h5V3M21 8h-5V3M8 21v-5H3M16 21v-5h5",
  Bell: "M5 17h14l-2-3V9a5 5 0 00-10 0v5zM10 21h4",
};
function Icon({ name, size = 18 }: { name: string; size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d={icons[name] || icons.Projects} />
    </svg>
  );
}
function Mark({ small = false }: { small?: boolean }) {
  return (
    <span
      className={`assistant-mark ${small ? "small" : ""}`}
      aria-hidden="true"
    >
      <i />
      <i />
      <i />
      <i />
    </span>
  );
}
function Status({
  children,
  tone = "",
}: {
  children: ReactNode;
  tone?: string;
}) {
  return (
    <span className={`status ${tone}`}>
      <span className="status-dot" />
      {children}
    </span>
  );
}
function ErrorNotice({ error }: { error: string }) {
  return error ? (
    <div className="error-notice" role="alert">
      {error}
    </div>
  ) : null;
}
function humanStatus(value: string) {
  return (value || "pending").replaceAll("_", " ");
}
function dateLabel(value?: string) {
  if (!value) return "";
  const d = new Date(value);
  return Number.isNaN(d.valueOf())
    ? ""
    : d.toLocaleString(undefined, {
        month: "short",
        day: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      });
}
function Empty({
  icon,
  title,
  children,
  action,
}: {
  icon: string;
  title: string;
  children: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="empty-state">
      <span className="empty-icon">
        <Icon name={icon} size={24} />
      </span>
      <h3>{title}</h3>
      <p>{children}</p>
      {action}
    </div>
  );
}

export function App() {
  const [state, setState] = useState<State | null>(null);
  const [page, setPage] = useState<Page>("Overview");
  const [selectedProject, setSelectedProject] = useState<string | null>(null);
  const [authRequired, setAuthRequired] = useState(false);
  const [connectionError, setConnectionError] = useState("");
  const [newProject, setNewProject] = useState(false);
  const [chatOpen, setChatOpen] = useState(false);
  const [chatExpanded, setChatExpanded] = useState(false);
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
  function navigate(next: Page) {
    setChatExpanded(false);
    setPage(next);
    setSelectedProject(null);
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
              onProject={(id) => {
                setSelectedProject(id);
                setPage("Projects");
              }}
              refresh={refresh}
            />
          )}
          {page === "Projects" && (
            <Projects
              state={state}
              selected={selectedProject}
              onSelect={setSelectedProject}
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
              {state.decisions.some((d) => !decisions.includes(d)) && (
                <section className="section-block">
                  <div className="section-heading">
                    <h2>Decision history</h2>
                  </div>
                  {state.decisions
                    .filter((d) => !decisions.includes(d))
                    .map((d) => (
                      <div className="history-row" key={d.id}>
                        <span>
                          <Icon name="Check" size={16} />
                          {d.title}
                        </span>
                        <Status>{humanStatus(d.status)}</Status>
                      </div>
                    ))}
                </section>
              )}
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
        <Chat
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
            setPage("Projects");
            await refresh();
          }}
        />
      )}
    </div>
  );
}
function PageHeading({
  eyebrow,
  title,
  description,
  action,
}: {
  eyebrow: string;
  title: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <div className="page-heading">
      <div>
        <p className="eyebrow">{eyebrow}</p>
        <h1>{title}</h1>
        <p className="page-description">{description}</p>
      </div>
      {action}
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
      <div
        className={`attention-banner ${decisions.length ? "needs-attention" : ""}`}
      >
        <span className="attention-symbol">
          <Icon name={decisions.length ? "Decisions" : "Check"} size={23} />
        </span>
        <div>
          <h2>
            {decisions.length
              ? `${decisions.length} ${decisions.length === 1 ? "decision needs" : "decisions need"} your judgment`
              : "No decisions waiting on you"}
          </h2>
          <p>
            {decisions.length
              ? "Your assistant has gathered the context. You make the call."
              : state.projects.length
                ? "Your assistant will bring you anything that needs a decision."
                : "Start with one meaningful outcome. The coordination happens from there."}
          </p>
        </div>
        {decisions.length > 0 && (
          <button
            className="text-button"
            onClick={() => onNavigate("Decisions")}
          >
            Review <Icon name="Arrow" size={16} />
          </button>
        )}
      </div>
      {decisions.length > 0 && (
        <div className="overview-decision">
          <DecisionCard
            decision={decisions[0]}
            projects={state.projects}
            refresh={refresh}
            compact
          />
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
        <ActivityList state={state} limit={5} />
      </section>
    </section>
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
}: {
  state: State;
  selected: string | null;
  onSelect: (id: string | null) => void;
  onNew: () => void;
  refresh: () => Promise<void>;
}) {
  const project = state.projects.find((p) => p.id === selected);
  if (project) {
    const agents = state.agents.filter((a) => a.project_id === project.id);
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
          action={<Status>{humanStatus(project.status)}</Status>}
        />
        <details className="project-setup-details">
          <summary>Project setup details</summary>
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
            Use this ID when binding an approved worker profile to this project.
          </p>
        </details>
        <ProjectDirectories
          key={project.id}
          project={project}
          refresh={refresh}
        />
        <CoordinateProject project={project} refresh={refresh} />
        <section className="detail-section">
          <p className="eyebrow">WHAT DONE LOOKS LIKE</p>
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
        <section className="section-block">
          <div className="section-heading">
            <h2>Agents and ownership</h2>
          </div>
          {agents.length ? (
            <div className="agent-list">
              {agents.map((a) => (
                <article key={a.id} className="agent-row">
                  <span className="agent-avatar">
                    {a.name?.slice(0, 1) || "A"}
                  </span>
                  <div>
                    <strong>{a.name || a.role}</strong>
                    <small>
                      {a.role} · {a.summary || "No progress summary yet"}
                    </small>
                    {a.last_update && (
                      <small>Last update {dateLabel(a.last_update)}</small>
                    )}
                    {a.next_check_in && (
                      <small>
                        Expected check-in {dateLabel(a.next_check_in)}
                      </small>
                    )}
                    {!!a.evidence?.length && (
                      <ul className="agent-evidence">
                        {a.evidence.map((item, index) => (
                          <li key={index}>{item}</li>
                        ))}
                      </ul>
                    )}
                  </div>
                  <Status>{humanStatus(a.status)}</Status>
                </article>
              ))}
            </div>
          ) : (
            <Empty icon="Projects" title="No agents assigned yet">
              Ask your assistant to coordinate this project. It will use the
              available runtimes and permissions.
            </Empty>
          )}
        </section>
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
  async function choose(choice: string) {
    setBusy(choice);
    setError("");
    try {
      await api(`/api/decisions/${encodeURIComponent(decision.id)}/resolve`, {
        method: "POST",
        body: JSON.stringify({ choice }),
      });
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
        {project && <span>{project.title}</span>}
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
      <ErrorNotice error={error} />
    </article>
  );
}
function ActivityList({ state, limit }: { state: State; limit?: number }) {
  const entries = limit ? state.activity.slice(0, limit) : state.activity;
  if (!entries.length)
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
      {entries.map((entry) => (
        <li key={entry.id}>
          <span className="activity-dot" />
          <div>
            <p>{entry.summary}</p>
            <span>
              {state.projects.find((p) => p.id === entry.project_id)?.title ||
                humanStatus(entry.kind || "workspace")}
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
function Chat({
  state,
  refresh,
  onClose,
  expanded,
  onExpand,
}: {
  state: State;
  refresh: () => Promise<void>;
  onClose: () => void;
  expanded: boolean;
  onExpand: () => void;
}) {
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const scroll = useRef<HTMLDivElement>(null);
  const name = state.assistant.name || "Assistant";
  useEffect(() => {
    if (scroll.current) scroll.current.scrollTop = scroll.current.scrollHeight;
  }, [state.messages.length, busy]);
  async function send(e: FormEvent) {
    e.preventDefault();
    const submitted = message.trim();
    if (!submitted || busy) return;
    setBusy(true);
    setError("");
    try {
      await api("/api/chat", {
        method: "POST",
        body: JSON.stringify({ message: submitted }),
      });
      setMessage("");
      await refresh();
    } catch (err) {
      setError(errorText(err));
      await refresh();
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <header className="chat-header">
        <Avatar avatar={state.assistant.avatar} small />
        <div>
          <h2>{name}</h2>
          <span>Your context, kept together</span>
        </div>
        <button
          className="icon-button chat-expand"
          aria-label={expanded ? "Return to workspace" : "Expand conversation"}
          aria-pressed={expanded}
          onClick={onExpand}
          title={
            expanded ? "Return to workspace" : "Make conversation the main view"
          }
        >
          <Icon name={expanded ? "Shrink" : "Expand"} />
        </button>
        <button
          className="icon-button mobile-close"
          aria-label="Close conversation"
          onClick={onClose}
        >
          <Icon name="Close" />
        </button>
      </header>
      {expanded && (
        <div className="chat-context-strip">
          <span>YOUR WORK, WITH CONTEXT</span>
          <p>
            {state.projects.filter((p) => p.status !== "completed").length}{" "}
            active projects <i>·</i> {pendingDecisions(state.decisions).length}{" "}
            open decisions <i>·</i> one conversation
          </p>
        </div>
      )}
      <div
        className="chat-messages"
        ref={scroll}
        role="log"
        aria-label="Conversation history"
        aria-live="polite"
      >
        {state.messages.length ? (
          state.messages.map((m) => (
            <article
              key={m.id}
              className={`message ${m.role === "user" ? "user-message" : "assistant-message"}`}
            >
              <div className="message-label">
                {m.role === "user" ? "You" : name}
                {m.created_at && (
                  <time dateTime={m.created_at}>{dateLabel(m.created_at)}</time>
                )}
              </div>
              <p>{m.content}</p>
            </article>
          ))
        ) : (
          <div className="chat-welcome">
            <span className="chat-orbit">
              <Avatar avatar={state.assistant.avatar} />
            </span>
            <p className="eyebrow">A LITTLE LESS TO CARRY</p>
            <h3>Start a conversation.</h3>
            <p>
              Share an outcome, ask about your projects, or tell {name} what
              matters to you.
            </p>
            <div className="suggestions">
              {[
                "What needs my attention?",
                "Help me set up a project.",
                "What do you remember about me?",
              ].map((text) => (
                <button
                  key={text}
                  onClick={() => {
                    setMessage(text);
                    document.getElementById("chat-message")?.focus();
                  }}
                >
                  {text}
                  <Icon name="Arrow" size={13} />
                </button>
              ))}
            </div>
          </div>
        )}
        {busy && (
          <Waiting
            label={`${name} is working through it…`}
            detail="Keeping the context together. An answer or a clear decision is on its way."
          />
        )}
      </div>
      <div className="chat-composer-wrap">
        <ErrorNotice error={error} />
        <form className="chat-composer" onSubmit={send}>
          <label className="sr-only" htmlFor="chat-message">
            Message {name}
          </label>
          <textarea
            id="chat-message"
            value={message}
            onChange={(e) => setMessage(e.target.value)}
            placeholder={`Ask ${name}, or hand over an outcome…`}
            rows={3}
            maxLength={20000}
            disabled={busy}
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                e.preventDefault();
                e.currentTarget.form?.requestSubmit();
              }
            }}
          />
          <div className="composer-footer">
            <span>⌘ / Ctrl + Enter to send</span>
            <button
              className="send-button"
              type="submit"
              disabled={busy || !message.trim()}
              aria-label="Send message"
            >
              <Icon name="Send" size={17} />
            </button>
          </div>
        </form>
        <p className="chat-footnote">One conversation across your projects.</p>
      </div>
    </>
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
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [confirm, setConfirm] = useState<string | null>(null);
  async function add(e: FormEvent) {
    e.preventDefault();
    if (!content.trim()) return;
    setBusy(true);
    setError("");
    try {
      await api("/api/memories", {
        method: "POST",
        body: JSON.stringify({ content: content.trim() }),
      });
      setContent("");
      await refresh();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }
  async function remove(id: string) {
    setBusy(true);
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
      setBusy(false);
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
            {state.memories.map((m) => (
              <article key={m.id} className="memory-card">
                <Icon name="Memory" size={17} />
                <div>
                  <p>{m.content}</p>
                  <small>{dateLabel(m.updated_at)}</small>
                </div>
                {confirm === m.id ? (
                  <div className="forget-actions">
                    <button
                      className="text-button danger"
                      disabled={busy}
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
                  <button
                    className="text-button"
                    onClick={() => setConfirm(m.id)}
                  >
                    Forget
                  </button>
                )}
              </article>
            ))}
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
          <ConfigurationFields
            config={config}
            onChange={(next) => {
              setConfig(next);
              setSaved(false);
            }}
          />
        )}
        {config && (
          <WorkerSettings
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
          <p className="field-hint">
            Use the access code from your local daemon setup.
          </p>
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
  async function coordinate() {
    setBusy(true);
    setError("");
    try {
      await api(`/api/projects/${encodeURIComponent(project.id)}/coordinate`, {
        method: "POST",
        body: "{}",
      });
      setSent(true);
      await refresh();
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  if (["completed", "archived", "cancelled"].includes(project.status))
    return null;
  return (
    <div className="coordinate-project">
      <div>
        <p>Ready to hand over the coordination?</p>
        <span>
          Ask your assistant to move this outcome forward with the approved
          agents.
        </span>
      </div>
      <button
        className="button primary"
        disabled={busy}
        onClick={() => void coordinate()}
      >
        {busy ? "Coordinating…" : "Coordinate this project"}
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
function ConfigurationFields({
  config,
  onChange,
}: {
  config: Config;
  onChange: (value: Config) => void;
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
      <details>
        <summary>Assistant and worker models</summary>
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
        />
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
      <details>
        <summary>Capacity and supervision</summary>
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
      </details>
    </div>
  );
}
