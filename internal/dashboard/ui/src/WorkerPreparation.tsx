import { useEffect, useRef, useState } from "react";
import { api, errorText, type Config, type Project } from "./api";
import { Waiting } from "./Identity";
import "./workers.css";

export function WorkerPreparation(props: { project: Project; demo: boolean }) {
  return <Preparation key={props.project.id} {...props} />;
}
function Preparation({ project, demo }: { project: Project; demo: boolean }) {
  const directories = project.directories || [];
  const [workspace, setWorkspace] = useState(
    directories.length === 1 ? directories[0] : "",
  );
  const [prepared, setPrepared] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const mounted = useRef(true);
  const preparing = useRef(false);
  const selected = directories.includes(workspace)
    ? workspace
    : directories.length === 1
      ? directories[0]
      : "";
  useEffect(() => {
    mounted.current = true;
    let current = true;
    api<Config>("/api/config")
      .then((config) => {
        if (current)
          setPrepared(
            !!config.workers?.some(
              (worker) =>
                worker.managed === true && worker.project_id === project.id,
            ),
          );
      })
      .catch((err) => {
        if (current) setError(errorText(err));
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
      mounted.current = false;
    };
  }, [project.id]);
  async function prepare() {
    if (preparing.current || demo || !selected) return;
    preparing.current = true;
    setBusy(true);
    setError("");
    try {
      await api(`/api/projects/${encodeURIComponent(project.id)}/worker`, {
        method: "POST",
        body: JSON.stringify({ workspace: selected }),
      });
      if (mounted.current) setPrepared(true);
    } catch (err) {
      if (mounted.current) setError(errorText(err));
    } finally {
      preparing.current = false;
      if (mounted.current) setBusy(false);
    }
  }
  return (
    <section
      className="worker-preparation"
      aria-labelledby={`prepare-${project.id}`}
    >
      <h3 id={`prepare-${project.id}`}>
        {prepared
          ? "Your project worker"
          : "Let your assistant prepare the worker"}
      </h3>
      {loading ? (
        <p className="field-hint" role="status">
          Checking project worker…
        </p>
      ) : prepared ? (
        <p className="worker-ready" role="status">
          A worker is prepared for <strong>{project.title}</strong>. Tell your
          assistant what you want to do next to start coordinating work.
        </p>
      ) : (
        <>
          <p className="field-hint">
            Your assistant sets up a private, isolated copy of your project and
            downloads free tools if needed. Preparing the worker does not start
            project work.
          </p>
          {!directories.length ? (
            <p className="field-hint">
              Link a project folder first, then your assistant can prepare its
              worker.
            </p>
          ) : (
            <>
              {directories.length > 1 && (
                <label>
                  Project folder
                  <select
                    value={selected}
                    disabled={busy || demo}
                    onChange={(e) => setWorkspace(e.target.value)}
                  >
                    <option value="">Choose a linked folder</option>
                    {directories.map((path) => (
                      <option key={path} value={path}>
                        {path}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              {directories.length === 1 && (
                <p className="worker-source">
                  Project folder: <span>{directories[0]}</span>
                </p>
              )}
              {busy ? (
                <Waiting
                  label={`Preparing a worker for ${project.title}…`}
                  detail="Your assistant is setting up the environment. The first preparation can take several minutes; you can continue browsing."
                />
              ) : (
                <button
                  type="button"
                  className="button secondary"
                  onClick={prepare}
                  disabled={demo || !selected}
                >
                  Prepare a worker
                </button>
              )}
            </>
          )}
        </>
      )}
      {error && (
        <div className="error-notice" role="alert">
          {error}
        </div>
      )}
      {demo && (
        <p className="field-hint">
          Worker preparation is unavailable in the demo.
        </p>
      )}
    </section>
  );
}
