// @vitest-environment jsdom
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ToolActivity } from "./ChatPanel";
import type { ChatToolEvent } from "./api";

const step = (
  id: string,
  status: ChatToolEvent["status"] = "completed",
): ChatToolEvent => ({
  id,
  tool: `tool_${id}`,
  label: `Step ${id}`,
  status,
  started_at: "2026-09-17T09:00:00Z",
});

afterEach(() => {
  cleanup();
});

function summary() {
  return screen.queryByText(/steps completed/);
}
function visibleLabels() {
  const group = screen.getByRole("group", { name: "Assistant tool activity" });
  const open = group.querySelector(":scope > ul");
  return open
    ? Array.from(open.querySelectorAll("li")).map(
        (li) => li.querySelector(".chat-tool-description")?.textContent ?? "",
      )
    : [];
}

describe("tool activity accordion", () => {
  it("keeps the newest finished step out while nothing is newer", () => {
    render(<ToolActivity events={[step("1"), step("2"), step("3")]} live />);
    // Two groupable steps remain once the newest is held out.
    expect(summary()?.textContent).toBe("2 steps completed");
    expect(visibleLabels().join()).toContain("Step 3");
  });

  it("folds the newest step in once a reply arrives", () => {
    render(<ToolActivity events={[step("1"), step("2"), step("3")]} />);
    expect(summary()?.textContent).toBe("3 steps completed");
    expect(visibleLabels()).toEqual([]);
  });

  // Two steps with the newest held out leaves one groupable step, which is
  // shown rather than hidden behind a summary.
  it("does not accordion a single groupable step", () => {
    render(<ToolActivity events={[step("1"), step("2")]} live />);
    expect(summary()).toBeNull();
    expect(visibleLabels().join()).toContain("Step 1");
    expect(visibleLabels().join()).toContain("Step 2");
  });

  it("accordions the same two steps once a reply arrives", () => {
    render(<ToolActivity events={[step("1"), step("2")]} />);
    expect(summary()?.textContent).toBe("2 steps completed");
  });

  it("never hides an unfinished or failed step, wherever it sits", () => {
    render(
      <ToolActivity
        events={[step("1"), step("2", "failed"), step("3"), step("4")]}
      />,
    );
    // The failure stays out of the accordion even though newer steps exist.
    expect(visibleLabels().join()).toContain("Step 2");
    expect(summary()?.textContent).toBe("3 steps completed");
  });

  it("keeps a running step visible beside the accordion", () => {
    render(
      <ToolActivity
        events={[step("1"), step("2"), step("3", "running")]}
        live
      />,
    );
    expect(visibleLabels().join()).toContain("Step 3");
    expect(summary()?.textContent).toBe("2 steps completed");
  });

  it("starts collapsed so the accordion does not cost a click to ignore", () => {
    render(<ToolActivity events={[step("1"), step("2"), step("3")]} />);
    const details = summary()?.parentElement as HTMLDetailsElement;
    expect(details.open).toBe(false);
    expect(within(details).getByText("Step 1")).toBeTruthy();
  });
});
