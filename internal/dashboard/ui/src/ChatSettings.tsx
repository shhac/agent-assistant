import { useEffect, useState } from "react";
import { api, type Config } from "./api";

type ModelOption = {
  id: string;
  name: string;
  default_effort: string;
  efforts: { id: string; description?: string }[];
};
type Catalog = {
  available: boolean;
  engine: string;
  detail: string;
  models: ModelOption[];
};

export function ChatSettings({
  config,
  onChange,
}: {
  config: Config;
  onChange: (value: Config) => void;
}) {
  const chat = (config.chat || {}) as Record<string, unknown>;
  const phrases = (chat.loading_phrases || {}) as Record<string, unknown>;
  const assistant = (config.model || {}) as Record<string, unknown>;
  const engine = String(assistant.engine || "codex");
  const local = engine === "codex" || engine === "claude";
  const automaticModel = engine === "claude" ? "haiku" : "gpt-5.6-luna";
  const enabled = phrases.enabled !== false;
  const model = String(phrases.model ?? "");
  const effort = String(phrases.effort ?? "low");
  const [advanced, setAdvanced] = useState(false);
  const [catalog, setCatalog] = useState<Catalog | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  const change = (patch: Record<string, unknown>) =>
    onChange({
      ...config,
      chat: { ...chat, loading_phrases: { ...phrases, ...patch } },
    });

  useEffect(() => {
    if (!local || !advanced) return;
    const controller = new AbortController();
    let active = true;
    setLoading(true);
    setError("");
    setCatalog(null);
    api<Catalog>("/api/models?profile=assistant", { signal: controller.signal })
      .then((result) => {
        if (active) setCatalog(result);
      })
      .catch(() => {
        if (active)
          setError(
            "Could not load model options. Your saved selection is unchanged.",
          );
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [local, engine, advanced, revision]);
  const options =
    catalog?.available && catalog.engine === engine ? catalog.models : [];
  const selected = options.find(
    (option) => option.id === (model || automaticModel),
  );
  const efforts = selected?.efforts || [];
  const chooseModel = (id: string) => {
    const option = options.find((item) => item.id === (id || automaticModel));
    const nextEffort =
      !effort || !option || option.efforts.some((item) => item.id === effort)
        ? effort
        : option.default_effort;
    change({ model: id, effort: nextEffort });
  };

  return (
    <fieldset className="config-field-group">
      <legend>Conversation</legend>
      <label className="profile-choice">
        <input
          type="checkbox"
          checked={enabled}
          onChange={(event) => change({ enabled: event.target.checked })}
          aria-describedby="loading-phrases-hint"
        />
        <span>Personalized loading phrases</span>
      </label>
      <p className="field-hint" id="loading-phrases-hint">
        A small model writes a brief loading message using only the last two
        messages, without tools. One extra model request per turn counts toward
        your shared model-call limit. If unavailable, a standard loading message
        appears.
      </p>
      {!local ? (
        <p className="field-hint">
          Your assistant uses an API provider. Loading messages use the standard
          fallback and make no additional model requests.
        </p>
      ) : (
        <>
          <p className="field-hint">
            Uses your assistant’s {engine === "claude" ? "Claude" : "Codex"} CLI
            login. Automatic model: {automaticModel}, low effort.
          </p>
          <details onToggle={(event) => setAdvanced(event.currentTarget.open)}>
            <summary>Loading phrase model</summary>
            <p className="field-hint">
              Optional override. Model options use your saved assistant engine
              and login; save changes to those settings before refreshing.
            </p>
            <label htmlFor="loading-phrase-model">
              Loading phrase model
              <select
                id="loading-phrase-model"
                value={model}
                onChange={(event) => chooseModel(event.target.value)}
                disabled={loading}
              >
                <option value="">Automatic ({automaticModel})</option>
                {model && !options.some((option) => option.id === model) && (
                  <option value={model}>{model} (saved selection)</option>
                )}
                {options.map((option) => (
                  <option key={option.id} value={option.id}>
                    {option.name}
                  </option>
                ))}
              </select>
            </label>
            <label htmlFor="loading-phrase-effort">
              Loading phrase reasoning effort
              <select
                id="loading-phrase-effort"
                value={effort}
                onChange={(event) => change({ effort: event.target.value })}
                disabled={loading || !selected}
              >
                <option value="">
                  Model default
                  {selected?.default_effort
                    ? ` (${selected.default_effort})`
                    : ""}
                </option>
                {effort && !efforts.some((item) => item.id === effort) && (
                  <option value={effort}>{effort} (saved selection)</option>
                )}
                {efforts.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.id}
                  </option>
                ))}
              </select>
            </label>
            <p className="field-hint" role="status">
              {loading
                ? "Looking up available models…"
                : error ||
                  (catalog && catalog.engine !== engine
                    ? "Save your assistant engine change, then refresh model options."
                    : catalog?.detail)}
            </p>
            <button
              type="button"
              className="secondary"
              disabled={loading}
              onClick={() => setRevision((value) => value + 1)}
            >
              Refresh loading model options
            </button>
          </details>
        </>
      )}
    </fieldset>
  );
}
