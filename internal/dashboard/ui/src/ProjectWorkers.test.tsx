// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { ProjectWorkers, type ProjectWorker } from "./ProjectWorkers";
import { normalizeState, type Project, type Agent } from "./api";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
const project: Project = {
  id: "hidden-project",
  title: "Garden",
  description: "",
  acceptance_criteria: [],
  status: "active",
};
const worker: ProjectWorker = {
  id: "hidden-profile",
  project_id: project.id,
  name: "Garden builder",
  workspace: "/work/garden",
  managed: true,
  capabilities: ["implement"],
  model: { engine: "claude", model: "opus-5", effort: "high" },
  model_status: "configured",
  settings_editable: true,
  detail: "Configuration applies to future assignments.",
};
const agent: Agent = {
  id: "hidden-assignment",
  project_id: project.id,
  profile_id: worker.id,
  name: "Build planting view",
  role: "worker",
  status: "running",
  task: "Make planting dates clearer",
  acceptance_criteria: "Keyboard accessible\nPasses checks",
  summary: "Implementing the calendar",
  broker_updated_at: "2026-01-02T03:04:00Z",
  last_update: "2026-01-02T03:30:00Z",
  next_check_in: "2026-01-02T03:15:00Z",
  evidence: ["Calendar test passes"],
};
const catalog = {
  available: true,
  detail: "Available from your CLI",
  engine: "claude",
  models: [
    {
      id: "opus-5",
      name: "Opus 5",
      default_effort: "high",
      efforts: [{ id: "high" }, { id: "low" }],
    },
  ],
  current: { model: "opus-5", effort: "high" },
  default: { model: "opus-5", effort: "high" },
};
const response = (body: unknown, ok = true) => ({
  ok,
  status: ok ? 200 : 409,
  json: async () => body,
});

