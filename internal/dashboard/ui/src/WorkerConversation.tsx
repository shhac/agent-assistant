import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { api, APIError, errorText, type Agent } from "./api";
import { ConversationMarkdown } from "./ConversationMarkdown";
import { stateDetail } from "./states";
import { fullDateLabel } from "./ui";
import "./work.css";

type Action = "pause" | "resume" | "stop";
type Controls = Record<Action | "message", boolean> & { reason?: string };
type Entry = {
  sequence: number;
  id: string;
  agent_id: string;
  kind: string;
  direction: string;
  content: string;
  created_at: string;
};
type Conversation = {
  messages: Entry[];
  next_cursor: number;
  has_more: boolean;
  oldest_sequence?: number;
  truncated?: boolean;
  history_limited?: boolean;
  controls: Controls;
};
/**
 * The technical account of what an assignment consumed, kept behind a summary
 * because most of the time the headline number is enough. Calls the provider
 * reported no usage for are named rather than folded into the total: a total
 * that is a lower bound must not read as a complete one.
 */
function resourceDetail(agent: Agent): string {
  const input = agent.usage_input_tokens ?? 0;
  const output = agent.usage_output_tokens ?? 0;
  const unknown = agent.usage_unknown_calls ?? 0;
  const budget = agent.token_budget ?? 0;
  if (!input && !output && !unknown) return "";
  const parts = [
    `${input.toLocaleString()} input tokens (including cached input, as the provider reports it)`,
    `${output.toLocaleString()} output tokens`,
  ];
  if (budget > 0) {
    parts.push(`budget ${budget.toLocaleString()} tokens for this assignment`);
  } else {
    parts.push("no token budget configured");
  }
  if (unknown > 0) {
    parts.push(
      `${unknown} call${unknown === 1 ? "" : "s"} reported no usage, so the total above is a lower bound`,
    );
  }
  return `${parts.join(" · ")}. Tokens, not money; no cost is estimated.`;
}

