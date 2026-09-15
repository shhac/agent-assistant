import type { Connection } from "./ConnectionsSettings";
import type { AvatarSpec } from "./Identity";
export interface Project {
  id: string;
  title: string;
  description: string;
  acceptance_criteria: string[] | string;
  status: string;
  updated_at?: string;
}
export interface Agent {
  id: string;
  name: string;
  role: string;
  status: string;
  project_id: string;
  last_update?: string;
  next_check_in?: string;
  summary?: string;
  evidence?: string[];
}
export interface Decision {
  id: string;
  project_id?: string;
  title: string;
  context: string;
  recommendation: string;
  choices: string[];
  status: string;
  created_at?: string;
}
export interface Message {
  id: string;
  role: string;
  content: string;
  created_at?: string;
}
export interface Memory {
  id: string;
  content: string;
  updated_at?: string;
}
export interface Activity {
  id: string;
  project_id?: string;
  kind?: string;
  summary: string;
  created_at?: string;
}
export interface Integration {
  id: string;
  name: string;
  status: string;
  detail?: string;
}
export interface PendingOperation {
  id: string;
  summary: string;
  project_id?: string;
}
export interface WorkerProfile {
  id: string;
  name?: string;
  endpoint?: string;
  api_key_env?: string;
  capabilities?: string[];
  project_id?: string;
  [key: string]: unknown;
}
export interface State {
  pending_operations: PendingOperation[];
  assistant: {
    name: string;
    personality: string;
    theme?: string;
    avatar?: AvatarSpec;
  };
  projects: Project[];
  agents: Agent[];
  decisions: Decision[];
  messages: Message[];
  memories: Memory[];
  activity: Activity[];
  integrations: Integration[];
  paused: boolean;
  demo: boolean;
}
export type Config = Record<string, unknown> & {
  assistant?: {
    name?: string;
    personality?: string;
    theme?: string;
    avatar?: AvatarSpec;
    [key: string]: unknown;
  };
  workers?: WorkerProfile[];
  connections?: Connection[];
};
export class APIError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}
export async function api<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const response = await fetch(path, {
    ...options,
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      "X-Requested-With": "agent-assistant",
      ...options.headers,
    },
  });
  const body = await response.json().catch(() => null);
  if (!response.ok) {
    const detail = body?.error;
    const message = typeof detail === "string" ? detail : detail?.message;
    throw new APIError(
      [message || `Request failed (${response.status})`, body?.hint]
        .filter(Boolean)
        .join(" "),
      response.status,
    );
  }
  return body as T;
}
export function normalizeState(raw: Partial<State>): State {
  return {
    assistant: raw.assistant ?? { name: "", personality: "" },
    pending_operations: raw.pending_operations ?? [],
    projects: raw.projects ?? [],
    agents: raw.agents ?? [],
    decisions: raw.decisions ?? [],
    messages: raw.messages ?? [],
    memories: raw.memories ?? [],
    activity: raw.activity ?? [],
    integrations: raw.integrations ?? [],
    paused: raw.paused ?? false,
    demo: raw.demo ?? false,
  };
}
export function pendingDecisions(decisions: Decision[]) {
  return decisions.filter(
    (d) => !["resolved", "answered", "cancelled", "closed"].includes(d.status),
  );
}
export function errorText(error: unknown) {
  return error instanceof Error
    ? error.message
    : "Something went wrong. Please try again.";
}
export function criteriaLines(
  criteria: Project["acceptance_criteria"],
): string[] {
  return (Array.isArray(criteria) ? criteria : (criteria || "").split("\n"))
    .map((s) => s.trim())
    .filter(Boolean);
}

let pairingRequest: Promise<void> | null = null;
export function bootstrapSession(): Promise<void> {
  if (pairingRequest) return pairingRequest;
  const token = new URLSearchParams(window.location.hash.slice(1)).get("token");
  if (!token) return Promise.resolve();
  // Remove the one-use credential from the address before any network work.
  window.history.replaceState(
    null,
    "",
    window.location.pathname + window.location.search,
  );
  pairingRequest = api<void>("/api/session", {
    method: "POST",
    body: JSON.stringify({ token }),
  });
  return pairingRequest;
}
