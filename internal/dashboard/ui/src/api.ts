import type { Connection } from "./ConnectionsSettings";
import type { AvatarSpec } from "./Identity";
export interface Project {
  id: string;
  title: string;
  description: string;
  acceptance_criteria: string[] | string;
  status: string;
  directories?: string[];
  scratch_directory?: string;
  updated_at?: string;
}
export interface WorkItem {
  id: string;
  project_id: string;
  title: string;
  objective: string;
  acceptance_criteria: string;
  status: "ready" | "active" | "review" | "accepted" | "legacy_completed";
  created_at: string;
  updated_at: string;
  review_revision: string;
  acceptance?: {
    revision: string;
    evidence: string[];
    reviewer: string;
    accepted_at: string;
  };
  legacy?: boolean;
}
export interface SteeringMessage {
  id: string;
  work_item_id: string;
  content: string;
  created_at: string;
}
export interface SteeringReceipt {
  message_id: string;
  agent_id: string;
  acknowledged_at: string;
}
export interface Agent {
  work_item_id?: string;
  id: string;
  parent_id?: string;
  name: string;
  role: string;
  status: string;
  project_id: string;
  profile_id?: string;
  task?: string;
  acceptance_criteria?: string;
  broker_updated_at?: string;
  last_progress_at?: string;
  last_update?: string;
  next_check_in?: string;
  summary?: string;
  evidence?: string[];
}
export interface Decision {
  work_item_id?: string;
  id: string;
  agent_id?: string;
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
export interface ChatToolEvent {
  id: string;
  tool: string;
  label: string;
  status: "running" | "completed" | "failed" | "interrupted";
  started_at: string;
  finished_at?: string;
}
export interface ChatTurn {
  id: string;
  message: string;
  status:
    "queued" | "running" | "completed" | "failed" | "interrupted" | "cancelled";
  created_at: string;
  started_at?: string;
  finished_at?: string;
  user_message_id?: string;
  assistant_message_id?: string;
  error?: string;
  loading_phrase?: string;
  events: ChatToolEvent[];
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
  work_items: WorkItem[];
  steering: SteeringMessage[];
  steering_receipts: SteeringReceipt[];
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
    work_items: raw.work_items ?? [],
    steering: raw.steering ?? [],
    steering_receipts: raw.steering_receipts ?? [],
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

export type FileSystemKind = "directory" | "file" | "any";
export interface FileSystemEntry {
  name: string;
  path: string;
  kind: "directory" | "file";
  selectable: boolean;
}
export interface FileSystemPage {
  path: string;
  parent: string | null;
  entries: FileSystemEntry[];
  next_cursor: string | null;
}
