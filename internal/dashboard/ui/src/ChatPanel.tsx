import { useEffect, useRef, useState, type FormEvent } from "react";
import { Avatar, Waiting } from "./Identity";
import {
  api,
  APIError,
  errorText,
  pendingDecisions,
  type ChatToolEvent,
  type ChatTurn,
  type State,
} from "./api";
import { ConversationMarkdown } from "./ConversationMarkdown";
import { isHeldUp } from "./states";
import "./chat.css";
function Icon({ name, size = 18 }: { name: string; size?: number }) {
  const icons: Record<string, string> = {
    Arrow: "M5 12h14M13 6l6 6-6 6",
    Close: "M6 6l12 12M18 6L6 18",
    Send: "M12 19V5M6 11l6-6 6 6",
    Expand: "M8 3H3v5M16 3h5v5M3 16v5h5M21 16v5h-5",
    Shrink: "M3 8h5V3M21 8h-5V3M8 21v-5H3M16 21v-5h5",
  };
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
      <path d={icons[name]} />
    </svg>
  );
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
type VisibleTurn = Omit<ChatTurn, "status"> & {
  status:
    ChatTurn["status"] | "waiting" | "sending" | "unconfirmed" | "rejected";
};
const active = (turn: VisibleTurn) =>
  ["waiting", "sending", "queued", "running"].includes(turn.status);
function chronological(a: { created_at?: string }, b: { created_at?: string }) {
  return (
    (Date.parse(a.created_at || "") || 0) -
    (Date.parse(b.created_at || "") || 0)
  );
}
function latestTurn(
  current: VisibleTurn | undefined,
  incoming: ChatTurn,
): VisibleTurn {
  const rank = (status: VisibleTurn["status"]) =>
    status === "waiting" ||
    status === "sending" ||
    status === "unconfirmed" ||
    status === "rejected"
      ? 0
      : status === "queued"
        ? 1
        : status === "running"
          ? 2
          : 3;
  // A poll begun before an acknowledgement must not undo its newer status.
  return current && rank(current.status) > rank(incoming.status)
    ? current
    : incoming;
}
function delivery(turn: VisibleTurn) {
  switch (turn.status) {
    case "waiting":
      return "Waiting to send · kept in this browser";
    case "sending":
      return "Sending…";
    case "unconfirmed":
      return "Delivery unconfirmed";
    case "rejected":
      return "Message not sent";
    case "queued":
      return "Queued · will follow the current reply";
    case "running":
      return "Received · working on your request";
    case "completed":
      return "Reply complete";
    case "cancelled":
      return "Cancelled before starting";
    case "interrupted":
      return "Reply interrupted · not automatically retried";
    case "failed":
      return "Reply failed · not automatically retried";
  }
}
/**
 * Tool activity, weighted by what still needs watching. Running and failed
 * operations stay visible; finished ones collapse into a count so a long
 * successful turn does not bury the reply it produced.
 */
