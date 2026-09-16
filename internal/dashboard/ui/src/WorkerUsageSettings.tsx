import type { Config } from "./api";

export function WorkerUsageSettings({
  config,
  onChange,
}: {
  config: Config;
  onChange: (value: Config) => void;
}) {
  const limits = (config.limits || {}) as Record<string, unknown>;
  const usage = (limits.worker_usage || {}) as Record<string, unknown>;
  const change = (patch: Record<string, unknown>) =>
    onChange({
      ...config,
      limits: { ...limits, worker_usage: { ...usage, ...patch } },
    });

  return (
    <fieldset className="config-field-group">
      <legend>Worker subscription usage</legend>
      <p className="field-hint" id="worker-usage-hint">
        Hold new starts, resumes, and follow-up instructions when a usage window reaches
        this percentage, including short and weekly windows. Existing workers
        continue. Work can start again once usage is below the limit. Set 0 to
        disable an engine’s limit.
      </p>
      {(["codex", "claude"] as const).map((engine) => {
        const key = `${engine}_max_used_percent`;
        return (
          <label key={engine} htmlFor={`worker-usage-${engine}`}>
            {engine === "codex" ? "Codex" : "Claude"} usage limit (%)
            <input
              id={`worker-usage-${engine}`}
              type="number"
              min={0}
              max={100}
              step={1}
              value={Number(usage[key] ?? 90)}
              onChange={(event) =>
                change({ [key]: Number(event.target.value) })
              }
              aria-describedby="worker-usage-hint"
            />
          </label>
        );
      })}
      <label htmlFor="worker-usage-unavailable">
        When usage is unavailable
        <select
          id="worker-usage-unavailable"
          value={String(usage.on_unavailable ?? "allow")}
          onChange={(event) => change({ on_unavailable: event.target.value })}
          aria-describedby="worker-usage-unavailable-hint"
        >
          <option value="allow">Allow new work (default)</option>
          <option value="pause">Hold new work until usage is available</option>
        </select>
      </label>
      <p className="field-hint" id="worker-usage-unavailable-hint">
        Usage comes from each managed worker’s configured CLI login. External
        broker accounts cannot be inspected locally, so this setting also
        applies to them. These limits do not cap subscription charges or
        assistant chat.
      </p>
    </fieldset>
  );
}