export function WorkerConversation({
  agent,
  refresh,
  demo,
  outcomeTitle,
}: {
  agent: Agent;
  refresh: () => Promise<void>;
  demo: boolean;
  outcomeTitle?: string;
}) {
  const [open, setOpen] = useState(false);
  return (
    <div className="worker-conversation">
      <button
        type="button"
        className="text-button"
        aria-expanded={open}
        aria-label={`${open ? "Hide conversation and controls" : "Conversation and controls"} for ${agent.name}`}
        onClick={() => setOpen(!open)}
      >
        {open ? "Hide conversation and controls" : "Conversation and controls"}
      </button>
      {open && (
        <WorkerConversationPanel
          agent={agent}
          refresh={refresh}
          demo={demo}
          outcomeTitle={outcomeTitle}
        />
      )}
    </div>
  );
}
function WorkerConversationPanel({
  agent,
  refresh,
  demo,
  outcomeTitle,
}: {
  agent: Agent;
  refresh: () => Promise<void>;
  demo: boolean;
  outcomeTitle?: string;
}) {
  const [messages, setMessages] = useState<Entry[]>([]);
  const [controls, setControls] = useState<Controls | null>(null);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [historyError, setHistoryError] = useState("");
  const [historyLimited, setHistoryLimited] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [draft, setDraft] = useState("");
  const [status, setStatus] = useState(agent.status);
  const [truncated, setTruncated] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [loadingOlder, setLoadingOlder] = useState(false);
  const oldest = useRef(0);
  const cursor = useRef(0);
  const fetching = useRef(false);
  const alive = useRef(true);
  const pendingControl = useRef<{
    action: Action;
    operation_id: string;
  } | null>(null);
  const pendingMessage = useRef<{ message_id: string; message: string } | null>(
    null,
  );
  const list = useRef<HTMLOListElement>(null);
  const pinned = useRef(true);
  const newest = useRef(0);
  const path = `/api/agents/${encodeURIComponent(agent.id)}`;
  useEffect(() => {
    setStatus(agent.status);
  }, [agent.status]);
  const load = useCallback(async () => {
    if (fetching.current) return;
    fetching.current = true;
    try {
      const page = await api<Conversation>(
        `${path}/conversation?after=${cursor.current}&limit=50`,
      );
      if (!alive.current) return;
      setMessages((old) =>
        [
          ...new Map(
            [...old, ...(page.messages || [])].map((entry) => [
              entry.sequence,
              entry,
            ]),
          ).values(),
        ].sort((a, b) => a.sequence - b.sequence),
      );
      cursor.current = Math.max(cursor.current, page.next_cursor || 0);
      if (
        page.oldest_sequence &&
        (!oldest.current || page.oldest_sequence < oldest.current)
      )
        oldest.current = page.oldest_sequence;
      setControls(page.controls || null);
      setHasMore(!!page.has_more);
      if (page.truncated) setTruncated(true);
      if (page.history_limited) setHistoryLimited(true);
      setLoadError("");
    } catch (err) {
      if (alive.current) {
        setLoadError(errorText(err));
        setControls(null);
      }
    } finally {
      fetching.current = false;
      if (alive.current) setLoading(false);
    }
  }, [path]);
  useEffect(() => {
    alive.current = true;
    void load();
    const timer = window.setInterval(() => void load(), 5000);
    return () => {
      alive.current = false;
      window.clearInterval(timer);
    };
  }, [load]);
  // The newest entry is the one an owner opens this panel to read. Older pages
  // are prepended, so only a growing tail scrolls, and only while the owner has
  // not scrolled back through the history themselves.
  useEffect(() => {
    const box = list.current;
    if (!box || !messages.length) return;
    const latest = messages[messages.length - 1].sequence;
    if (latest <= newest.current) return;
    const first = newest.current === 0;
    newest.current = latest;
    if (first || pinned.current) box.scrollTop = box.scrollHeight;
  }, [messages]);
  async function earlier() {
    if (!oldest.current || loadingOlder) return;
    setLoadingOlder(true);
    try {
      const page = await api<Conversation>(
        `${path}/conversation?before=${oldest.current}&limit=50`,
      );
      if (!alive.current) return;
      setMessages((old) =>
        [
          ...new Map(
            [...(page.messages || []), ...old].map((entry) => [
              entry.sequence,
              entry,
            ]),
          ).values(),
        ].sort((a, b) => a.sequence - b.sequence),
      );
      if (page.oldest_sequence)
        oldest.current = Math.min(oldest.current, page.oldest_sequence);
      setTruncated(!!page.truncated);
      if (page.history_limited) setHistoryLimited(true);
      setHistoryError("");
    } catch (err) {
      if (alive.current) setHistoryError(errorText(err));
    } finally {
      if (alive.current) setLoadingOlder(false);
    }
  }
  async function control(action: Action) {
    if (busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    pendingControl.current ||= { action, operation_id: crypto.randomUUID() };
    try {
      const updated = await api<Agent>(`${path}/control`, {
        method: "POST",
        body: JSON.stringify(pendingControl.current),
      });
      pendingControl.current = null;
      setStatus(updated.status || agent.status);
      setControls(null);
      setNotice(
        action === "pause"
          ? updated.status === "paused"
            ? "Worker paused. The current operation finished and progress was saved."
            : "Pause requested. The worker finishes the current operation and saves progress; wait for confirmation."
          : action === "stop"
            ? updated.status === "cancelled"
              ? "Worker stopped. This assignment has ended and cannot resume. Cleanup is confirmed; the outcome remains unaccepted."
              : "Stop requested. This ends the assignment and it cannot resume; wait for cleanup confirmation."
            : "Resume requested for the existing assignment.",
      );
      await load();
      try {
        await refresh();
      } catch {
        setNotice(
          "Request recorded. Refresh the project to see its latest state.",
        );
      }
    } catch (err) {
      if (err instanceof APIError && err.status >= 400 && err.status < 500) {
        pendingControl.current = null;
        setControls(null);
        await load();
      }
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }
  async function send(event: FormEvent) {
    event.preventDefault();
    if (busy || !draft.trim()) return;
    setBusy(true);
    setError("");
    setNotice("");
    pendingMessage.current ||= {
      message_id: crypto.randomUUID(),
      message: draft.trim(),
    };
    try {
      await api(`${path}/messages`, {
        method: "POST",
        body: JSON.stringify(pendingMessage.current),
      });
      pendingMessage.current = null;
      setDraft("");
      setNotice(
        "Direction recorded for this outcome. The agent must explicitly acknowledge that it has read and considered it; recording is not completion.",
      );
      await load();
      try {
        await refresh();
      } catch {
        /* Recorded delivery must never become a retry. */
      }
    } catch (err) {
      if (err instanceof APIError && err.status >= 400 && err.status < 500)
        pendingMessage.current = null;
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }
  const pendingState = [
    "pause_requested",
    "stop_requested",
    "resuming",
  ].includes(status);
  const usageDetail = resourceDetail(agent);
  const uncertain = pendingControl.current || pendingMessage.current;
  return (
    <section
      className="worker-conversation-panel"
      aria-label={`Conversation with ${agent.name}`}
    >
      <div className="work-heading">
        <h4>{agent.name}</h4>
        <span role="status">{stateDetail(status)}</span>
      </div>
      <p className="field-hint">
        Assistant instructions, worker reports, questions and control events for
        this assignment. Only recorded exchanges appear; earlier work may
        predate conversation recording. Private model reasoning is not included.
      </p>
      {agent.status === "retry_wait" && fullDateLabel(agent.retry_at) && (
        <p role="status" className="field-hint">
          Next provider retry after {fullDateLabel(agent.retry_at)}. Saved work
          is preserved.
        </p>
      )}
      {agent.status === "usage_wait" && (
        <p role="status" className="field-hint">
          {agent.resource_hold_owner_action
            ? "Waiting for your decision about worker resources. Saved work and conversation are preserved; nothing failed."
            : fullDateLabel(agent.resource_hold_resets_at)
              ? `Waiting for the account allowance to reset after ${fullDateLabel(agent.resource_hold_resets_at)}. Work continues by itself; saved work is preserved.`
              : "Waiting for worker resources. Work continues by itself once they are available; saved work is preserved."}
        </p>
      )}
      {usageDetail && (
        <details className="field-hint">
          <summary>Resource use</summary>
          <p>{usageDetail}</p>
        </details>
      )}
      {!!agent.context_compactions && (
        <p className="field-hint">
          {agent.context_compactions} context checkpoint
          {agent.context_compactions === 1 ? "" : "s"} saved. The full worker
          transcript is retained.
        </p>
      )}
      {agent.provider_failure_kind && (
        <details className="field-hint">
          <summary>Model failure details</summary>
          <p>
            {agent.provider_failure_kind.replaceAll("_", " ")}
            {agent.model_failure_engine
              ? ` · ${agent.model_failure_engine}`
              : ""}
            {agent.model_failure_phase ? ` · ${agent.model_failure_phase}` : ""}
          </p>
          {agent.model_failure_code && (
            <p>
              Diagnostic: <code>{agent.model_failure_code}</code>
            </p>
          )}
          {agent.model_exit_code !== undefined && (
            <p>CLI exit code: {agent.model_exit_code}</p>
          )}
          {!agent.model_failure_code && (
            <p>No detailed diagnostic was recorded for this attempt.</p>
          )}
        </details>
      )}
      <div className="worker-controls" aria-label="Worker controls">
        <button
          type="button"
          className="button secondary"
          disabled={
            demo || busy || pendingState || !!uncertain || !controls?.pause
          }
          onClick={() => void control("pause")}
        >
          Pause worker
        </button>
        <button
          type="button"
          className="button secondary"
          disabled={
            demo || busy || pendingState || !!uncertain || !controls?.resume
          }
          onClick={() => void control("resume")}
        >
          Resume worker
        </button>
        <button
          type="button"
          className="button secondary"
          disabled={
            demo ||
            busy ||
            status === "stop_requested" ||
            status === "resuming" ||
            !!uncertain ||
            !controls?.stop
          }
          onClick={() => void control("stop")}
        >
          Stop worker
        </button>
      </div>
      <p className="field-hint">
        Pause finishes the current operation and saves progress so the worker
        can resume. Stop ends this assignment and it cannot resume. Neither
        action accepts the outcome.
      </p>
      {controls?.reason && <p className="field-hint">{controls.reason}</p>}
      {pendingControl.current && !busy && (
        <button
          type="button"
          className="button secondary"
          onClick={() => void control(pendingControl.current!.action)}
        >
          Retry {pendingControl.current.action} request
        </button>
      )}
      {error && (
        <p role="alert" className="error-notice">
          {error}
          {uncertain
            ? " Outcome unconfirmed. Retry the same request safely."
            : ""}
        </p>
      )}
      {notice && <p role="status">{notice}</p>}
      {loadError && (
        <p role="alert" className="error-notice">
          {loadError} Controls are unavailable until the worker state can be
          refreshed.
        </p>
      )}
      {historyError && (
        <p role="alert" className="error-notice">
          Earlier messages could not be loaded: {historyError}
        </p>
      )}
      {historyLimited && (
        <p className="field-hint">
          Older conversation entries are no longer retained. This is a partial
          history.
        </p>
      )}
      {loading ? (
        <p role="status">Loading conversation…</p>
      ) : (
        <>
          {truncated && (
            <p className="field-hint">
              Showing recent retained messages. More recorded history is
              available.
            </p>
          )}
          {truncated && (
            <button
              type="button"
              className="text-button"
              disabled={loadingOlder}
              onClick={() => void earlier()}
            >
              {loadingOlder
                ? "Loading earlier messages…"
                : "Load earlier messages"}
            </button>
          )}
          {!messages.length && (
            <p className="muted">
              No conversation has been recorded for this assignment yet.
            </p>
          )}
          <ol
            className="worker-message-list"
            ref={list}
            onScroll={(event) => {
              const box = event.currentTarget;
              pinned.current =
                box.scrollHeight - box.scrollTop - box.clientHeight < 40;
            }}
            tabIndex={0}
            aria-label={`Recorded messages for ${agent.name}`}
          >
            {messages.map((entry) => (
              <li key={entry.sequence}>
                <div className="worker-message-meta">
                  <strong>
                    {entry.kind === "delivery_rejected"
                      ? "Message refused"
                      : entry.kind === "delivery_uncertain"
                        ? "Delivery unconfirmed"
                        : entry.kind === "delivery"
                          ? "Runtime delivery receipt"
                          : entry.kind === "tool"
                            ? "Tool activity"
                            : entry.direction === "daemon_to_worker"
                              ? "Assistant → worker"
                              : entry.direction === "worker_to_daemon"
                                ? "Worker → assistant"
                                : entry.direction === "owner_to_worker"
                                  ? "You → outcome agents"
                                  : entry.kind.replaceAll("_", " ")}
                  </strong>
                  <time dateTime={entry.created_at}>
                    {new Date(entry.created_at).toLocaleString()}
                  </time>
                </div>
                {entry.direction === "worker_to_daemon" ? (
                  <pre className="worker-message-literal">{entry.content}</pre>
                ) : (
                  <ConversationMarkdown content={entry.content} />
                )}
              </li>
            ))}
          </ol>
        </>
      )}
      <button
        type="button"
        className="text-button"
        disabled={loading}
        onClick={() => void load()}
      >
        {hasMore ? "Load next messages" : "Refresh conversation"}
      </button>
      <form className="work-form" onSubmit={(event) => void send(event)}>
        <label>
          Direction for {outcomeTitle || "this assignment’s outcome"}
          <textarea
            rows={2}
            value={draft}
            maxLength={12000}
            disabled={demo || (!controls?.message && !pendingMessage.current)}
            readOnly={busy || !!pendingMessage.current}
            onChange={(event) => setDraft(event.target.value)}
          />
        </label>
        <p className="field-hint">
          This direction is saved with the outcome and shared with its
          responsible agents. It does not silently start a new task.
        </p>
        <button
          className="button secondary"
          disabled={
            demo ||
            busy ||
            !!pendingControl.current ||
            !draft.trim() ||
            (!controls?.message && !pendingMessage.current)
          }
        >
          {busy
            ? "Recording…"
            : pendingMessage.current
              ? "Retry same direction"
              : "Send direction"}
        </button>
      </form>
    </section>
  );
}
