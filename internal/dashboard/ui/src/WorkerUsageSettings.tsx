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
  const budget = Number(limits.worker_token_budget ?? 0);

  return (
    <>
      <fieldset className="config-field-group">
        <legend>Worker subscription usage</legend>
        <p className="field-hint" id="worker-usage-hint">
          Hold worker model requests when a usage window reaches this
          percentage, including short and weekly windows. This covers starting
          and resuming work and every model invocation a running worker makes.
          Work continues on its own once usage is below the limit. Set 0 to
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
            <option value="pause">
              Hold new work until usage is available
            </option>
          </select>
        </label>
        <p className="field-hint" id="worker-usage-unavailable-hint">
          Usage comes from each managed worker’s configured CLI login. External
          broker accounts cannot be inspected locally, so this setting also
          applies to them. This is headroom on a shared account, not a cap on
          subscription charges, and it does not apply to assistant chat. Account
          usage is re-read about once a minute and the provider caches its own
          figures, so an already-admitted invocation can carry usage past the
          limit.
        </p>
      </fieldset>

      <fieldset className="config-field-group">
        <legend>Worker token budget</legend>
        <label htmlFor="worker-token-budget">
          Tokens per assignment
          <input
            id="worker-token-budget"
            type="number"
            min={0}
            step={1000}
            value={budget}
            onChange={(event) =>
              onChange({
                ...config,
                limits: {
                  ...limits,
                  worker_token_budget: Number(event.target.value),
                },
              })
            }
            aria-describedby="worker-token-budget-hint"
          />
        </label>
        <p className="field-hint" id="worker-token-budget-hint">
          0 disables this budget, which is the default. Otherwise an assignment
          waits for you once the tokens its provider reported — input, including
          cached input, plus output, across its whole life including summaries
          and resumes — reach this number. It is a threshold checked before the
          next model invocation, not a prepaid cap: the invocation that crosses
          it finishes, and a CLI or provider may make more than one upstream
          request inside one invocation. Raise the number and resume to continue
          with the saved work. If a provider reports no usage for a call, that
          consumption cannot be established: the assignment waits rather than
          treating it as free, and only setting this to 0 continues it. This
          counts tokens, not money, and no price is estimated.
        </p>
      </fieldset>
    </>
  );
}
