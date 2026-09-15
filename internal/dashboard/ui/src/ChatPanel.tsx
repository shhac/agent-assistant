import { useEffect, useRef, useState, type FormEvent } from "react";
import { Avatar, Waiting } from "./Identity";
import { api, errorText, pendingDecisions, type State } from "./api";
import { ConversationMarkdown } from "./ConversationMarkdown";
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
export function ChatPanel({
  state,
  refresh,
  onClose,
  expanded,
  onExpand,
  onProjectOpen,
}: {
  state: State;
  refresh: () => Promise<void>;
  onClose: () => void;
  expanded: boolean;
  onExpand: () => void;
  onProjectOpen?: (id: string) => void;
}) {
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [pending, setPending] = useState<{
    id: string;
    content: string;
    created_at: string;
    previousIDs: Set<string>;
    status: "sending" | "failed" | "sent";
  } | null>(null);
  const sending = useRef(false);
  const persisted =
    pending &&
    state.messages.find(
      (m) =>
        m.role === "user" &&
        m.content === pending.content &&
        !pending.previousIDs.has(m.id),
    );
  const messages =
    pending && !persisted
      ? [...state.messages, { ...pending, role: "user" }]
      : state.messages;
  const scroll = useRef<HTMLDivElement>(null);
  const name = state.assistant.name || "Assistant";
  useEffect(() => {
    if (scroll.current) scroll.current.scrollTop = scroll.current.scrollHeight;
  }, [messages.length, busy]);
  useEffect(() => {
    if (!busy && pending?.status === "sent" && persisted) setPending(null);
  }, [busy, pending, persisted]);
  async function send(e: FormEvent) {
    e.preventDefault();
    const submitted = message.trim();
    if (!submitted || sending.current || pending) return;
    sending.current = true;
    setBusy(true);
    setError("");
    setPending({
      id: crypto.randomUUID(),
      content: submitted,
      created_at: new Date().toISOString(),
      previousIDs: new Set(state.messages.map((m) => m.id)),
      status: "sending",
    });
    setMessage("");
    try {
      await api("/api/chat", {
        method: "POST",
        body: JSON.stringify({ message: submitted }),
      });
      setPending((p) => p && { ...p, status: "sent" });
    } catch (err) {
      setError(errorText(err));
      setPending((p) => p && { ...p, status: "failed" });
    }
    try {
      await refresh();
    } catch {
      setError(
        (previous) =>
          previous ||
          "The reply could not be refreshed. Refresh the conversation before sending again.",
      );
    } finally {
      sending.current = false;
      setBusy(false);
    }
  }
  async function refreshConversation() {
    try {
      await refresh();
    } catch (err) {
      setError(errorText(err));
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
              {pending && (m.id === pending.id || m.id === persisted?.id) && (
                <span className="message-delivery">
                  {busy
                    ? "Sent · waiting for a reply"
                    : pending.status === "failed"
                      ? persisted
                        ? "Received · reply interrupted"
                        : "Delivery unconfirmed"
                      : "Sent · refreshing conversation"}
                </span>
              )}
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
        {error && (
          <div className="error-notice" role="alert">
            {error}
          </div>
        )}
        {pending && !busy && (
          <div className="chat-recovery">
            <p>
              {pending.status === "failed"
                ? persisted
                  ? "Your message was received. Review any recorded actions before trying again."
                  : "Your message is kept here. Refresh to check whether it arrived before trying again."
                : "Your message was sent. Refresh to load the latest conversation."}
            </p>
            <button type="button" onClick={refreshConversation}>
              Refresh conversation
            </button>
            {pending.status === "failed" && (
              <button
                type="button"
                onClick={() => {
                  setMessage((current) =>
                    current
                      ? `${current}\n\n${pending.content}`
                      : pending.content,
                  );
                  setPending(null);
                  setError("");
                  document.getElementById("chat-message")?.focus();
                }}
              >
                Restore message to draft
              </button>
            )}
          </div>
        )}
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
              disabled={busy || !!pending || !message.trim()}
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
