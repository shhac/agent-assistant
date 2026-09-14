import type { WorkerProfile } from "./api";

const capabilities = ["coordinate", "implement", "review", "research"] as const;

export function WorkerSettings({
  workers,
  onChange,
}: {
  workers: WorkerProfile[];
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
    <details className="worker-settings">
      <summary>
        Approved worker profiles <span>{workers.length}</span>
      </summary>
      <p className="field-hint">
        Profiles define where your assistant may delegate. Changes take effect
        when you save preferences. Removing a profile does not stop an existing
        run.
      </p>
      <p className="field-hint">
        The local Docker broker supports implementation and review. Bind it to
        its approved project ID and select implement or review; it does not
        provide a coordinate capability.
      </p>
      {workers.map((worker, index) => (
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
              onChange={(e) => update(index, { api_key_env: e.target.value })}
              pattern="[A-Za-z_][A-Za-z0-9_]*"
              autoComplete="off"
            />
            <span className="field-hint">
              Enter the variable name, never its token value.
            </span>
          </label>
          <label htmlFor={`worker-${index}-project`}>
            Project ID (optional)
            <input
              id={`worker-${index}-project`}
              value={worker.project_id || ""}
              onChange={(e) => update(index, { project_id: e.target.value })}
            />
            <span className="field-hint">
              Required by the local Docker broker. Find the ID under Project
              setup details.
            </span>
          </label>
          <fieldset className="worker-capabilities">
            <legend>Permitted capabilities</legend>
            {capabilities.map((capability) => (
              <label key={capability}>
                <input
                  type="checkbox"
                  checked={worker.capabilities?.includes(capability) || false}
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
      ))}
      <button
        type="button"
        className="button secondary"
        onClick={() =>
          onChange([
            ...workers,
            {
              id: "",
              name: "",
              endpoint: "",
              api_key_env: "",
              capabilities: [],
            },
          ])
        }
      >
        Add worker profile
      </button>
    </details>
  );
}
