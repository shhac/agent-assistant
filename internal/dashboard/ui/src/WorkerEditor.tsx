import { useEffect, useState } from "react";
import { api, errorText } from "./api";
import type { Catalog } from "./ModelSettings";
import type { ProjectWorker } from "./ProjectWorkers";

export function WorkerEditor({
  worker,
  projectID,
  onCancel,
  onSaved,
  disabled,
}: {
  worker: ProjectWorker;
  projectID: string;
  onCancel: () => void;
  onSaved: (worker: ProjectWorker) => Promise<void>;
  disabled: boolean;
}) {
  const [name, setName] = useState(worker.name);
  const [engine, setEngine] = useState(worker.model?.engine || "codex");
  const [model, setModel] = useState(worker.model?.model || "");
  const [effort, setEffort] = useState(worker.model?.effort || "");
  const [catalog, setCatalog] = useState<Catalog | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    let current = true;
    const controller = new AbortController();
    setLoading(true);
    setCatalog(null);
    setError("");
    const query = new URLSearchParams({
      profile: "worker",
      worker_profile: worker.id,
      engine,
    });
    api<Catalog>(`/api/models?${query}`, { signal: controller.signal })
      .then((result) => {
        if (current) setCatalog(result);
      })
      .catch((err) => {
        if (current) setError(errorText(err));
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
      controller.abort();
    };
  }, [worker.id, engine, revision]);
  const options =
    catalog?.available && catalog.engine === engine ? catalog.models : [];
  const chosen = options.find((item) => item.id === model);
  const valid =
    name.trim() &&
    chosen &&
    (!effort || chosen.efforts.some((item) => item.id === effort));
  async function save(event: React.FormEvent) {
    event.preventDefault();
    if (busy || disabled || !valid) return;
    setBusy(true);
    setError("");
    try {
      const result = await api<ProjectWorker>(
        `/api/projects/${encodeURIComponent(projectID)}/workers/${encodeURIComponent(worker.id)}`,
        {
          method: "PUT",
          body: JSON.stringify({ name: name.trim(), engine, model, effort }),
        },
      );
      await onSaved(result);
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  }
  return (
    <form
      className="worker-editor"
      onSubmit={save}
      aria-label={`Edit ${worker.name}`}
    >
      <p className="field-hint">
        Choose a model available to this worker's CLI login. Changes apply to
        future assignments.
      </p>
      <fieldset disabled={disabled || busy}>
        <legend>Worker settings</legend>
        <label>
          Worker name
          <input
            value={name}
            onChange={(event) => setName(event.target.value)}
            required
            maxLength={120}
          />
        </label>
        <label>
          Worker engine
          <select
            value={engine}
            onChange={(event) => {
              setEngine(event.target.value);
              setModel("");
              setEffort("");
            }}
          >
            <option value="codex">Codex CLI</option>
            <option value="claude">Claude Code CLI</option>
          </select>
        </label>
        <label>
          Worker model
          <select
            value={model}
            disabled={loading || !options.length}
            onChange={(event) => {
              const next = options.find(
                (item) => item.id === event.target.value,
              );
              setModel(event.target.value);
              setEffort(next?.default_effort || "");
            }}
          >
            {!chosen && (
              <option value={model}>
                {model ? `${model} (saved selection)` : "Choose a model"}
              </option>
            )}
            {options.map((item) => (
              <option value={item.id} key={item.id}>
                {item.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Worker effort
          <select
            value={effort}
            disabled={loading || !chosen}
            onChange={(event) => setEffort(event.target.value)}
          >
            <option value="">Model default</option>
            {effort && !chosen?.efforts.some((item) => item.id === effort) && (
              <option value={effort}>{effort} (saved selection)</option>
            )}
            {chosen?.efforts.map((item) => (
              <option value={item.id} key={item.id}>
                {item.id}
              </option>
            ))}
          </select>
        </label>
        <p className="field-hint" role="status">
          {loading ? "Looking up available models…" : catalog?.detail}
        </p>
        {!loading && !chosen && model && (
          <p className="field-hint">
            The saved model is not in this catalog. Choose an available model to
            save settings.
          </p>
        )}
        <button
          className="text-button"
          type="button"
          disabled={loading}
          onClick={() => setRevision((value) => value + 1)}
        >
          Refresh model options
        </button>
      </fieldset>
      {error && (
        <div role="alert" className="error-notice">
          {error}
        </div>
      )}
      {disabled && (
        <p className="field-hint">
          Editing is no longer available. Finish or resolve active work before
          changing this worker.
        </p>
      )}
      <div className="worker-editor-actions">
        <button
          className="button secondary"
          type="button"
          disabled={busy}
          onClick={onCancel}
        >
          Cancel editing
        </button>
        <button
          className="button primary"
          type="submit"
          disabled={busy || disabled || loading || !valid}
        >
          {busy ? "Saving…" : "Save worker settings"}
        </button>
      </div>
    </form>
  );
}
