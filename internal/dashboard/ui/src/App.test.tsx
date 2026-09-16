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
import { bootstrapSession, normalizeState, type State } from "./api";

const initial = (): State =>
  normalizeState({
    assistant: { name: "Iris", personality: "Concise and thoughtful." },
  });
let state: State;
let calls: { path: string; options?: RequestInit }[];
let respond: (
  path: string,
  options?: RequestInit,
) => { status?: number; body?: unknown };
beforeEach(() => {
  state = initial();
  calls = [];
  window.history.replaceState(null, "", "/");
  respond = (path) => ({ body: path === "/api/state" ? state : {} });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string, options?: RequestInit) => {
      calls.push({ path: input, options });
      const result = respond(input, options);
      const status = result.status || 200;
      return {
        ok: status >= 200 && status < 300,
        status,
        json: async () => result.body ?? {},
      };
    }),
  );
  HTMLDialogElement.prototype.showModal = function () {
    this.setAttribute("open", "");
  };
  HTMLDialogElement.prototype.close = function () {
    this.removeAttribute("open");
  };
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("owner dashboard flows", () => {
  it("uses the configured name and an honest outcome-focused empty workspace", async () => {
    render(<App />);
    expect(
      await screen.findByRole("button", { name: "Iris overview" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Start with the outcome" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Overview" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Today" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Tomorrow" })).toBeNull();
    expect(screen.getByLabelText("Message Iris")).toBeTruthy();
    expect(screen.queryByText("Preview mode", { exact: false })).toBeNull();
  });
  it("expands the conversation without losing its draft and restores navigation", async () => {
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      callback(0);
      return 0;
    });
    render(<App />);
    const field = await screen.findByLabelText("Message Iris");
    fireEvent.change(field, { target: { value: "Keep this context." } });
    fireEvent.click(
      screen.getByRole("button", { name: "Expand conversation" }),
    );
    expect(
      screen
        .getByRole("button", { name: "Return to workspace" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(document.querySelector(".workspace")!.hasAttribute("inert")).toBe(
      true,
    );
    expect(document.activeElement).toBe(field);
    fireEvent.click(
      screen.getByRole("button", { name: "Return to workspace" }),
    );
    expect(document.querySelector(".workspace")!.hasAttribute("inert")).toBe(
      false,
    );
    expect(field).toHaveProperty("value", "Keep this context.");
  });
  it("keeps a failed decision visible and exposes the actionable error", async () => {
    state.decisions = [
      {
        id: "decision-1",
        title: "Which review scope?",
        context: "The broader review takes longer.",
        recommendation: "Review the changed behavior first.",
        choices: ["Focused review", "Full review"],
        status: "pending",
      },
    ];
    respond = (path) =>
      path.includes("/resolve")
        ? {
            status: 409,
            body: {
              error: "This decision changed.",
              hint: "Refresh its context before choosing.",
            },
          }
        : { body: state };
    render(<App />);
    const choice = await screen.findByRole("button", {
      name: "Focused review",
    });
    fireEvent.click(choice);
    expect(await screen.findByRole("alert")).toHaveProperty(
      "textContent",
      "This decision changed. Refresh its context before choosing.",
    );
    expect(
      screen.getByRole("heading", { name: "Which review scope?" }),
    ).toBeTruthy();
    await waitFor(() => expect(choice).toHaveProperty("disabled", false));
    const submitted = calls.filter((c) => c.path.endsWith("/resolve"));
    expect(submitted).toHaveLength(1);
    expect(JSON.parse(submitted[0].options!.body as string)).toEqual({
      choice: "Focused review",
    });
    expect(submitted[0].options!.headers).toHaveProperty(
      "X-Requested-With",
      "agent-assistant",
    );
    expect(submitted[0].options!.credentials).toBe("same-origin");
  });
  it("preserves the submitted message when delivery is not confirmed", async () => {
    respond = (path) =>
      path === "/api/chat/messages"
        ? {
            status: 503,
            body: { error: "Configure an assistant model in Settings." },
          }
        : { body: state };
    render(<App />);
    const field = await screen.findByLabelText("Message Iris");
    fireEvent.change(field, {
      target: { value: "Please coordinate this project." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Send message" }));
    expect(await screen.findByRole("alert")).toHaveProperty(
      "textContent",
      "Configure an assistant model in Settings.",
    );
    expect(screen.getByText("Please coordinate this project.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Retry delivery" })).toBeTruthy();
    expect(field).toHaveProperty("value", "");
    expect(screen.queryByText("Iris is working through it…")).toBeNull();
  });
  it("records acceptance criteria as text without silently starting project work", async () => {
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: "Add project" }));
    fireEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: "New project",
      }),
    );
    fireEvent.change(screen.getByLabelText("Project name"), {
      target: { value: "A useful dashboard" },
    });
    fireEvent.change(screen.getByLabelText("Desired outcome"), {
      target: { value: "Make pending decisions easy to find." },
    });
    fireEvent.change(screen.getByLabelText("What does done look like?"), {
      target: {
        value:
          "Decision context is visible\nError responses preserve the draft",
      },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create project" }));
    await waitFor(() =>
      expect(calls.some((c) => c.path === "/api/projects")).toBe(true),
    );
    const body = JSON.parse(
      calls.find((c) => c.path === "/api/projects")!.options!.body as string,
    );
    expect(body.acceptance_criteria).toBe(
      "Decision context is visible\nError responses preserve the draft",
    );
    expect(calls.some((c) => c.path.endsWith("/coordinate"))).toBe(false);
  });
  it("preserves unrelated configuration when changing the assistant name", async () => {
    const config = {
      assistant: state.assistant,
      dashboard: { addr: "127.0.0.1:8340" },
      workers: [
        {
          id: "runner-a",
          name: "Local worker",
          endpoint: "http://127.0.0.1:8350",
          api_key_env: "WORKER_KEY",
          capabilities: ["implement"],
        },
      ],
      model: { model: "configured-model", api_key_env: "TEST_MODEL_KEY" },
    };
    respond = (path) => ({ body: path === "/api/config" ? config : state });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: /^Settings$/ }));
    const name = await screen.findByLabelText("Name");
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Save preferences" }),
      ).toHaveProperty("disabled", false),
    );
    fireEvent.change(name, { target: { value: "Fern" } });
    fireEvent.click(screen.getByRole("button", { name: "Save preferences" }));
    await waitFor(() =>
      expect(
        calls.some(
          (c) => c.path === "/api/config" && c.options?.method === "PUT",
        ),
      ).toBe(true),
    );
    const saved = JSON.parse(
      calls.find(
        (c) => c.path === "/api/config" && c.options?.method === "PUT",
      )!.options!.body as string,
    );
    expect(saved).toEqual({
      ...config,
      assistant: { ...config.assistant, name: "Fern" },
    });
  });
  it("binds a worker to its approved project without discarding other configuration", async () => {
    const config = {
      assistant: state.assistant,
      model: { model: "kept-model" },
      workers: [
        {
          id: "local",
          name: "Local broker",
          endpoint: "http://127.0.0.1:8350",
          api_key_env: "WORKER_TOKEN",
          capabilities: ["implement"],
          project_id: "prior-project",
          future_option: "preserved",
        },
      ],
    };
    state.projects = [
      {
        id: "approved-project",
        title: "Approved project",
        description: "",
        acceptance_criteria: "",
        status: "ready",
      },
    ];
    respond = (path) => ({ body: path === "/api/config" ? config : state });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: /^Settings$/ }));
    const project = await screen.findByLabelText(/^Project$/);
    fireEvent.change(project, { target: { value: "approved-project" } });
    fireEvent.click(screen.getByLabelText("review"));
    fireEvent.click(screen.getByRole("button", { name: "Save preferences" }));
    await waitFor(() =>
      expect(
        calls.some(
          (c) => c.path === "/api/config" && c.options?.method === "PUT",
        ),
      ).toBe(true),
    );
    const saved = JSON.parse(
      calls.find(
        (c) => c.path === "/api/config" && c.options?.method === "PUT",
      )!.options!.body as string,
    );
    expect(saved.model).toEqual(config.model);
    expect(saved.workers).toEqual([
      {
        ...config.workers[0],
        project_id: "approved-project",
        capabilities: ["implement", "review"],
      },
    ]);
    expect(saved.workers[0].capabilities).not.toContain("coordinate");
    expect(saved.workers[0].api_key_env).toBe("WORKER_TOKEN");
  });
  it("saves independent engine and effort profiles without changing provider credentials", async () => {
    const model = {
      engine: "codex",
      model: "gpt-6-astra",
      effort: "high",
      codex_bin: "codex",
      codex_home: "/fixture/assistant-login",
      base_url: "https://api.example.test/v1",
      api_key_env: "PA_KEY",
      max_tokens: 4096,
    };
    const config = {
      assistant: state.assistant,
      model,
      worker_model: { ...model, api_key_env: "WORKER_KEY" },
    };
    respond = (path) => ({ body: path === "/api/config" ? config : state });
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: /^Settings$/ }));
    expect(await screen.findByLabelText("Assistant engine")).toHaveProperty(
      "value",
      "codex",
    );
    expect(
      screen.getByLabelText(/^Assistant custom model identifier/),
    ).toHaveProperty("value", "gpt-6-astra");
    expect(screen.getByLabelText(/^Assistant reasoning effort/)).toHaveProperty(
      "value",
      "high",
    );
    expect(
      screen.queryByLabelText(/^Assistant maximum output tokens per call/),
    ).toBeNull();
    expect(screen.getByLabelText(/^Assistant Codex home/)).toHaveProperty(
      "value",
      "/fixture/assistant-login",
    );
    fireEvent.change(screen.getByLabelText(/^Worker Codex home/), {
      target: { value: "/fixture/worker-login" },
    });
    fireEvent.change(screen.getByLabelText("Worker engine"), {
      target: { value: "openai-compatible" },
    });
    fireEvent.change(screen.getByLabelText(/^Worker custom model identifier/), {
      target: { value: "provider-model" },
    });
    fireEvent.change(screen.getByLabelText(/^Worker custom reasoning effort/), {
      target: { value: "low" },
    });
    expect(
      screen.getByLabelText(/^Worker API key environment variable/),
    ).toHaveProperty("value", "WORKER_KEY");
    expect(
      screen.getByLabelText(/^Worker maximum output tokens per call/),
    ).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Save preferences" }));
    await waitFor(() =>
      expect(
        calls.some(
          (c) => c.path === "/api/config" && c.options?.method === "PUT",
        ),
      ).toBe(true),
    );
    const saved = JSON.parse(
      calls.find(
        (c) => c.path === "/api/config" && c.options?.method === "PUT",
      )!.options!.body as string,
    );
    expect(saved.model).toEqual(model);
    expect(saved.worker_model).toEqual({
      ...config.worker_model,
      codex_home: "/fixture/worker-login",
      engine: "openai-compatible",
      model: "provider-model",
      effort: "low",
    });
  });
  it("requires an inspection note, preserves it after failure, and never retries interrupted work", async () => {
    state.pending_operations = [
      {
        id: "uncertain-operation",
        summary: "A worker dispatch result is unknown.",
      },
    ];
    let fail = true;
    respond = (path) => {
      if (path.endsWith("/acknowledge")) {
        if (fail)
          return {
            status: 409,
            body: { error: "Inspection could not be recorded." },
          };
        state = { ...state, pending_operations: [] };
        return { body: {} };
      }
      return { body: state };
    };
    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: /^Decisions/ }));
    expect(
      await screen.findByRole("button", { name: "Record inspection" }),
    ).toHaveProperty("disabled", true);
    expect(screen.queryByText("Nothing needs your decision")).toBeNull();
    const note = screen.getByLabelText("What did you find?");
    fireEvent.change(note, {
      target: { value: "Checked broker logs: dispatch was not accepted." },
    });
    fireEvent.click(screen.getByRole("button", { name: "Record inspection" }));
    expect(await screen.findByRole("alert")).toHaveProperty(
      "textContent",
      "Inspection could not be recorded.",
    );
    expect(note).toHaveProperty(
      "value",
      "Checked broker logs: dispatch was not accepted.",
    );
    fail = false;
    fireEvent.click(screen.getByRole("button", { name: "Record inspection" }));
    expect(await screen.findByText("Nothing needs your decision")).toBeTruthy();
    const writes = calls.filter((c) => c.options?.method === "POST");
    expect(writes).toHaveLength(2);
    expect(
      writes.every(
        (c) => c.path === "/api/operations/uncertain-operation/acknowledge",
      ),
    ).toBe(true);
    expect(JSON.parse(writes[0].options!.body as string)).toEqual({
      note: "Checked broker logs: dispatch was not accepted.",
    });
  });
  it("requires owner access before rendering private project data", async () => {
    respond = () => ({
      status: 401,
      body: { error: "Owner access required." },
    });
    render(<App />);
    expect(await screen.findByLabelText("Dashboard access code")).toBeTruthy();
    expect(
      screen.queryByRole("navigation", { name: "Main navigation" }),
    ).toBeNull();
  });
  it("removes a pairing token before exchanging it and reuses one request", async () => {
    window.history.replaceState(null, "", "/#token=one-use-fixture");
    const first = bootstrapSession();
    const second = bootstrapSession();
    expect(window.location.hash).toBe("");
    expect(first).toBe(second);
    await first;
    expect(calls.filter((c) => c.path === "/api/session")).toHaveLength(1);
    expect(window.localStorage.length).toBe(0);
  });
});
