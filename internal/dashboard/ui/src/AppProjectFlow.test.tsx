// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { App } from "./App";
import { normalizeState } from "./api";

beforeEach(() => window.history.replaceState(null, "", "/"));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("project context and next outcome", () => {
  it("links decisions, activity and interrupted operations by name and sends the owner's next outcome", async () => {
    const project = {
      id: "project-internal-42",
      title: "Garden planner",
      description: "A personal planning tool",
      status: "active",
      acceptance_criteria: [],
      directories: [],
    };
    const state = normalizeState({
      assistant: { name: "Iris", personality: "" },
      projects: [project],
      decisions: [
        {
          id: "choice-1",
          project_id: project.id,
          title: "Choose a scope",
          context: "Clarify the scope",
          recommendation: "Start small",
          choices: ["Small"],
          status: "pending",
        },
      ],
      activity: [
        {
          id: "activity-1",
          project_id: project.id,
          summary: "Context recorded",
        },
      ],
      pending_operations: [
        {
          id: "operation-1",
          project_id: project.id,
          summary: "Check the interrupted worker",
        },
      ],
    });
    const calls: { path: string; options?: RequestInit }[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string, options?: RequestInit) => {
        calls.push({ path, options });
        return {
          ok: true,
          status: 200,
          json: async () => (path === "/api/state" ? state : { workers: [] }),
        };
      }),
    );
    render(<App />);
    await screen.findByRole("button", { name: "Iris overview" });
    const decision = screen
      .getByRole("heading", { name: "Choose a scope" })
      .closest("article")!;
    expect(
      within(decision)
        .getByRole("link", { name: "Garden planner" })
        .getAttribute("href"),
    ).toBe("#/projects/project-internal-42");
    const activity = screen.getByText("Context recorded").closest("li")!;
    fireEvent.click(
      within(activity).getByRole("link", { name: "Garden planner" }),
    );
    const next = await screen.findByRole("textbox", {
      name: "What would you like to do next?",
    });
    expect(window.location.hash).toBe("#/projects/project-internal-42");
    fireEvent.change(next, {
      target: {
        value: "Make the seasonal planting view easier to understand.",
      },
    });
    fireEvent.click(
      screen.getByRole("button", { name: "Work on this with me" }),
    );
    await waitFor(() =>
      expect(calls.some((call) => call.path.endsWith("/coordinate"))).toBe(
        true,
      ),
    );
    const request = calls.find((call) => call.path.endsWith("/coordinate"))!;
    expect(request.options?.method).toBe("POST");
    expect(JSON.parse(request.options?.body as string)).toEqual({
      next: "Make the seasonal planting view easier to understand.",
    });
    fireEvent.click(screen.getByRole("button", { name: /^Decisions/ }));
    expect(window.location.hash).toBe("");
    const operations = await screen.findByRole("region", {
      name: "Interrupted operations",
    });
    fireEvent.click(
      within(operations).getByRole("link", { name: "Garden planner" }),
    );
    await screen.findByRole("textbox", {
      name: "What would you like to do next?",
    });
    fireEvent.click(screen.getByRole("button", { name: /All projects/ }));
    expect(window.location.hash).toBe("");
    expect(
      screen.queryByRole("textbox", {
        name: "What would you like to do next?",
      }),
    ).toBeNull();
  });
});
