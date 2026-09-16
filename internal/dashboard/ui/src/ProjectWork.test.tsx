// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ProjectWork } from "./ProjectWork";
import { normalizeState, type Project, type WorkItem } from "./api";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
const project: Project = {
  id: "private-project",
  title: "Garden",
  description: "A lasting project",
  acceptance_criteria: [],
  status: "active",
};
const outcome: WorkItem = {
  id: "private-work",
  project_id: project.id,
  title: "Make planting dates clear",
  objective: "Show the next planting window",
  acceptance_criteria: "Keyboard accessible\nCalendar checks pass",
  status: "review",
  created_at: "2026-09-01T10:00:00Z",
  updated_at: "2026-09-02T10:00:00Z",
  review_revision: "private-review-v1",
};
function fixture() {
  return normalizeState({
    projects: [project],
    work_items: [outcome],
    agents: [
      {
        id: "private-agent",
        work_item_id: outcome.id,
        project_id: project.id,
        name: "Garden builder",
        role: "worker",
        status: "completed",
        summary: "Planting window verified",
        evidence: ["Keyboard navigation verified", "Calendar checks pass"],
      },
    ],
  });
}
function mockFetch(
  handler: (
    path: string,
    body: Record<string, unknown>,
  ) => { status?: number; body?: unknown },
) {
  const fetch = vi.fn(async (path: string, options?: RequestInit) => {
    const result = handler(path, JSON.parse(String(options?.body || "{}")));
    const status = result.status || 200;
    return { ok: status < 400, status, json: async () => result.body || {} };
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}
it("normalizes old snapshots without inventing accepted outcomes or receipts", () => {
  const state = normalizeState({
    projects: [{ ...project, status: "completed" }],
  });
  expect(state.work_items).toEqual([]);
  expect(state.steering).toEqual([]);
  expect(state.steering_receipts).toEqual([]);
  render(<ProjectWork project={project} state={state} refresh={vi.fn()} />);
  expect(screen.getByText(/No outcomes recorded yet/)).toBeTruthy();
  expect(screen.queryByText("Accepted")).toBeNull();
});
it("records the next outcome under the same project and preserves it across refresh failure", async () => {
  const fetch = mockFetch(() => ({}));
  const refresh = vi
    .fn()
    .mockRejectedValueOnce(new Error("Offline"))
    .mockResolvedValue(undefined);
  render(<ProjectWork project={project} state={fixture()} refresh={refresh} />);
  fireEvent.click(screen.getByRole("button", { name: "Define an outcome" }));
  fireEvent.change(screen.getByLabelText("Outcome name"), {
    target: { value: "Export a planting plan" },
  });
  fireEvent.change(screen.getByLabelText("What should change?"), {
    target: { value: "Let me print the plan" },
  });
  fireEvent.change(screen.getByLabelText("How will we know it is done?"), {
    target: { value: "Print preview includes all beds" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Record outcome" }));
  expect(await screen.findByRole("alert")).toHaveProperty(
    "textContent",
    expect.stringContaining("Outcome recorded"),
  );
  expect(fetch).toHaveBeenCalledTimes(1);
  expect(fetch.mock.calls[0][0]).toBe(`/api/projects/${project.id}/work-items`);
  expect(JSON.parse(String(fetch.mock.calls[0][1]?.body))).toEqual({
    title: "Export a planting plan",
    objective: "Let me print the plan",
    acceptance_criteria: "Print preview includes all beds",
  });
  fireEvent.click(
    screen.getByRole("button", { name: "Refresh recorded outcome" }),
  );
  await waitFor(() => expect(refresh).toHaveBeenCalledTimes(2));
  expect(fetch).toHaveBeenCalledTimes(1);
});
it("accepts only the displayed evidence revision and makes stale reviews explicit", async () => {
  const state = fixture();
  const fetch = mockFetch(() => ({
    status: 409,
    body: { error: "Evidence changed since this review." },
  }));
  const refresh = vi.fn().mockResolvedValue(undefined);
  const { rerender } = render(
    <ProjectWork project={project} state={state} refresh={refresh} />,
  );
  const card = screen.getByRole("article", { name: outcome.title });
  expect(within(card).getByText("Keyboard accessible")).toBeTruthy();
  expect(within(card).getByText("Keyboard navigation verified")).toBeTruthy();
  expect(card.textContent).not.toContain(outcome.id);
  expect(card.textContent).not.toContain(outcome.review_revision);
  fireEvent.click(
    within(card).getByRole("button", { name: "Accept this outcome" }),
  );
  await screen.findByText(/This outcome changed/);
  expect(JSON.parse(String(fetch.mock.calls[0][1]?.body))).toEqual({
    review_revision: "private-review-v1",
    evidence: ["Keyboard navigation verified", "Calendar checks pass"],
  });
  expect(
    screen.getByRole("button", { name: "Accept this outcome" }),
  ).toHaveProperty("disabled", true);
  fireEvent.click(screen.getByRole("button", { name: "Refresh evidence" }));
  await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
  rerender(
    <ProjectWork
      project={project}
      state={{
        ...state,
        work_items: [{ ...outcome, review_revision: "private-review-v2" }],
      }}
      refresh={refresh}
    />,
  );
  expect(
    screen.getByRole("button", { name: "Accept this outcome" }),
  ).toHaveProperty("disabled", false);
  expect(fetch).toHaveBeenCalledTimes(1);
});
it("retries uncertain steering with the same request and labels receipt separately from completion", async () => {
  let attempts = 0;
  const fetch = mockFetch(() =>
    ++attempts === 1
      ? { status: 503, body: { error: "Connection interrupted" } }
      : {},
  );
  const state = fixture();
  state.work_items[0] = { ...outcome, status: "active" };
  const refresh = vi.fn().mockResolvedValue(undefined);
  const { rerender } = render(
    <ProjectWork project={project} state={state} refresh={refresh} />,
  );
  const details = screen
    .getByText("Direction and acknowledgments")
    .closest("details")!;
  fireEvent.click(within(details).getByText("Direction and acknowledgments"));
  fireEvent.change(screen.getByLabelText(`Direction for ${outcome.title}`), {
    target: { value: "Keep the keyboard shortcuts" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Send direction" }));
  await screen.findByRole("button", { name: "Retry same direction" });
  const original = JSON.parse(String(fetch.mock.calls[0][1]?.body));
  expect(original.message_id).toMatch(/^[\da-f-]{36}$/);
  fireEvent.click(screen.getByRole("button", { name: "Retry same direction" }));
  await screen.findByText(/Direction recorded. Awaiting worker acknowledgment/);
  expect(JSON.parse(String(fetch.mock.calls[1][1]?.body))).toEqual(original);
  expect(refresh).toHaveBeenCalledTimes(1);
  const sent = {
    id: original.message_id,
    work_item_id: outcome.id,
    content: original.message,
    created_at: "2026-09-03T10:00:00Z",
  };
  rerender(
    <ProjectWork
      project={project}
      state={{ ...state, steering: [sent] }}
      refresh={refresh}
    />,
  );
  expect(
    screen.getByText("Recorded · awaiting worker acknowledgment"),
  ).toBeTruthy();
  rerender(
    <ProjectWork
      project={project}
      state={{
        ...state,
        steering: [sent],
        steering_receipts: [
          {
            message_id: sent.id,
            agent_id: "private-agent",
            acknowledged_at: "2026-09-03T10:01:00Z",
          },
        ],
      }}
      refresh={refresh}
    />,
  );
  expect(screen.getByText(/Acknowledged by Garden builder/)).toBeTruthy();
  expect(
    screen.queryByText("Recorded · awaiting worker acknowledgment"),
  ).toBeNull();
  expect(screen.queryByText("Accepted")).toBeNull();
});
it("labels migrated completion honestly and displays frozen accepted evidence", () => {
  const state = fixture();
  state.work_items = [
    { ...outcome, id: "history", status: "legacy_completed", legacy: true },
    {
      ...outcome,
      id: "accepted",
      title: "Saved outcome",
      status: "accepted",
      review_revision: "frozen",
      acceptance: {
        revision: "frozen",
        evidence: ["Frozen acceptance evidence"],
        reviewer: "owner",
        accepted_at: "2026-09-03T11:00:00Z",
      },
    },
  ];
  render(<ProjectWork project={project} state={state} refresh={vi.fn()} />);
  expect(screen.getByText("Previously completed")).toBeTruthy();
  expect(screen.getByText(/Imported completion history/)).toBeTruthy();
  expect(screen.getByText("Frozen acceptance evidence")).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: "Accept this outcome" }),
  ).toBeNull();
});

it("shows and submits current evidence when a previously accepted outcome reopens", async () => {
  const state = fixture();
  state.work_items = [
    {
      ...outcome,
      review_revision: "new-review",
      acceptance: {
        revision: "old-review",
        evidence: ["Outdated acceptance evidence"],
        reviewer: "owner",
        accepted_at: "2026-09-01T11:00:00Z",
      },
    },
  ];
  const fetch = mockFetch(() => ({}));
  render(
    <ProjectWork
      project={project}
      state={state}
      refresh={vi.fn().mockResolvedValue(undefined)}
    />,
  );
  const currentEvidence = screen.getByRole("heading", {
    name: "Reported evidence",
  }).parentElement!;
  expect(
    within(currentEvidence).getByText("Keyboard navigation verified"),
  ).toBeTruthy();
  expect(
    within(currentEvidence).getByText("Calendar checks pass"),
  ).toBeTruthy();
  expect(
    within(currentEvidence).queryByText("Outdated acceptance evidence"),
  ).toBeNull();
  expect(
    screen.queryByRole("heading", { name: "Accepted evidence" }),
  ).toBeNull();
  expect(
    screen.getByText("Previous acceptance · historical evidence"),
  ).toBeTruthy();
  expect(
    screen.getByText(/does not cover this outcome’s current review/),
  ).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Accept this outcome" }));
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
  expect(JSON.parse(String(fetch.mock.calls[0][1]?.body))).toEqual({
    review_revision: "new-review",
    evidence: ["Keyboard navigation verified", "Calendar checks pass"],
  });
});

it("shows worker interruption and recovery count prominently even alongside other active work", () => {
  const state = fixture();
  state.work_items[0] = {
    ...outcome,
    status: "interrupted",
    status_reason: "The CLI process exited before completing its report",
  };
  state.agents[0] = {
    ...state.agents[0],
    status: "interrupted",
    summary: "The CLI process exited before completing its report",
    recoveries: 2,
  };
  render(<ProjectWork project={project} state={state} refresh={vi.fn()} />);
  const attention = screen.getByLabelText("Worker attention");
  expect(within(attention).getByText("Garden builder")).toBeTruthy();
  expect(within(attention).getByText("Interrupted")).toBeTruthy();
  expect(
    within(attention).getByText(/2 recovery attempts recorded/),
  ).toBeTruthy();
  expect(
    within(attention).getByRole("button", {
      name: "Conversation and controls for Garden builder",
    }),
  ).toBeTruthy();
  // The stopped assignment explains itself in place, and states who acts next
  // rather than handing the owner an undifferentiated summary line.
  expect(
    within(attention).getByLabelText("What stopped Garden builder"),
  ).toBeTruthy();
  expect(within(attention).getByText("Execution was interrupted")).toBeTruthy();
  expect(screen.queryByText("In progress")).toBeNull();
});

it("inspects and controls an assignment in exactly one place", () => {
  const state = fixture();
  state.agents[0] = { ...state.agents[0], status: "interrupted", recoveries: 1 };
  render(<ProjectWork project={project} state={state} refresh={vi.fn()} />);
  // Two copies of one assignment meant two independent conversation polls.
  expect(
    screen.getAllByRole("button", {
      name: "Conversation and controls for Garden builder",
    }),
  ).toHaveLength(1);
  expect(screen.getAllByLabelText("Assignment for Garden builder")).toHaveLength(
    1,
  );
});
it("queues the next outcome explicitly after a named predecessor", async () => {
  const fetch = mockFetch(() => ({}));
  const state = fixture();
  const refresh = vi.fn().mockResolvedValue(undefined);
  render(<ProjectWork project={project} state={state} refresh={refresh} />);
  fireEvent.click(screen.getByRole("button", { name: "Define an outcome" }));
  fireEvent.change(screen.getByLabelText("Outcome name"), {
    target: { value: "Print the plan" },
  });
  fireEvent.change(screen.getByLabelText("What should change?"), {
    target: { value: "A printable plan" },
  });
  fireEvent.change(screen.getByLabelText("How will we know it is done?"), {
    target: { value: "All beds included" },
  });
  fireEvent.change(screen.getByLabelText("When should it start?"), {
    target: { value: outcome.id },
  });
  expect(
    screen.getByText(
      /Queueing authorizes the assistant to commission this work automatically/,
    ),
  ).toBeTruthy();
  fireEvent.click(
    screen.getByRole("button", { name: `Queue after ${outcome.title}` }),
  );
  await waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
  expect(fetch.mock.calls[0][0]).toBe(
    `/api/projects/${project.id}/work-items/queue`,
  );
  expect(JSON.parse(String(fetch.mock.calls[0][1]?.body))).toEqual({
    title: "Print the plan",
    objective: "A printable plan",
    acceptance_criteria: "All beds included",
    after_work_item_id: outcome.id,
  });
});
it("withdraws automatic queue permission while preserving the named draft", async () => {
  const state = fixture();
  state.agents = [];
  state.work_items.push({
    ...outcome,
    id: "next",
    title: "Export next",
    status: "queued",
    commission_requested: true,
    after_work_item_id: outcome.id,
    status_reason: "Waiting for the planting dates outcome to be accepted",
  });
  const fetch = mockFetch(() => ({}));
  const refresh = vi.fn().mockResolvedValue(undefined);
  render(<ProjectWork project={project} state={state} refresh={refresh} />);
  // Queued work reads as a position in a sequence: order, title, what it is
  // waiting for and whether it may start, all without expanding anything.
  const queue = screen.getByLabelText("Queued outcomes");
  const row = within(queue).getByRole("listitem", { name: "Export next" });
  expect(within(row).getByText("1")).toBeTruthy();
  expect(within(row).getByText("Export next")).toBeTruthy();
  expect(within(row).getByText("Queued next")).toBeTruthy();
  expect(
    within(row).getByText(
      `Waits for ${outcome.title} to be accepted · automatic coordination authorized`,
    ),
  ).toBeTruthy();
  // The full brief and queue management stay one disclosure away.
  const detail = within(row).getByText("Brief and queue management");
  expect((detail.parentElement as HTMLDetailsElement).open).toBe(false);
  fireEvent.click(detail);
  fireEvent.click(
    within(row).getByRole("button", { name: "Remove from queue" }),
  );
  await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
  expect(fetch.mock.calls[0][0]).toBe("/api/work-items/next/queue");
  expect(fetch.mock.calls[0][1]?.method).toBe("DELETE");
});

it("flags possibly superseded queued work for review without accepting or starting it", async () => {
  const state = fixture();
  state.agents = [];
  state.work_items.push({
    ...outcome,
    id: "next",
    title: "Lifecycle controls",
    status: "queued",
    commission_requested: true,
    after_work_item_id: outcome.id,
  });
  const fetch = mockFetch(() => ({}));
  const investigate = vi.fn();
  render(
    <ProjectWork
      project={project}
      state={state}
      refresh={vi.fn()}
      onInvestigate={investigate}
    />,
  );
  const row = within(screen.getByLabelText("Queued outcomes")).getByRole(
    "listitem",
    { name: "Lifecycle controls" },
  );
  fireEvent.click(within(row).getByText("Brief and queue management"));
  fireEvent.click(
    within(row).getByRole("button", { name: "Ask for an evidence review" }),
  );
  expect(investigate).toHaveBeenCalledTimes(1);
  const prompt = String(investigate.mock.calls[0][0]);
  expect(prompt).toContain("Lifecycle controls");
  expect(prompt).toContain("Do not accept it or start it");
  // Asking a question is not accepting the outcome or commissioning it.
  expect(fetch).not.toHaveBeenCalled();
});