function ToolActivity({ events }: { events: ChatToolEvent[] }) {
  const settled = events.filter((e) => e.status === "completed");
  const notable = events.filter((e) => e.status !== "completed");
  return (
    <div
      className="chat-tools"
      role="group"
      aria-label="Assistant tool activity"
    >
      {!!notable.length && (
        <ul>
          {notable.map((event) => (
            <ToolRow key={event.id} event={event} />
          ))}
        </ul>
      )}
      {!!settled.length && (
        <details className="chat-tools-settled">
          <summary>
            {settled.length} {settled.length === 1 ? "step" : "steps"} completed
          </summary>
          <ul>
            {settled.map((event) => (
              <ToolRow key={event.id} event={event} />
            ))}
          </ul>
        </details>
      )}
    </div>
  );
}
function ToolRow({ event }: { event: ChatToolEvent }) {
  return (
    <li className={`chat-tool chat-tool-${event.status}`}>
      <span className="chat-tool-marker" aria-hidden="true">
        {event.status === "completed"
          ? "✓"
          : ["failed", "interrupted"].includes(event.status)
            ? "!"
            : ""}
      </span>
      <span className="chat-tool-description">
        {event.label || "Using a tool"}
        <small>
          {event.status === "running"
            ? "In progress"
            : event.status === "completed"
              ? "Completed"
              : event.status === "interrupted"
                ? "Outcome unconfirmed"
                : "Failed"}
        </small>
      </span>
      <details>
        <summary>Tool details</summary>
        <code>{event.tool}</code>
      </details>
    </li>
  );
}
export function ChatPanel({
  state,
  refresh,
  onClose,
  expanded,
  onExpand,
  onProjectOpen,
  prefill,
}: {
  state: State;
  refresh: () => Promise<void>;
  onClose: () => void;
  expanded: boolean;
  onExpand: () => void;
  onProjectOpen?: (id: string) => void;
  prefill?: { text: string; nonce: number } | null;
}) {
  const [message, setMessage] = useState("");
  const draftRef = useRef("");
  function setDraft(value: string) {
    draftRef.current = value;
    setMessage(value);
  }
  // A handoff from elsewhere in the workspace proposes a message; it never
  // sends one, so the owner still decides what to ask and when.
  useEffect(() => {
    if (!prefill) return;
    draftRef.current = prefill.text;
    setMessage(prefill.text);
    document.getElementById("chat-message")?.focus();
  }, [prefill]);
  const [turns, setTurns] = useState<VisibleTurn[]>([]);
  const [error, setError] = useState("");
  const [pollError, setPollError] = useState("");
  const [cancelling, setCancelling] = useState<Set<string>>(new Set());
  const turnsRef = useRef(turns);
  turnsRef.current = turns;
  const refreshRef = useRef(refresh);
  refreshRef.current = refresh;
  const submitLocks = useRef(new Set<string>());
  const outgoing = useRef<VisibleTurn[]>([]);
  const delivering = useRef(false);
  const uncertainDelivery = useRef<string | null>(null);
  const confirmed = useRef(new Set<string>());
  const scroll = useRef<HTMLDivElement>(null);
  const name = state.assistant.name || "Assistant";
  const messageIDs = new Set(state.messages.map((m) => m.id));
  const messages = [
    ...state.messages,
    ...turns
      .filter((t) => !t.user_message_id || !messageIDs.has(t.user_message_id))
      .map((t) => ({
        id: t.user_message_id || t.id,
        role: "user",
        content: t.message,
        created_at: t.created_at,
      })),
  ].sort(chronological);
  const turnsByMessage = new Map(
    turns.map((t) => [t.user_message_id || t.id, t]),
  );
  const heldUp = state.attention.filter((a) => isHeldUp(a.execution));
  const running = turns.find((t) => t.status === "running");
  const queueCount = turns.filter((t) => t.status === "queued").length;
  const eventSignature = turns
    .map(
      (t) =>
        `${t.id}:${t.status}:${t.events?.map((e) => `${e.id}:${e.status}`).join(",")}`,
    )
    .join("|");
  useEffect(() => {
    if (scroll.current) scroll.current.scrollTop = scroll.current.scrollHeight;
  }, [messages.length, eventSignature]);
  useEffect(() => {
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    let previousSignature = "";
    async function poll() {
      try {
        const result = await api<{ turns: ChatTurn[] }>("/api/chat/turns");
        if (stopped) return;
        const incoming = result.turns || [];
        incoming.forEach((t) => confirmed.current.add(t.id));
        if (
          uncertainDelivery.current &&
          confirmed.current.has(uncertainDelivery.current)
        ) {
          uncertainDelivery.current = null;
          drainOutgoing();
        }
        const signature = incoming
          .map(
            (t) =>
              `${t.id}:${t.status}:${t.user_message_id}:${t.assistant_message_id}`,
          )
          .join("|");
        setTurns((current) => {
          const merged = new Map(current.map((t) => [t.id, t]));
          incoming.forEach((t) =>
            merged.set(t.id, latestTurn(merged.get(t.id), t)),
          );
          return [...merged.values()].sort(chronological);
        });
        setPollError("");
        if (signature !== previousSignature) {
          await refreshRef.current();
          previousSignature = signature;
        }
      } catch (err) {
        if (!stopped)
          setPollError(
            `Live conversation updates unavailable: ${errorText(err)}`,
          );
      } finally {
        if (!stopped)
          timer = setTimeout(poll, turnsRef.current.some(active) ? 1000 : 3000);
      }
    }
    void poll();
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  }, []);
  function enqueue(turn: VisibleTurn) {
    if (submitLocks.current.has(turn.id)) return;
    submitLocks.current.add(turn.id);
    if (turn.status === "unconfirmed") outgoing.current.unshift(turn);
    else outgoing.current.push(turn);
    drainOutgoing();
  }
  function drainOutgoing() {
    const next = outgoing.current[0];
    if (
      delivering.current ||
      !next ||
      (uncertainDelivery.current && uncertainDelivery.current !== next.id)
    )
      return;
    outgoing.current.shift();
    delivering.current = true;
    void deliver(next);
  }
  async function deliver(turn: VisibleTurn) {
    const wasUnconfirmed = turn.status === "unconfirmed";
    const controller = new AbortController();
    let timedOut = false;
    const acceptanceTimeout = setTimeout(() => {
      timedOut = true;
      controller.abort();
    }, 15_000);
    setTurns((current) =>
      current.map((t) =>
        t.id === turn.id ? { ...t, status: "sending", error: undefined } : t,
      ),
    );
    try {
      const saved = await api<ChatTurn>("/api/chat/messages", {
        method: "POST",
        signal: controller.signal,
        body: JSON.stringify({ id: turn.id, message: turn.message }),
      });
      confirmed.current.add(turn.id);
      if (uncertainDelivery.current === turn.id)
        uncertainDelivery.current = null;
      setTurns((current) =>
        current.map((t) =>
          t.id === turn.id && ["sending", "unconfirmed"].includes(t.status)
            ? saved
            : t,
        ),
      );
    } catch (err) {
      const rejected =
        !wasUnconfirmed &&
        err instanceof APIError &&
        err.status >= 400 &&
        err.status < 500 &&
        err.status !== 408;
      if (!rejected && !confirmed.current.has(turn.id))
        uncertainDelivery.current = turn.id;
      setTurns((current) =>
        current.map((t) =>
          t.id === turn.id && t.status === "sending"
            ? {
                ...t,
                status: rejected ? "rejected" : "unconfirmed",
                error: timedOut
                  ? "Delivery confirmation timed out. Checking whether your message arrived."
                  : errorText(err),
              }
            : t,
        ),
      );
    } finally {
      clearTimeout(acceptanceTimeout);
      submitLocks.current.delete(turn.id);
      delivering.current = false;
      drainOutgoing();
    }
  }
  function send(e: FormEvent) {
    e.preventDefault();
    const submitted = draftRef.current.trim();
    if (!submitted) return;
    const turn: VisibleTurn = {
      id: crypto.randomUUID(),
      message: submitted,
      created_at: new Date().toISOString(),
      status: "waiting",
      events: [],
    };
    setTurns((current) => [...current, turn]);
    setDraft("");
    void enqueue(turn);
  }
  async function cancel(turn: VisibleTurn) {
    if (turn.status === "waiting") {
      outgoing.current = outgoing.current.filter((t) => t.id !== turn.id);
      submitLocks.current.delete(turn.id);
      setTurns((current) => current.filter((t) => t.id !== turn.id));
      return;
    }
    setCancelling((current) => new Set(current).add(turn.id));
    setError("");
    try {
      const saved = await api<ChatTurn>(
        `/api/chat/messages/${encodeURIComponent(turn.id)}`,
        { method: "DELETE" },
      );
      setTurns((current) => current.map((t) => (t.id === turn.id ? saved : t)));
    } catch (err) {
      setError(errorText(err));
    } finally {
      setCancelling((current) => {
        const next = new Set(current);
        next.delete(turn.id);
        return next;
      });
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
          <span className="chat-expand-label">
            {expanded ? "Return to workspace" : "Expand conversation"}
          </span>
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
        {messages.length ? (
          messages.map((m) => (
            <article
              key={m.id}
              className={`message ${m.role === "user" ? "user-message" : "assistant-message"}`}
            >
              <div className="message-label">
                {m.role === "user"
                  ? "You"
                  : m.role === "system"
                    ? "Status"
                    : name}
                {m.created_at && (
                  <time dateTime={m.created_at}>{dateLabel(m.created_at)}</time>
                )}
              </div>
              <ConversationMarkdown
                content={m.content}
                projects={state.projects}
                onProjectOpen={onProjectOpen}
              />
              {turnsByMessage.has(m.id) &&
                (() => {
                  const turn = turnsByMessage.get(m.id)!;
                  return (
                    <div className="chat-turn-status">
                      <span className="message-delivery">{delivery(turn)}</span>
                      {["queued", "waiting"].includes(turn.status) && (
                        <button
                          type="button"
                          className="chat-turn-action"
                          disabled={cancelling.has(turn.id)}
                          onClick={() => void cancel(turn)}
                          aria-label={`Cancel queued message: ${turn.message}`}
                        >
                          Cancel
                        </button>
                      )}
                      {turn.error && (
                        <p className="chat-turn-error" role="alert">
                          {turn.error}
                        </p>
                      )}
                      {turn.status === "unconfirmed" && (
                        <div className="chat-recovery">
                          <p>
                            Your message is kept here while delivery is checked.
                            Retrying delivery cannot start a second reply.
                          </p>
                          <button
                            type="button"
                            onClick={() => void enqueue(turn)}
                          >
                            Retry delivery
                          </button>
                        </div>
                      )}
                      {turn.status === "rejected" && (
                        <div className="chat-recovery">
                          <button
                            type="button"
                            onClick={() => {
                              setDraft(
                                draftRef.current
                                  ? `${draftRef.current}\n\n${turn.message}`
                                  : turn.message,
                              );
                              setTurns((current) =>
                                current.filter((t) => t.id !== turn.id),
                              );
                              document.getElementById("chat-message")?.focus();
                            }}
                          >
                            Restore message to draft
                          </button>
                          <button
                            type="button"
                            onClick={() =>
                              setTurns((current) =>
                                current.filter((t) => t.id !== turn.id),
                              )
                            }
                          >
                            Discard message
                          </button>
                        </div>
                      )}
                      {!!turn.events?.length && (
                        <ToolActivity events={turn.events} />
                      )}
                      {turn.status === "running" && (
                        <Waiting
                          label={
                            turn.model_status ||
                            turn.loading_phrase ||
                            `${name} is working through it…`
                          }
                          detail={
                            turn.retry_at && !turn.retry_at.startsWith("0001-")
                              ? `Next attempt after ${new Date(turn.retry_at).toLocaleTimeString()}. Recorded actions will not be replayed.`
                              : "An answer or a clear decision is on its way."
                          }
                        />
                      )}
                    </div>
                  );
                })()}
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
                    setDraft(text);
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
      </div>
      <div className="chat-composer-wrap">
        {/* A reply is accurate for the moment it was written. This states the
            current state so an older "it is running" is never the only
            status an owner can see after work has stopped. */}
        {!!heldUp.length && (
          <p className="chat-live-status" role="status">
            Live status: {heldUp.length}{" "}
            {heldUp.length === 1 ? "outcome is" : "outcomes are"} not moving.
            Replies above describe the moment they were written.
          </p>
        )}
        {error && (
          <div className="error-notice" role="alert">
            {error}
          </div>
        )}
        {pollError && (
          <p className="chat-update-error" role="status">
            {pollError} Your messages remain saved; reconnecting…
          </p>
        )}
        {turns.some((t) => t.status === "waiting") &&
          turns.some((t) => t.status === "unconfirmed") && (
            <p className="chat-queue-summary">
              Waiting for delivery confirmation before sending the following
              messages. Keep this page open.
            </p>
          )}
        {(running || queueCount > 0) && (
          <p className="chat-queue-summary">
            {queueCount
              ? `${queueCount} ${queueCount === 1 ? "message" : "messages"} queued`
              : "You can keep writing"}{" "}
            · Each message gets its own reply.
          </p>
        )}
        <form className="chat-composer" onSubmit={send}>
          <label className="sr-only" htmlFor="chat-message">
            Message {name}
          </label>
          <textarea
            id="chat-message"
            value={message}
            onChange={(e) => setDraft(e.target.value)}
            placeholder={`Ask ${name}, or hand over an outcome…`}
            rows={3}
            maxLength={20000}
            onKeyDown={(e) => {
              if (
                e.key === "Enter" &&
                !e.shiftKey &&
                !e.nativeEvent.isComposing &&
                e.keyCode !== 229
              ) {
                e.preventDefault();
                e.currentTarget.form?.requestSubmit();
              }
            }}
          />
          <div className="composer-footer">
            <span>Enter to send · Shift + Enter for a new line</span>
            <button
              className="send-button"
              type="submit"
              disabled={!message.trim()}
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
