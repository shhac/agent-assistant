import { useEffect, useRef, useState } from "react";
import { api, errorText } from "./api";
import { Icon } from "./ui";

export interface QueuedTurn {
  id: string;
  message: string;
  revision: number;
}

export interface QueueHold {
  turn_id: string;
  reason: string;
  expires_at: string;
}

/** Refreshed well inside the daemon's two-minute lease. */
const HEARTBEAT_MS = 30_000;

/**
 * The queue of messages waiting to run, and the controls for changing it.
 *
 * Changing a queued message holds the queue at that message: it and everything
 * behind it stay put while the owner works, so an order they never saw cannot
 * execute. The hold is the daemon's and lapses on its own, so closing this tab
 * mid-edit releases it rather than stranding the queue.
 *
 * Moving is available from the keyboard as well as by dragging, because a queue
 * that can only be reordered with a pointer is a queue some owners cannot
 * reorder at all.
 */
export function ChatQueue({
  turns,
  revision,
  hold,
  cancelling,
  onCancel,
  onChanged,
}: {
  turns: QueuedTurn[];
  revision: number;
  hold?: QueueHold | null;
  cancelling?: Set<string>;
  onCancel?: (id: string) => void;
  onChanged: () => Promise<void> | void;
}) {
  const [editing, setEditing] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [order, setOrder] = useState<string[] | null>(null);
  const dragging = useRef<string | null>(null);
  const held = useRef<string | null>(null);

  // The lease is the daemon's, so it has to be refreshed while a change is open
  // and released when it is not.
  useEffect(() => {
    const target = editing ?? (order ? turns[0]?.id : undefined);
    if (!target) return;
    const reason = editing ? "editing" : "reordering";
    let stopped = false;
    const take = async () => {
      if (stopped) return;
      try {
        await api(`/api/chat/messages/${encodeURIComponent(target)}/hold`, {
          method: "POST",
          body: JSON.stringify({ reason }),
        });
        held.current = target;
      } catch (err) {
        setError(errorText(err));
      }
    };
    void take();
    const timer = setInterval(() => void take(), HEARTBEAT_MS);
    return () => {
      stopped = true;
      clearInterval(timer);
      const release = held.current;
      held.current = null;
      if (release)
        void api(`/api/chat/messages/${encodeURIComponent(release)}/hold`, {
          method: "DELETE",
        }).catch(() => {
          /* The lease lapses on its own; a failed release is not the owner's problem. */
        });
    };
  }, [editing, order, turns]);

  const shown = order
    ? order
        .map((id) => turns.find((t) => t.id === id))
        .filter((t): t is QueuedTurn => !!t)
    : turns;

  async function save(turn: QueuedTurn) {
    setBusy(true);
    setError("");
    try {
      await api(`/api/chat/messages/${encodeURIComponent(turn.id)}`, {
        method: "PATCH",
        body: JSON.stringify({ message: draft, revision: turn.revision }),
      });
      setEditing(null);
      setDraft("");
      await onChanged();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }

  async function commit(next: string[]) {
    setBusy(true);
    setError("");
    try {
      await api("/api/chat/queue", {
        method: "PUT",
        body: JSON.stringify({ order: next, revision }),
      });
      setOrder(null);
      await onChanged();
    } catch (err) {
      setError(errorText(err));
      setOrder(null);
    } finally {
      setBusy(false);
    }
  }

  function move(id: string, by: number) {
    const current = shown.map((t) => t.id);
    const from = current.indexOf(id);
    const to = from + by;
    if (from < 0 || to < 0 || to >= current.length) return;
    const next = [...current];
    next.splice(to, 0, ...next.splice(from, 1));
    setOrder(next);
    void commit(next);
  }

  if (!turns.length) return null;
  return (
    <section className="chat-queue" aria-label="Queued messages">
      <p className="chat-queue-summary">
        {turns.length} {turns.length === 1 ? "message" : "messages"} queued ·
        Each gets its own reply.
      </p>
      {hold && (
        <p className="chat-queue-held" role="status">
          The queue is paused while you change it. It resumes on its own if you
          leave this open.
        </p>
      )}
      {error && (
        <p className="error-notice" role="alert">
          {error}
        </p>
      )}
      <ol className="chat-queue-list">
        {shown.map((turn, index) => (
          <li
            key={turn.id}
            className="chat-queue-item"
            aria-label={`Queued message ${index + 1}`}
            draggable={editing === null}
            onDragStart={() => {
              dragging.current = turn.id;
              setOrder(shown.map((t) => t.id));
            }}
            onDragOver={(event) => {
              event.preventDefault();
              const from = dragging.current;
              if (!from || from === turn.id) return;
              const current = shown.map((t) => t.id);
              const next = [...current];
              next.splice(
                current.indexOf(turn.id),
                0,
                ...next.splice(current.indexOf(from), 1),
              );
              setOrder(next);
            }}
            onDragEnd={() => {
              dragging.current = null;
              if (order) void commit(order);
            }}
          >
            <span className="chat-queue-position" aria-hidden="true">
              {index + 1}
            </span>
            {editing === turn.id ? (
              <div className="chat-queue-editor">
                <label htmlFor={`queue-edit-${turn.id}`}>
                  Edit queued message
                  <textarea
                    id={`queue-edit-${turn.id}`}
                    rows={3}
                    maxLength={24000}
                    value={draft}
                    onChange={(event) => setDraft(event.target.value)}
                  />
                </label>
                <div className="chat-queue-actions">
                  <button
                    type="button"
                    disabled={busy || !draft.trim()}
                    onClick={() => void save(turn)}
                  >
                    Save
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setEditing(null);
                      setDraft("");
                    }}
                  >
                    Cancel
                  </button>
                </div>
              </div>
            ) : (
              <>
                <p className="chat-queue-text">{turn.message}</p>
                <div className="chat-queue-actions">
                  <button
                    type="button"
                    aria-label={`Move message ${index + 1} earlier`}
                    disabled={busy || index === 0}
                    onClick={() => move(turn.id, -1)}
                  >
                    <Icon name="Arrow" size={13} />
                    Earlier
                  </button>
                  <button
                    type="button"
                    aria-label={`Move message ${index + 1} later`}
                    disabled={busy || index === shown.length - 1}
                    onClick={() => move(turn.id, 1)}
                  >
                    <Icon name="Arrow" size={13} />
                    Later
                  </button>
                  <button
                    type="button"
                    aria-label={`Edit message ${index + 1}`}
                    disabled={busy}
                    onClick={() => {
                      setEditing(turn.id);
                      setDraft(turn.message);
                    }}
                  >
                    Edit
                  </button>
                  {onCancel && (
                    <button
                      type="button"
                      aria-label={`Cancel queued message: ${turn.message}`}
                      disabled={busy || cancelling?.has(turn.id)}
                      onClick={() => onCancel(turn.id)}
                    >
                      Cancel
                    </button>
                  )}
                </div>
              </>
            )}
          </li>
        ))}
      </ol>
    </section>
  );
}