describe("project workers", () => {
  it("separates preparation from commissioned work without exposing identifiers", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => response({ workers: [worker] })),
    );
    const view = render(
      <ProjectWorkers
        project={project}
        state={normalizeState({})}
        refresh={async () => {}}
      />,
    );
    expect(await screen.findByText("Garden builder")).toBeTruthy();
    expect(screen.getByText("Prepared · no active assignments")).toBeTruthy();
    expect(screen.getByText(/No work has been commissioned/)).toBeTruthy();
    expect(screen.getByText("Claude Code · opus-5 · high effort")).toBeTruthy();
    expect(view.container.textContent).not.toContain("hidden-");
  });
  it("matches assignments by profile and decisions by assignment, separates report from supervision", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        response({
          workers: [
            worker,
            { ...worker, id: "other-profile", name: "Reviewer" },
          ],
        }),
      ),
    );
    render(
      <ProjectWorkers
        project={project}
        state={normalizeState({
          agents: [
            agent,
            {
              ...agent,
              id: "other-project-run",
              project_id: "other",
              task: "Unrelated task",
            },
          ],
          decisions: [
            {
              id: "d",
              agent_id: agent.id,
              project_id: project.id,
              title: "Pick the date format",
              context: "Two formats",
              recommendation: "Local format",
              choices: [],
              status: "pending",
            },
            {
              id: "d2",
              agent_id: "other",
              title: "Unrelated decision",
              context: "",
              recommendation: "",
              choices: [],
              status: "pending",
            },
          ],
        })}
        refresh={async () => {}}
      />,
    );
    const profile = await screen.findByRole("article", {
      name: "Garden builder",
    });
    expect(within(profile).getByText("1 active assignment")).toBeTruthy();
    expect(
      within(screen.getByRole("article", { name: "Reviewer" })).getByText(
        "Prepared · no active assignments",
      ),
    ).toBeTruthy();
    const run = screen.getByRole("article", { name: agent.name });
    expect(within(run).getByText("Last worker report")).toBeTruthy();
    expect(within(run).getByText("Local coordination update")).toBeTruthy();
    expect(
      within(run).getByText(/silence alone does not confirm a blocker/),
    ).toBeTruthy();
    expect(within(run).getByText("Pick the date format")).toBeTruthy();
    expect(screen.queryByText("Unrelated decision")).toBeNull();
    expect(screen.queryByText("Unrelated task")).toBeNull();
    fireEvent.click(within(run).getByText("Acceptance criteria and evidence"));
    expect(within(run).getByText("Keyboard accessible")).toBeTruthy();
    expect(within(run).getByText("Calendar test passes")).toBeTruthy();
  });
  it("edits a managed worker using its login's model catalog and preserves drafts during refresh", async () => {
    const fetch = vi.fn(async (path: string, options?: RequestInit) => {
      if (path.startsWith("/api/models")) return response(catalog);
      if (options?.method === "PUT")
        return response({
          ...worker,
          name: "Orchard builder",
          model: { ...worker.model, effort: "low" },
        });
      return response({ workers: [worker] });
    });
    vi.stubGlobal("fetch", fetch);
    const refresh = vi.fn(async () => {});
    render(
      <ProjectWorkers
        project={project}
        state={normalizeState({})}
        refresh={refresh}
      />,
    );
    fireEvent.click(
      await screen.findByRole("button", { name: "Edit worker settings" }),
    );
    await screen.findByRole("option", { name: "Opus 5" });
    expect(
      fetch.mock.calls.some(
        ([path]) =>
          path ===
          "/api/models?profile=worker&worker_profile=hidden-profile&engine=claude",
      ),
    ).toBe(true);
    fireEvent.change(screen.getByLabelText("Worker name"), {
      target: { value: "Orchard builder" },
    });
    fireEvent.change(screen.getByLabelText("Worker effort"), {
      target: { value: "low" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Refresh workers" }));
    await waitFor(() =>
      expect(
        fetch.mock.calls.filter(([path]) => path.endsWith("/workers")).length,
      ).toBe(2),
    );
    expect(
      (screen.getByLabelText("Worker name") as HTMLInputElement).value,
    ).toBe("Orchard builder");
    expect(screen.getByLabelText("Worker model").tagName).toBe("SELECT");
    fireEvent.click(
      screen.getByRole("button", { name: "Save worker settings" }),
    );
    await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
    const request = fetch.mock.calls.find(
      ([, options]) => options?.method === "PUT",
    )!;
    expect(request[0]).toBe(
      "/api/projects/hidden-project/workers/hidden-profile",
    );
    expect(JSON.parse(request[1]!.body as string)).toEqual({
      name: "Orchard builder",
      engine: "claude",
      model: "opus-5",
      effort: "low",
    });
  });
  it("keeps rejected edits visible without changing settings", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string, options?: RequestInit) => {
        if (path.startsWith("/api/models")) return response(catalog);
        if (options?.method === "PUT")
          return response(
            { error: "Worker became active; wait for completion" },
            false,
          );
        return response({ workers: [worker] });
      }),
    );
    render(
      <ProjectWorkers
        project={project}
        state={normalizeState({})}
        refresh={async () => {}}
      />,
    );
    fireEvent.click(
      await screen.findByRole("button", { name: "Edit worker settings" }),
    );
    await screen.findByRole("option", { name: "Opus 5" });
    fireEvent.click(
      screen.getByRole("button", { name: "Save worker settings" }),
    );
    expect((await screen.findByRole("alert")).textContent).toContain(
      "Worker became active",
    );
    expect(
      screen.getByRole("form", { name: "Edit Garden builder" }),
    ).toBeTruthy();
  });
  it("does not allow saving when the model catalog is unavailable", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string) =>
        response(
          path.startsWith("/api/models")
            ? {
                ...catalog,
                available: false,
                detail: "Sign in to this worker's CLI",
                models: [],
              }
            : { workers: [worker] },
        ),
      ),
    );
    render(
      <ProjectWorkers
        project={project}
        state={normalizeState({})}
        refresh={async () => {}}
      />,
    );
    fireEvent.click(
      await screen.findByRole("button", { name: "Edit worker settings" }),
    );
    await screen.findByText("Sign in to this worker's CLI");
    expect(
      (
        screen.getByRole("button", {
          name: "Save worker settings",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    expect(
      (screen.getByLabelText("Worker model") as HTMLSelectElement).value,
    ).toBe("opus-5");
    expect(screen.queryByRole("textbox", { name: /model/i })).toBeNull();
  });
  it("disables editing when the server reports unresolved work", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        response({ workers: [{ ...worker, settings_editable: false }] }),
      ),
    );
    render(
      <ProjectWorkers
        project={project}
        state={normalizeState({
          agents: [
            {
              ...agent,
              broker_updated_at: "0001-01-01T00:00:00Z",
              status: "reconciling",
            },
          ],
        })}
        refresh={async () => {}}
      />,
    );
    expect(
      (
        (await screen.findByRole("button", {
          name: "Edit worker settings",
        })) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    const report = screen.getByText("Last worker report").parentElement!;
    expect(within(report).getByText("Not recorded")).toBeTruthy();
  });
});
