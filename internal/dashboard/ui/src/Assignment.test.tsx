// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Assignment } from "./Assignment";
import type { Agent } from "./api";

const base: Agent = {
  id: "agent-1",
  name: "Garden builder",
  role: "worker",
  status: "paused",
  project_id: "project-1",
  summary: "Paused after the owner asked for a pause.",
  last_update: "2026-09-16T16:00:00Z",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function show(agent: Agent) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => ({ ok: true, status: 200, json: async () => ({}) })),
  );
  render(<Assignment agent={agent} demo={false} refresh={vi.fn()} />);
}

describe("assignment", () => {
  // The highlight rule and the failure card's own rule used to be separate
  // lists. When they disagreed the card rendered nothing and the summary was
  // suppressed, leaving an amber card with no account of itself.
  it("always accounts for a worker it highlights", () => {
    for (const status of ["paused", "pause_requested", "stop_requested"]) {
      cleanup();
      show({ ...base, status });
      const card = screen.getByLabelText("Assignment for Garden builder");
      expect(card.className).toContain("needs-attention");
      expect(
        screen.getByText("Paused after the owner asked for a pause."),
      ).toBeTruthy();
    }
  });

  it("uses the failure card when it has an explanation to give", () => {
    show({
      ...base,
      status: "blocked",
      provider_failure_kind: "unknown",
      model_failure_evidence: "untyped_error",
    });
    expect(screen.getByLabelText("What stopped Garden builder")).toBeTruthy();
  });

  it("shows a settled worker's summary without highlighting it", () => {
    show({ ...base, status: "completed", summary: "All checks recorded." });
    const card = screen.getByLabelText("Assignment for Garden builder");
    expect(card.className).not.toContain("needs-attention");
    expect(screen.getByText("All checks recorded.")).toBeTruthy();
  });
});

// A full account allowance is ordinary waiting with a known end. Painting it
// amber sends the owner looking for a problem that does not exist.
describe("assignment resource waits", () => {
  it("reports a quota wait as waiting, not as attention", () => {
    show({
      ...base,
      status: "usage_wait",
      summary: "New worker work paused: codex five_hour is 92.0% consumed",
      resource_hold_kind: "subscription_quota",
      resource_hold_resets_at: "2026-09-17T20:00:00Z",
      usage_input_tokens: 120000,
      usage_output_tokens: 8000,
    });
    const card = screen.getByLabelText("Assignment for Garden builder");
    expect(card.className).not.toContain("needs-attention");
    expect(screen.getAllByText(/Waiting for worker resources/).length).toBe(2);
    expect(screen.getByText(/continues by itself/)).toBeTruthy();
    // A published reset is what the provider reports, not a promise about when
    // this assignment resumes.
    expect(
      screen.getByText(/reports its allowance resets around/),
    ).toBeTruthy();
    expect(screen.getByText(/128,000 tokens/)).toBeTruthy();
  });

  it("asks for the owner when the budget is theirs to raise", () => {
    show({
      ...base,
      status: "usage_wait",
      summary: "Worker token budget reached: 100000 of 100000 tokens used",
      resource_hold_kind: "token_budget",
      resource_hold_owner_action: true,
      usage_input_tokens: 90000,
      usage_output_tokens: 10000,
      token_budget: 100000,
    });
    const card = screen.getByLabelText("Assignment for Garden builder");
    expect(card.className).toContain("needs-attention");
    expect(screen.getByText(/Waiting for your decision/)).toBeTruthy();
    expect(screen.getByText(/100,000 of 100,000 tokens/)).toBeTruthy();
  });

  // Usage a provider never reported is not zero, and the owner has to be able
  // to see that the total they are reading is incomplete.
  it("says when some calls reported no usage", () => {
    show({
      ...base,
      status: "running",
      usage_input_tokens: 5000,
      usage_output_tokens: 500,
      usage_unknown_calls: 2,
    });
    expect(
      screen.getByText(/5,500 tokens · 2 calls reported no usage/),
    ).toBeTruthy();
  });
});
