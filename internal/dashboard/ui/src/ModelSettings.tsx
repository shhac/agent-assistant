import type { Config } from "./api";

export function ModelSettings({
  config,
  onChange,
  group,
  title,
}: {
  config: Config;
  onChange: (value: Config) => void;
  group: "model" | "worker_model";
  title: string;
}) {
  const model = (config[group] || {}) as Record<string, unknown>;
  const value = (key: string) => String(model[key] ?? "");
  const change = (key: string, next: string | number) =>
    onChange({ ...config, [group]: { ...model, [key]: next } });
  const codex = value("engine") === "codex";
  return (
    <fieldset className="config-field-group">
      <legend>{title} model</legend>
      <p className="field-hint">
        {group === "model"
          ? "Used for conversation, coordination and acceptance reviews."
          : "Used by the local worker broker. Restart that broker after changing its profile; external brokers manage their own models."}
      </p>
      <label htmlFor={`${group}-engine`}>
        {title} engine
        <select
          id={`${group}-engine`}
          value={value("engine")}
          onChange={(e) => change("engine", e.target.value)}
        >
          <option value="" disabled>
            Select engine
          </option>
          <option value="codex">Codex CLI</option>
          <option value="openai-compatible">OpenAI-compatible API</option>
        </select>
      </label>
      <label htmlFor={`${group}-model`}>
        {title} model identifier
        <input
          id={`${group}-model`}
          value={value("model")}
          onChange={(e) => change("model", e.target.value)}
          autoComplete="off"
        />
      </label>
      <label htmlFor={`${group}-effort`}>
        {title} reasoning effort
        <select
          id={`${group}-effort`}
          value={value("effort")}
          onChange={(e) => change("effort", e.target.value)}
        >
          <option value="">Provider default</option>
          {[
            "none",
            "minimal",
            "low",
            "medium",
            "high",
            "xhigh",
            "max",
            "ultra",
          ].map((effort) => (
            <option key={effort} value={effort}>
              {effort}
            </option>
          ))}
        </select>
        <span className="field-hint">
          Supported efforts depend on the selected model. Unsupported
          combinations return an error; no other model is silently selected.
        </span>
      </label>
      {codex ? (
        <>
          <label htmlFor={`${group}-codex_bin`}>
            {title} Codex executable
            <input
              id={`${group}-codex_bin`}
              value={value("codex_bin")}
              onChange={(e) => change("codex_bin", e.target.value)}
            />
          </label>
          <p className="field-hint">
            Uses the daemon account’s Codex login. Use a dedicated CODEX_HOME
            without global AGENTS files, then run codex login there. The model
            proposes actions; the daemon controls which tools can execute.
          </p>
        </>
      ) : (
        <>
          <label htmlFor={`${group}-base_url`}>
            {title} provider API base URL
            <input
              type="url"
              id={`${group}-base_url`}
              value={value("base_url")}
              onChange={(e) => change("base_url", e.target.value)}
            />
          </label>
          <label htmlFor={`${group}-api_key_env`}>
            {title} API key environment variable
            <input
              id={`${group}-api_key_env`}
              value={value("api_key_env")}
              onChange={(e) => change("api_key_env", e.target.value)}
              pattern="[A-Za-z_][A-Za-z0-9_]*"
              autoComplete="off"
            />
            <span className="field-hint">
              Enter a variable name, never its secret value.
            </span>
          </label>
        </>
      )}
      {!codex && (
        <label htmlFor={`${group}-max_tokens`}>
          {title} maximum output tokens per call
          <input
            type="number"
            min={128}
            max={group === "worker_model" ? 32768 : 131072}
            id={`${group}-max_tokens`}
            value={value("max_tokens")}
            onChange={(e) => change("max_tokens", Number(e.target.value))}
          />
          <span className="field-hint">
            Separate from reasoning effort and the number of agent turns.
          </span>
        </label>
      )}
      {codex && (
        <p className="field-hint">
          Codex has no per-request output-token cap here. Daemon turn limits and
          process time/output bounds still apply.
        </p>
      )}
    </fieldset>
  );
}
