// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { WorkerPreparation } from "./WorkerPreparation";
import { WorkerSettings } from "./WorkerSettings";
import type { Project, WorkerProfile } from "./api";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
const project: Project = {
  id: "private-project-id",
  title: "Garden planner",
  description: "",
  acceptance_criteria: [],
  status: "active",
  directories: ["/work/garden"],
};
function response(body: unknown, ok = true) {
  return { ok, status: ok ? 200 : 400, json: async () => body };
}

describe("assistant-managed worker preparation", () => {
  it("prepares the sole linked folder and shows pending feedback before success", async () => {
    let complete!: (value: unknown) => void;
    const fetch = vi.fn(async (_path: string, options?: RequestInit) =>
      options?.method === "POST"
        ? new Promise((resolve) => {
            complete = resolve;
          })
        : response({ workers: [] }),
    );
    vi.stubGlobal("fetch", fetch);
    render(<WorkerPreparation project={project} demo={false} />);
    fireEvent.click(
      await screen.findByRole("button", { name: "Prepare a worker" }),
    );
    expect(
      screen.getByText("Preparing a worker for Garden planner…"),
    ).toBeTruthy();
    expect(fetch).toHaveBeenLastCalledWith(
      "/api/projects/private-project-id/worker",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ workspace: "/work/garden" }),
      }),
    );
    await act(async () =>
      complete(response({ managed: true, project_id: project.id })),
    );
    expect(screen.getByText("Garden planner")).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: "Prepare a worker" }),
    ).toBeNull();
    expect(screen.getByRole("status").textContent).toContain(
      "what you want to do next",
    );
  });
  it("requires an explicit choice among folders and never asks for runtime or model identifiers", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => response({ workers: [] })),
    );
    const view = render(
      <WorkerPreparation
        project={{ ...project, directories: ["/work/one", "/work/two"] }}
        demo={false}
      />,
    );
    const button = await screen.findByRole("button", {
      name: "Prepare a worker",
    });
    expect((button as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByRole("combobox", { name: "Project folder" }), {
      target: { value: "/work/two" },
    });
    expect((button as HTMLButtonElement).disabled).toBe(false);
    expect(view.container.textContent).not.toMatch(
      /Docker|private-project-id|API key|model identifier/,
    );
  });
  it("asks for a linked folder and respects demo mode", async () => {
    const fetch = vi.fn(async () => response({ workers: [] }));
    vi.stubGlobal("fetch", fetch);
    const view = render(
      <WorkerPreparation
        project={{ ...project, directories: [] }}
        demo={false}
      />,
    );
    expect(await screen.findByText(/Link a project folder first/)).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: "Prepare a worker" }),
    ).toBeNull();
    view.rerender(<WorkerPreparation project={project} demo={true} />);
    expect(
      (
        screen.getByRole("button", {
          name: "Prepare a worker",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
    expect(fetch).toHaveBeenCalledTimes(1);
  });
  it("recognizes a managed binding and ignores workers for another project", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        response({ workers: [{ managed: true, project_id: project.id }] }),
      ),
    );
    const view = render(<WorkerPreparation project={project} demo={false} />);
    expect(await screen.findByText("Your project worker")).toBeTruthy();
    view.rerender(
      <WorkerPreparation
        project={{ ...project, id: "different" }}
        demo={false}
      />,
    );
    expect(
      await screen.findByRole("button", { name: "Prepare a worker" }),
    ).toBeTruthy();
  });
  it("keeps preparation failures visible and retryable", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_path: string, options?: RequestInit) =>
        options?.method === "POST"
          ? response(
              { error: "Unable to prepare the isolated environment" },
              false,
            )
          : response({ workers: [] }),
      ),
    );
    render(<WorkerPreparation project={project} demo={false} />);
    fireEvent.click(
      await screen.findByRole("button", { name: "Prepare a worker" }),
    );
    expect((await screen.findByRole("alert")).textContent).toBe(
      "Unable to prepare the isolated environment",
    );
    expect(
      (
        screen.getByRole("button", {
          name: "Prepare a worker",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false);
  });
});
describe("worker settings", () => {
  it("shows managed workers with project names and preserves them while editing external workers", () => {
    const workers: WorkerProfile[] = [
      {
        id: "managed-hidden-id",
        managed: true,
        name: "Garden helper",
        project_id: project.id,
      },
      {
        id: "external-one",
        name: "External researcher",
        project_id: project.id,
      },
    ];
    const change = vi.fn();
    const view = render(
      <WorkerSettings
        workers={workers}
        projects={[project]}
        onChange={change}
      />,
    );
    expect(
      screen.getByRole("link", { name: "Garden planner" }).getAttribute("href"),
    ).toBe("#/projects/private-project-id");
    expect(view.container.textContent).not.toContain("managed-hidden-id");
    expect(screen.getAllByLabelText("Profile ID")).toHaveLength(1);
    expect(
      screen.getByText("Advanced: external workers").closest("details")?.open,
    ).toBe(false);
    fireEvent.change(screen.getByLabelText("Display name"), {
      target: { value: "Research partner" },
    });
    expect(change).toHaveBeenLastCalledWith([
      workers[0],
      { ...workers[1], name: "Research partner" },
    ]);
    expect(screen.getByLabelText("Project").tagName).toBe("SELECT");
    fireEvent.click(
      screen.getByRole("button", { name: "Add external worker", hidden: true }),
    );
    expect(change.mock.lastCall?.[0][2].id).toMatch(/^external-/);
  });
});
