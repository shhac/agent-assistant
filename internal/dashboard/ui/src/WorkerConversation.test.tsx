// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { WorkerConversation } from "./WorkerConversation";
import type { Agent } from "./api";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
const agent: Agent = {
  id: "private-worker",
  name: "Garden builder",
  role: "worker",
  status: "running",
  project_id: "private-project",
  work_item_id: "private-outcome",
};
const controls = { pause: true, resume: false, stop: true, message: true };
const page = {
  messages: [
    {
      sequence: 1,
      id: "m1",
      agent_id: agent.id,
      kind: "instruction",
      direction: "daemon_to_worker",
      content: "Keep **keyboard access**.",
      created_at: "2026-09-04T10:00:00Z",
    },
    {
      sequence: 2,
      id: "m2",
      agent_id: agent.id,
      kind: "report",
      direction: "worker_to_daemon",
      content: "Keyboard checks pass.",
      created_at: "2026-09-04T10:01:00Z",
    },
  ],
  next_cursor: 2,
  has_more: false,
  controls,
};
function mock(
  handler: (
    path: string,
    body: Record<string, unknown>,
  ) => { status?: number; body?: unknown },
) {
  const fetch = vi.fn(async (path: string, options?: RequestInit) => {
    const result = handler(path, JSON.parse(String(options?.body || "{}")));
    return {
      ok: !result.status || result.status < 400,
      status: result.status || 200,
      json: async () => result.body || {},
    };
  });
  vi.stubGlobal("fetch", fetch);
  return fetch;
}
function open(refresh = vi.fn().mockResolvedValue(undefined)) {
  render(
    <WorkerConversation
      agent={agent}
      demo={false}
      outcomeTitle="Planting dates"
      refresh={refresh}
    />,
  );
  fireEvent.click(
    screen.getByRole("button", {
      name: "Conversation and controls for Garden builder",
    }),
  );
}
it("shows chronological shared conversation and uses incremental refresh without duplicate entries", async () => {
  const fetch = mock((path) => ({
    body: path.includes("after=2")
      ? {
          ...page,
          messages: [
            {
              ...page.messages[1],
              sequence: 3,
              id: "m3",
              content: "Final evidence prepared.",
            },
          ],
          next_cursor: 3,
        }
      : page,
  }));
  open();
  await screen.findByText("Keyboard checks pass.");
  const entries = screen.getAllByRole("listitem");
  expect(entries[0].textContent).toContain("Assistant → worker");
  expect(entries[1].textContent).toContain("Worker → assistant");
  expect(screen.getByText("keyboard access").tagName).toBe("STRONG");
  expect(screen.getByRole("button", { name: "Resume worker" })).toHaveProperty(
    "disabled",
    true,
  );
  fireEvent.click(screen.getByRole("button", { name: "Refresh conversation" }));
  await screen.findByText("Final evidence prepared.");
  expect(fetch.mock.calls[1][0]).toContain("after=2");
  expect(screen.getAllByRole("listitem")).toHaveLength(3);
});
it("requests graceful pause without claiming it is already paused and disables in-flight controls", async () => {
  const fetch = mock((path) =>
    path.endsWith("/control")
      ? { body: { ...agent, status: "pause_requested" } }
      : { body: page },
  );
  open();
  await screen.findByText("Keyboard checks pass.");
  fireEvent.click(screen.getByRole("button", { name: "Pause worker" }));
  await screen.findByText("Pause requested · waiting for current operation");
  expect(screen.getByRole("button", { name: "Stop worker" })).toHaveProperty(
    "disabled",
    false,
  );
  const control = fetch.mock.calls.find(([path]) => path.endsWith("/control"))!;
  expect(JSON.parse(String(control[1]?.body))).toMatchObject({
    action: "pause",
    operation_id: expect.stringMatching(/^[\da-f-]{36}$/),
  });
  expect(
    screen.getByText(
      /The worker finishes the current operation and saves progress; wait for confirmation/,
    ),
  ).toBeTruthy();
});
it("retries an unconfirmed stop with the same operation ID", async () => {
  let stops = 0;
  const fetch = mock((path) =>
    path.endsWith("/control")
      ? ++stops === 1
        ? { status: 503, body: { error: "Connection lost" } }
        : { body: { ...agent, status: "stop_requested" } }
      : { body: page },
  );
  open();
  await screen.findByText("Keyboard checks pass.");
  fireEvent.click(screen.getByRole("button", { name: "Stop worker" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "Retry stop request" }),
  );
  await screen.findByText("Stop requested · waiting for cleanup");
  const calls = fetch.mock.calls.filter(([path]) => path.endsWith("/control"));
  expect(calls).toHaveLength(2);
  expect(calls[0][1]?.body).toBe(calls[1][1]?.body);
});
it("honors unsupported controls and refreshes capability state after a stale action", async () => {
  let rejected = false;
  const fetch = mock((path) => {
    if (path.endsWith("/control")) {
      rejected = true;
      return {
        status: 409,
        body: { error: "Worker state changed; inspect current status." },
      };
    }
    return {
      body: {
        ...page,
        controls: rejected
          ? {
              pause: false,
              resume: false,
              stop: false,
              message: false,
              reason: "This external runtime does not support controls.",
            }
          : controls,
      },
    };
  });
  open();
  await screen.findByText("Keyboard checks pass.");
  fireEvent.click(screen.getByRole("button", { name: "Pause worker" }));
  await screen.findByText("This external runtime does not support controls.");
  expect(screen.getByRole("alert").textContent).toContain(
    "Worker state changed",
  );
  for (const name of ["Pause worker", "Resume worker", "Stop worker"])
    expect(screen.getByRole("button", { name })).toHaveProperty(
      "disabled",
      true,
    );
  expect(
    fetch.mock.calls.filter(([path]) => path.endsWith("/control")),
  ).toHaveLength(1);
});
it("records owner direction with an immutable retry and describes its outcome scope", async () => {
  let sends = 0;
  const fetch = mock((path) =>
    path.endsWith("/messages")
      ? ++sends === 1
        ? { status: 503, body: { error: "No response" } }
        : {}
      : { body: page },
  );
  open();
  await screen.findByText("Keyboard checks pass.");
  fireEvent.change(screen.getByLabelText("Direction for Planting dates"), {
    target: { value: "Keep dark mode" },
  });
  fireEvent.click(screen.getByRole("button", { name: "Send direction" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "Retry same direction" }),
  );
  await screen.findByText(/Direction recorded for this outcome/);
  const calls = fetch.mock.calls.filter(([path]) => path.endsWith("/messages"));
  expect(calls[0][1]?.body).toBe(calls[1][1]?.body);
  expect(screen.getByText(/shared with its responsible agents/)).toBeTruthy();
});
it("loads earlier retained messages without moving the live polling cursor backwards", async () => {
  const fetch = mock((path) => ({
    body: path.includes("before=1")
      ? {
          ...page,
          messages: [
            {
              ...page.messages[0],
              sequence: 0,
              id: "old",
              content: "Initial outcome agreed.",
            },
          ],
          oldest_sequence: 0,
          next_cursor: 0,
          truncated: false,
        }
      : { ...page, oldest_sequence: 1, truncated: true },
  }));
  open();
  fireEvent.click(
    await screen.findByRole("button", { name: "Load earlier messages" }),
  );
  await screen.findByText("Initial outcome agreed.");
  expect(screen.getAllByRole("listitem")[0].textContent).toContain(
    "Initial outcome agreed.",
  );
  expect(
    screen.queryByRole("button", { name: "Load earlier messages" }),
  ).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Refresh conversation" }));
  expect(fetch.mock.calls.at(-1)?.[0]).toContain("after=2");
});
it("distinguishes trimmed history from pagination and pre-recording gaps", async () => {
  mock(() => ({ body: { ...page, history_limited: true, truncated: false } }));
  open();
  await screen.findByText("Keyboard checks pass.");
  expect(
    screen.getByText(/Older conversation entries are no longer retained/),
  ).toBeTruthy();
  expect(
    screen.getByText(/earlier work may predate conversation recording/),
  ).toBeTruthy();
  expect(
    screen.queryByRole("button", { name: "Load earlier messages" }),
  ).toBeNull();
  expect(
    screen.getByRole("list", { name: "Recorded messages for Garden builder" })
      .tabIndex,
  ).toBe(0);
});
it("keeps worker controls available if only older-history retrieval fails", async () => {
  mock((path) =>
    path.includes("before=")
      ? { status: 503, body: { error: "Archive unavailable" } }
      : { body: { ...page, oldest_sequence: 1, truncated: true } },
  );
  open();
  fireEvent.click(
    await screen.findByRole("button", { name: "Load earlier messages" }),
  );
  await screen.findByText(
    /Earlier messages could not be loaded: Archive unavailable/,
  );
  expect(screen.getByRole("button", { name: "Pause worker" })).toHaveProperty(
    "disabled",
    false,
  );
  expect(screen.queryByText(/Controls are unavailable until/)).toBeNull();
});
it("shows provider cooldown separately from login failure and preserves worker controls", async () => {
  mock(() => ({
    body: { ...page, controls: { ...controls, message: false } },
  }));
  render(
    <WorkerConversation
      agent={{
        ...agent,
        status: "retry_wait",
        retry_at: "2026-09-16T15:00:00Z",
        provider_failures: 2,
        context_compactions: 1,
      }}
      demo={false}
      refresh={vi.fn()}
    />,
  );
  fireEvent.click(
    screen.getByRole("button", {
      name: "Conversation and controls for Garden builder",
    }),
  );
  await screen.findByText("Keyboard checks pass.");
  expect(screen.getByText("Waiting for model provider")).toBeTruthy();
  expect(screen.getByText(/Next provider retry after/)).toBeTruthy();
  expect(
    screen.getByText(
      /1 context checkpoint saved. The full worker transcript is retained/,
    ),
  ).toBeTruthy();
  expect(screen.getByRole("button", { name: "Pause worker" })).toHaveProperty(
    "disabled",
    false,
  );
  expect(screen.getByRole("button", { name: "Resume worker" })).toHaveProperty(
    "disabled",
    true,
  );
});
