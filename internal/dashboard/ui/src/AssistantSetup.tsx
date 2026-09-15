import { useEffect, useRef, useState, type FormEvent } from "react";
import { api, errorText, type Config } from "./api";
import { Avatar, Waiting, themes, type AvatarSpec } from "./Identity";
interface Recommendation {
  id: string;
  name: string;
  personality: string;
  theme: string;
  avatar: AvatarSpec;
  rationale: string;
  avatar_svg?: string;
  applied?: boolean;
}
interface SetupState {
  messages: { role: string; content: string }[];
  recommendation?: Recommendation;
  questions: string[];
}
export function AssistantSetup({
  currentName,
  onApplied,
  demo,
}: {
  currentName: string;
  onApplied: (assistant: NonNullable<Config["assistant"]>) => Promise<void>;
  demo: boolean;
}) {
  const [state, setState] = useState<SetupState>({
    messages: [],
    questions: [],
  });
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [applying, setApplying] = useState(false);
  const [error, setError] = useState("");
  const [applied, setApplied] = useState(false);
  const log = useRef<HTMLDivElement>(null);
  useEffect(() => {
    let active = true;
    api<SetupState>("/api/setup")
      .then((value) => {
        if (active)
          setState({
            ...value,
            messages: value.messages || [],
            questions: value.questions || [],
          });
      })
      .catch((e) => {
        if (active) setError(errorText(e));
      });
    return () => {
      active = false;
    };
  }, []);
  useEffect(() => {
    if (log.current) log.current.scrollTop = log.current.scrollHeight;
  }, [state.messages.length, busy]);
  async function interview(e?: FormEvent) {
    e?.preventDefault();
    if (busy || demo) return;
    setBusy(true);
    setError("");
    setApplied(false);
    try {
      const next = await api<SetupState>("/api/setup/interview", {
        method: "POST",
        body: JSON.stringify({ message: message.trim() }),
      });
      setState({
        ...next,
        messages: next.messages || [],
        questions: next.questions || [],
      });
      setMessage("");
    } catch (e) {
      setError(errorText(e));
    } finally {
      setBusy(false);
    }
  }
  async function apply() {
    if (!state.recommendation) return;
    setApplying(true);
    setError("");
    try {
      const assistant = await api<NonNullable<Config["assistant"]>>(
        "/api/setup/apply",
        {
          method: "POST",
          body: JSON.stringify({
            recommendation_id: state.recommendation.id,
            accepted: true,
          }),
        },
      );
      await onApplied(assistant);
      setApplied(true);
    } catch (e) {
      setError(errorText(e));
    } finally {
      setApplying(false);
    }
  }
  const proposal = state.recommendation;
  const isApplied = applied || proposal?.applied === true;
  return (
    <section className="setup-studio" aria-labelledby="setup-title">
      <div className="setup-intro">
        <span className="eyebrow">A GOOD WORKING RELATIONSHIP</span>
        <h2 id="setup-title">Meet your assistant.</h2>
        <p>
          A few preferences are enough. Your assistant can suggest a name, a
          voice and a look that fit how you like to work.
        </p>
      </div>
      {error && (
        <div className="error-notice" role="alert">
          {error}
        </div>
      )}
      {demo && (
        <p className="field-hint">
          This is a preview workspace. The guided conversation uses your
          configured model outside demo mode; manual preferences are available
          below.
        </p>
      )}
      {state.messages.length > 0 && (
        <div
          className="setup-dialogue"
          role="log"
          aria-label="Assistant setup conversation"
          ref={log}
        >
          {state.messages.map((m, i) => (
            <article
              key={i}
              className={m.role === "user" ? "setup-answer" : "setup-question"}
            >
              <span>
                {m.role === "user" ? "You" : currentName || "Your assistant"}
              </span>
              <p>{m.content}</p>
            </article>
          ))}
        </div>
      )}
      {!state.messages.length && !busy && (
        <div className="setup-start">
          <Avatar />
          <div>
            <h3>How do you want us to work together?</h3>
            <p>
              We can talk through your preferences before you choose anything.
            </p>
          </div>
          <button
            type="button"
            className="button primary"
            disabled={demo}
            onClick={() => void interview()}
          >
            Let’s get acquainted
          </button>
        </div>
      )}
      {busy && (
        <Waiting
          label="Considering what suits you"
          detail="A thoughtful suggestion, based on your preferences."
        />
      )}
      {state.messages.length > 0 && (
        <form className="setup-reply" onSubmit={interview}>
          <label htmlFor="setup-answer">
            Your preferences
            <textarea
              id="setup-answer"
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              rows={2}
              maxLength={12000}
              placeholder="I like concise updates, a calm tone, and a little personality…"
              disabled={busy || demo}
            />
          </label>
          <button
            className="button secondary"
            disabled={busy || demo || !message.trim()}
          >
            Continue conversation
          </button>
        </form>
      )}
      {proposal && (
        <div className="identity-proposal">
          <div className="proposal-heading">
            <Avatar avatar={proposal.avatar} />
            <div>
              <span className="eyebrow">A SUGGESTION FOR YOU</span>
              <h3>{proposal.name}</h3>
              <span>
                {themes.find((t) => t.id === proposal.theme)?.name ||
                  proposal.theme}
              </span>
            </div>
          </div>
          <p className="proposal-personality">{proposal.personality}</p>
          <p className="field-hint">{proposal.rationale}</p>
          <div className="proposal-actions">
            <span role="status">
              {isApplied
                ? "Your assistant’s identity is saved."
                : "Nothing changes until you apply this suggestion."}
            </span>
            <button
              type="button"
              className="button primary"
              disabled={applying || busy || isApplied || demo}
              onClick={() => void apply()}
            >
              {applying
                ? "Applying…"
                : isApplied
                  ? "Applied"
                  : "Use this identity"}
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
