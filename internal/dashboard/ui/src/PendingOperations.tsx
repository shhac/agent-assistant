import { ProjectLink } from "./ProjectLink";
import { useState, type FormEvent } from "react";
import { api, errorText, type PendingOperation, type Project } from "./api";

export function PendingOperations({
  operations,
  projects,
  refresh,
}: {
  operations: PendingOperation[];
  projects: Project[];
  refresh: () => Promise<void>;
}) {
  if (!operations.length) return null;
  return (
    <section className="pending-operations" aria-label="Interrupted operations">
      <div className="section-heading">
        <h2>Interrupted work needs an inspection</h2>
      </div>
      <p className="section-description">
        An operation was interrupted before its result could be confirmed. Check
        the external result and record what you found. Recording an inspection
        never retries the work.
      </p>
      {operations.map((operation) => (
        <OperationInspection
          key={operation.id}
          operation={operation}
          project={projects.find((p) => p.id === operation.project_id)}
          refresh={refresh}
        />
      ))}
    </section>
  );
}
function OperationInspection({
  operation,
  project,
  refresh,
}: {
  operation: PendingOperation;
  project?: Project;
  refresh: () => Promise<void>;
}) {
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!note.trim() || busy) return;
    setBusy(true);
    setError("");
    try {
      await api(
        `/api/operations/${encodeURIComponent(operation.id)}/acknowledge`,
        { method: "POST", body: JSON.stringify({ note: note.trim() }) },
      );
      await refresh();
    } catch (failure) {
      setError(errorText(failure));
    } finally {
      setBusy(false);
    }
  }
  return (
    <form className="operation-inspection" onSubmit={submit}>
      <h3>{operation.summary}</h3>
      {project && (
        <p className="field-hint">
          <ProjectLink project={project} />
        </p>
      )}
      <details>
        <summary>Operation details</summary>
        <code>{operation.id}</code>
      </details>
      <label htmlFor={`inspection-${operation.id}`}>
        What did you find?
        <textarea
          id={`inspection-${operation.id}`}
          value={note}
          onChange={(e) => setNote(e.target.value)}
          required
          maxLength={10000}
          rows={3}
          placeholder="Record the evidence you checked and the operation's actual result."
        />
      </label>
      {error && (
        <div className="error-notice" role="alert">
          {error}
        </div>
      )}
      <button className="button warm" disabled={busy || !note.trim()}>
        {busy ? "Recording…" : "Record inspection"}
      </button>
    </form>
  );
}
