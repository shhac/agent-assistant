// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { AttentionSummary, executionLabel } from "./AttentionSummary";
import type { Project, ProjectAttention } from "./api";

const projects: Project[] = [
  {
    id: "p1",
    title: "Garden planner",
    description: "",
    acceptance_criteria: [],
    status: "active",
  },
];

const blocked: ProjectAttention = {
  project_id: "p1",
  work_item_id: "w1",
  agent_id: "a1",
  agent_name: "Suggestions worker",
  execution: "blocked",
  reason: "The attempt stopped without a classified provider error.",
  next_action: "owner",
  recovery: "held",
  last_progress_at: new Date(Date.now() - 45 * 60_000).toISOString(),
  open_decisions: 0,
  pending_operations: 0,
};

function show(attention: ProjectAttention[], decisionCount = 0) {
  const onOpen = vi.fn();
  render(
    <AttentionSummary
      attention={attention}
      projects={projects}
      decisionCount={decisionCount}
      onOpen={onOpen}
      onReview={vi.fn()}
      hasProjects
    />,
  );
  return onOpen;
}

afterEach(() => {
  cleanup();
});

describe("attention summary", () => {
  it("never reports a quiet decision queue as healthy work", () => {
    show([blocked]);
    expect(screen.queryByText("No decisions waiting on you")).toBeNull();
    expect(screen.getByText("1 outcome is waiting on you")).toBeTruthy();
    expect(
      screen.getByRole("region", { name: "Work needing attention" }),
    ).toBeTruthy();
  });

  it("keeps operational problems separate from owner decisions", () => {
    show([blocked], 2);
    expect(screen.getByText("2 decisions and 1 outcome need you")).toBeTruthy();
    expect(
      screen.getByText("Separate from decisions waiting on you"),
    ).toBeTruthy();
  });

  it("states the blocker, last progress, owner and recovery status", () => {
    show([blocked]);
    const row = screen.getByRole("button", { name: /Garden planner/ });
    expect(row.textContent).toContain("Suggestions worker");
    expect(row.textContent).toContain(
      "The attempt stopped without a classified provider error.",
    );
    expect(row.textContent).toContain("Last progress 45 min ago");
    expect(row.textContent).toContain("You act next");
    expect(row.textContent).toContain("Held until you decide");
    expect(row.textContent).toContain("Blocked");
  });

  it("opens the affected work in one click", () => {
    const onOpen = show([blocked]);
    fireEvent.click(screen.getByRole("button", { name: /Garden planner/ }));
    expect(onOpen).toHaveBeenCalledWith("p1", "w1");
  });

  it("attributes a scheduled retry to the worker, not the owner", () => {
    show([
      {
        ...blocked,
        execution: "retry_wait",
        next_action: "worker",
        recovery: "scheduled",
        reason: "Model provider temporarily unavailable",
      },
    ]);
    const row = screen.getByRole("button", { name: /Garden planner/ });
    expect(row.textContent).toContain("The worker acts next");
    expect(row.textContent).toContain("Retry scheduled");
    expect(screen.getByText("1 outcome is not moving")).toBeTruthy();
  });

  it("says what is actually known when nothing is held up", () => {
    show([
      {
        ...blocked,
        execution: "running",
        next_action: "worker",
        recovery: "",
        reason: "Work is running",
      },
    ]);
    expect(screen.getByText("No decisions waiting on you")).toBeTruthy();
    expect(
      screen.getByText(
        "No work is reported blocked, and nothing needs your judgment.",
      ),
    ).toBeTruthy();
    expect(
      screen.queryByRole("region", { name: "Work needing attention" }),
    ).toBeNull();
  });

  it("surfaces work awaiting review as the owner's turn", () => {
    show([
      {
        ...blocked,
        execution: "review",
        next_action: "owner",
        recovery: "",
        reason: "Execution finished; acceptance review is required",
      },
    ]);
    expect(screen.getByText("Ready for your review")).toBeTruthy();
  });

  it("never prints a raw state identifier", () => {
    show([
      {
        ...blocked,
        execution: "reconciling",
        next_action: "assistant",
        recovery: "checking",
        reason: "Checking worker state before any retry",
      },
    ]);
    expect(document.body.textContent).not.toContain("reconciling");
    expect(screen.getByText("Checking worker state")).toBeTruthy();
    expect(executionLabel("pause_requested")).toBe("Pausing");
    expect(executionLabel("retry_wait")).not.toContain("_");
  });

  it("does not demand the owner for a control they already requested", () => {
    show([{ ...blocked, execution: "pause_requested", next_action: "owner" }]);
    expect(
      screen.queryByRole("region", { name: "Work needing attention" }),
    ).toBeNull();
  });
});
