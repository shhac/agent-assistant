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
  render(
    <Assignment agent={agent} demo={false} refresh={vi.fn()} />,
  );
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
    expect(
      screen.getByLabelText("What stopped Garden builder"),
    ).toBeTruthy();
  });

  it("shows a settled worker's summary without highlighting it", () => {
    show({ ...base, status: "completed", summary: "All checks recorded." });
    const card = screen.getByLabelText("Assignment for Garden builder");
    expect(card.className).not.toContain("needs-attention");
    expect(screen.getByText("All checks recorded.")).toBeTruthy();
  });
});
