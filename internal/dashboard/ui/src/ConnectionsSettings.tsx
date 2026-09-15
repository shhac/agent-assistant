import { useEffect, useState } from "react";
import { api, errorText } from "./api";
export interface Connection {
  id: string;
  name: string;
  tool: "lin" | "agent-slack" | "agent-notion" | "agent-fathom";
  profiles: string[];
}
interface ProfileDiscovery {
  tool: string;
  profiles: { name: string }[];
  available: boolean;
  detail: string;
  selectable: boolean;
}
const tools: { id: Connection["tool"]; name: string; description: string }[] = [
  { id: "lin", name: "Linear", description: "Assignments and project context" },
  {
    id: "agent-slack",
    name: "Slack",
    description: "Conversations and team context",
  },
  {
    id: "agent-notion",
    name: "Notion",
    description: "Documents and shared knowledge",
  },
  {
    id: "agent-fathom",
    name: "Fathom",
    description: "Meeting notes and decisions",
  },
];
export function ConnectionsSettings({
  connections,
  onChange,
}: {
  connections: Connection[];
  onChange: (connections: Connection[]) => void;
}) {
  return (
    <section
      className="connections-settings"
      aria-labelledby="connections-title"
    >
      <div className="settings-section-title">
        <span className="connection-heading-symbol" aria-hidden="true">
          ↗
        </span>
        <div>
          <h2 id="connections-title">Your connected accounts</h2>
          <p>
            Keep work and personal accounts distinct. Give each connection a
            name and choose the profiles your assistant can read.
          </p>
        </div>
      </div>
      {!connections.length && (
        <div className="connections-empty">
          <p>Start with the place your project context already lives.</p>
          <span>
            Linear, Slack, Notion and Fathom use the CLI profiles already on
            this computer.
          </span>
        </div>
      )}
      {connections.map((connection, index) => (
        <ConnectionEditor
          key={connection.id}
          connection={connection}
          index={index}
          onChange={(next) =>
            onChange(
              connections.map((value, i) => (i === index ? next : value)),
            )
          }
          onRemove={() => onChange(connections.filter((_, i) => i !== index))}
        />
      ))}
      <button
        type="button"
        className="button secondary"
        onClick={() =>
          onChange([
            ...connections,
            {
              id: `connection-${Math.random().toString(36).slice(2, 10)}`,
              name: "",
              tool: "lin",
              profiles: [],
            },
          ])
        }
      >
        Add a connection
      </button>
      <p className="field-hint">
        Accounts authenticate through their CLI. No credentials are entered
        here. Choose at least one profile for each connection, or remove the
        unfinished connection before saving preferences. Notion will become
        selectable when its CLI supports choosing an account.
      </p>
    </section>
  );
}
function ConnectionEditor({
  connection,
  index,
  onChange,
  onRemove,
}: {
  connection: Connection;
  index: number;
  onChange: (connection: Connection) => void;
  onRemove: () => void;
}) {
  const [discovery, setDiscovery] = useState<ProfileDiscovery | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    let current = true;
    setLoading(true);
    setError("");
    setDiscovery(null);
    api<ProfileDiscovery>(
      `/api/connection-profiles?tool=${encodeURIComponent(connection.tool)}`,
    )
      .then((value) => {
        if (current) setDiscovery(value);
      })
      .catch((e) => {
        if (current) setError(errorText(e));
      })
      .finally(() => {
        if (current) setLoading(false);
      });
    return () => {
      current = false;
    };
  }, [connection.tool, revision]);
  const service = tools.find((t) => t.id === connection.tool);
  const discovered = (discovery?.profiles || []).map((p) => p.name);
  const names = [...new Set([...discovered, ...connection.profiles])];
  return (
    <fieldset className="connection-editor">
      <legend>
        {connection.name || `New ${service?.name || "connection"} connection`}
      </legend>
      <div className="connection-editor-heading">
        <span className="integration-symbol" aria-hidden="true">
          {service?.name.slice(0, 1)}
        </span>
        <p>{service?.description}</p>
        <button
          type="button"
          className="text-button"
          onClick={onRemove}
          aria-label={`Remove connection ${connection.name || index + 1}`}
        >
          Remove
        </button>
      </div>
      <div className="connection-name-grid">
        <label htmlFor={`connection-${index}-name`}>
          Connection name
          <input
            id={`connection-${index}-name`}
            value={connection.name}
            onChange={(e) => onChange({ ...connection, name: e.target.value })}
            placeholder="e.g. Work projects"
            required
            maxLength={80}
          />
        </label>
        <label htmlFor={`connection-${index}-tool`}>
          Service
          <select
            id={`connection-${index}-tool`}
            value={connection.tool}
            onChange={(e) =>
              onChange({
                ...connection,
                tool: e.target.value as Connection["tool"],
                profiles: [],
              })
            }
          >
            {tools.map((tool) => (
              <option
                key={tool.id}
                value={tool.id}
                disabled={tool.id === "agent-notion"}
              >
                {tool.name}
                {tool.id === "agent-notion"
                  ? " (account selection unavailable)"
                  : ""}
              </option>
            ))}
          </select>
        </label>
      </div>
      <div className="profile-selection-heading">
        <strong>Allowed account profiles</strong>
        <button
          type="button"
          className="text-button"
          disabled={loading}
          onClick={() => setRevision(revision + 1)}
        >
          {loading ? "Finding profiles…" : "Refresh profiles"}
        </button>
      </div>
      {error && (
        <p className="error-notice" role="alert">
          {error}
        </p>
      )}
      {discovery?.detail && <p className="field-hint">{discovery.detail}</p>}
      {!loading && !names.length && (
        <p className="field-hint">
          No profiles found. Set up an account with{" "}
          <code>{connection.tool}</code>, then refresh.
        </p>
      )}
      <div className="profile-choices">
        {names.map((name) => (
          <label key={name} className="profile-choice">
            <input
              type="checkbox"
              checked={connection.profiles.includes(name)}
              disabled={
                loading ||
                !discovery?.available ||
                !discovery.selectable ||
                !discovered.includes(name)
              }
              onChange={(e) =>
                onChange({
                  ...connection,
                  profiles: e.target.checked
                    ? [...connection.profiles, name]
                    : connection.profiles.filter((p) => p !== name),
                })
              }
            />
            <span>
              {name}
              {!discovered.includes(name) && !loading && (
                <small>Saved profile; currently unavailable</small>
              )}
            </span>
          </label>
        ))}
      </div>
      {discovery && !discovery.selectable && (
        <p className="field-hint">
          This CLI cannot select a specific account yet. The assistant will keep
          this connection unavailable rather than use a different account.
        </p>
      )}
    </fieldset>
  );
}
