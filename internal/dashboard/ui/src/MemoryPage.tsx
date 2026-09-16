import { useState, type FormEvent } from "react";
import { dateLabel, Empty, ErrorNotice, Icon, PageHeading } from "./ui";
import { api, errorText, type State } from "./api";

export function MemoryView({
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
