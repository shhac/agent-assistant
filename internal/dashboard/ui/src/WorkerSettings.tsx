import type { Project, WorkerProfile } from "./api";

import "./workers.css";

const capabilities = ["coordinate", "implement", "review", "research"] as const;

export function WorkerSettings({
  workers,
  onChange,
  projects = [],
}: {
  workers: WorkerProfile[];
  projects?: Project[];
  onChange: (workers: WorkerProfile[]) => void;
}) {
  function update(index: number, patch: Partial<WorkerProfile>) {
    onChange(
      workers.map((worker, i) =>
        i === index ? { ...worker, ...patch } : worker,
      ),
    );
  }
  return (
    <section className="worker-settings">
      <h3>Project workers</h3>
      <p className="field-hint">
        Ask your assistant to prepare a worker from a project folder. It handles
        setup and chooses the configured model for you.
      </p>
      {workers
        .filter((worker) => worker.managed === true)
        .map((worker) => {
          const project = projects.find(
            (project) => project.id === worker.project_id,
          );
          return (
            <div className="managed-worker" key={worker.id}>
              <strong>
                {worker.name ||
                  (project ? `${project.title} worker` : "Project worker")}
              </strong>
              {project ? (
                <a href={`#/projects/${encodeURIComponent(project.id)}`}>
                  {project.title}
                </a>
              ) : (
                <span>Project unavailable</span>
              )}
              <span className="field-hint">
                Prepared and managed by your assistant
              </span>
            </div>
          );
        })}
      {!workers.some((worker) => worker.managed === true) && (
        <p className="field-hint">
          No project workers prepared yet. Open a project to get started.
        </p>
      )}
      <details className="external-worker-settings">
        <summary>Advanced: external workers</summary>
        <p className="field-hint">
          Connect an existing worker service. Changes take effect when you save
          preferences. Removing a profile does not stop an existing run.
        </p>
        {workers.map((worker, index) =>
          worker.managed === true ? null : (
            <fieldset className="worker-profile" key={index}>
              <legend>
                {worker.name || worker.id || `New worker ${index + 1}`}
              </legend>
              <label htmlFor={`worker-${index}-id`}>
                Profile ID
                <input
                  id={`worker-${index}-id`}
                  value={worker.id}
                  onChange={(e) => update(index, { id: e.target.value })}
                  required
                  maxLength={120}
                />
              </label>
              <label htmlFor={`worker-${index}-name`}>
                Display name
                <input
                  id={`worker-${index}-name`}
                  value={worker.name || ""}
                  onChange={(e) => update(index, { name: e.target.value })}
                  maxLength={120}
                />
              </label>
              <label htmlFor={`worker-${index}-endpoint`}>
                Broker endpoint
                <input
                  id={`worker-${index}-endpoint`}
                  type="url"
                  value={worker.endpoint || ""}
                  onChange={(e) => update(index, { endpoint: e.target.value })}
                  required
                />
                <span className="field-hint">
                  An approved HTTPS endpoint, or an HTTP broker on loopback.
                </span>
              </label>
              <label htmlFor={`worker-${index}-key`}>
                API key environment variable
                <input
                  id={`worker-${index}-key`}
                  value={worker.api_key_env || ""}
                  onChange={(e) =>
                    update(index, { api_key_env: e.target.value })
                  }
                  pattern="[A-Za-z_][A-Za-z0-9_]*"
                  autoComplete="off"
                />
                <span className="field-hint">
                  Enter the variable name, never its token value.
                </span>
              </label>
              <label htmlFor={`worker-${index}-project`}>
                Project
                <select
                  id={`worker-${index}-project`}
                  value={worker.project_id || ""}
                  onChange={(e) =>
                    update(index, { project_id: e.target.value })
                  }
                >
                  <option value="">Any project</option>
                  {worker.project_id &&
                    !projects.some(
                      (project) => project.id === worker.project_id,
                    ) && (
                      <option value={worker.project_id}>
                        Unavailable project
                      </option>
                    )}
                  {projects.map((project) => (
                    <option key={project.id} value={project.id}>
                      {project.title}
                    </option>
                  ))}
                </select>
              </label>
              {projects.some((project) => project.id === worker.project_id) && (
                <a
                  href={`#/projects/${encodeURIComponent(worker.project_id || "")}`}
                >
                  Open{" "}
                  {
                    projects.find((project) => project.id === worker.project_id)
                      ?.title
                  }
                </a>
              )}
              <fieldset className="worker-capabilities">
                <legend>Permitted capabilities</legend>
                {capabilities.map((capability) => (
                  <label key={capability}>
                    <input
                      type="checkbox"
                      checked={
                        worker.capabilities?.includes(capability) || false
                      }
                      onChange={(e) =>
                        update(index, {
                          capabilities: e.target.checked
                            ? [...(worker.capabilities || []), capability]
                            : (worker.capabilities || []).filter(
                                (c) => c !== capability,
                              ),
                        })
                      }
                    />
                    {capability}
                  </label>
                ))}
              </fieldset>
              <button
                type="button"
                className="text-button danger"
                onClick={() => onChange(workers.filter((_, i) => i !== index))}
              >
                Remove profile
              </button>
            </fieldset>
          ),
        )}
        <button
          type="button"
          className="button secondary"
          onClick={() =>
            onChange([
              ...workers,
              {
                id: `external-${crypto.randomUUID()}`,
                name: "",
                endpoint: "",
                api_key_env: "",
                capabilities: [],
              },
            ])
          }
        >
          Add external worker
        </button>
      </details>
    </section>
  );
}
