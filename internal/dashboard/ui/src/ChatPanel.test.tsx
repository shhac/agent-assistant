// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { ChatPanel } from "./ChatPanel";
import { ConversationMarkdown } from "./ConversationMarkdown";
import { normalizeState, type State } from "./api";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
const initial = () =>
  normalizeState({ assistant: { name: "Iris", personality: "" } });
function panel(state = initial(), refresh = vi.fn(async () => {})) {
  return (
    <ChatPanel
      state={state}
      refresh={refresh}
      expanded={false}
      onExpand={() => {}}
      onClose={() => {}}
    />
  );
}
function deferredFetch() {
  let resolve!: (result: unknown) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise((yes, no) => {
    resolve = yes;
    reject = no;
  });
  const fetch = vi.fn(() => promise);
  vi.stubGlobal("fetch", fetch);
  return {
    fetch,
    resolve: async () => {
      await act(async () => {
        resolve({ ok: true, json: async () => ({}) });
      });
    },
    reject: async () => {
      await act(async () => {
        reject(new Error("Connection lost"));
      });
    },
  };
}
function typeAndSend(text: string) {
  const input = screen.getByRole("textbox");
  fireEvent.change(input, { target: { value: text } });
  fireEvent.keyDown(input, { key: "Enter", code: "Enter" });
  return input as HTMLTextAreaElement;
}
describe("conversation", () => {
  it("shows a sent turn immediately, accepts a next draft and reconciles polling without duplication", async () => {
    const request = deferredFetch();
    const state = initial();
    const view = render(panel(state));
    const input = typeAndSend("Please coordinate this");
    expect(
      within(screen.getByRole("log")).getAllByText("Please coordinate this"),
    ).toHaveLength(1);
    expect(input.value).toBe("");
    expect(screen.getByText("Iris is working through it…")).toBeTruthy();
    fireEvent.change(input, { target: { value: "Another thought" } });
    fireEvent.keyDown(input, { key: "Enter" });
    expect(request.fetch).toHaveBeenCalledTimes(1);
    state.messages = [
      { id: "saved", role: "user", content: "Please coordinate this" },
    ];
    view.rerender(panel(state));
    expect(
      within(screen.getByRole("log")).getAllByText("Please coordinate this"),
    ).toHaveLength(1);
    await request.resolve();
    expect(input.value).toBe("Another thought");
    expect(
      (
        screen.getByRole("button", {
          name: "Send message",
        }) as HTMLButtonElement
      ).disabled,
    ).toBe(false);
  });
  it("does not submit Shift+Enter or an IME composition", async () => {
    const request = deferredFetch();
    render(panel());
    const input = screen.getByRole("textbox");
    fireEvent.change(input, { target: { value: "A draft" } });
    fireEvent.keyDown(input, { key: "Enter", shiftKey: true });
    fireEvent.keyDown(input, { key: "Enter", isComposing: true });
    fireEvent.keyDown(input, { key: "Enter", keyCode: 229 });
    expect(request.fetch).not.toHaveBeenCalled();
    fireEvent.keyDown(input, { key: "Enter" });
    expect(request.fetch).toHaveBeenCalledTimes(1);
    await request.resolve();
  });
  it("keeps a failed message recoverable and preserves a next draft without replaying", async () => {
    const request = deferredFetch();
    render(panel());
    const input = typeAndSend("My original request");
    fireEvent.change(input, { target: { value: "A second thought" } });
    await request.reject();
    expect(screen.getByText("Delivery unconfirmed")).toBeTruthy();
    expect(input.value).toBe("A second thought");
    fireEvent.click(
      screen.getByRole("button", { name: "Restore message to draft" }),
    );
    expect(input.value).toBe("A second thought\n\nMy original request");
    expect(request.fetch).toHaveBeenCalledTimes(1);
  });
  it("recognizes persisted failed turns and does not confuse an older identical message", async () => {
    const request = deferredFetch();
    const state = initial();
    state.messages = [{ id: "older", role: "user", content: "Try it" }];
    const view = render(panel(state));
    typeAndSend("Try it");
    expect(within(screen.getByRole("log")).getAllByText("Try it")).toHaveLength(
      2,
    );
    state.messages = [
      ...state.messages,
      { id: "new", role: "user", content: "Try it" },
    ];
    view.rerender(panel(state));
    await request.reject();
    expect(within(screen.getByRole("log")).getAllByText("Try it")).toHaveLength(
      2,
    );
    expect(screen.getByText("Received · reply interrupted")).toBeTruthy();
  });
  it("renders formatting and project names while suppressing unsafe HTML, URLs and image requests", () => {
    const open = vi.fn();
    const state: State = initial();
    state.projects = [
      { id: "proj-123", title: "Garden planner" } as State["projects"][number],
    ];
    const view = render(
      <ConversationMarkdown
        projects={state.projects}
        onProjectOpen={open}
        content={
          '**Ready** with `notes`\n\n- A task\n\n[proj-123](#/projects/proj-123)\n\n[Unsafe](javascript:alert%281%29)\n\n[Docs](https://example.org)\n\n<img src="x" onerror="alert(1)"><script>alert(1)</script>\n\n![tracking](https://example.org/pixel.png)'
        }
      />,
    );
    expect(view.container.querySelector("strong")?.textContent).toBe("Ready");
    expect(view.container.querySelector("code")?.textContent).toBe("notes");
    expect(view.container.querySelector("li")?.textContent).toBe("A task");
    expect(view.container.querySelectorAll("script,img")).toHaveLength(0);
    expect(screen.getByText("Unsafe").getAttribute("href")).toBe("");
    expect(screen.getByRole("link", { name: "Docs" }).getAttribute("rel")).toBe(
      "noopener noreferrer",
    );
    fireEvent.click(screen.getByRole("link", { name: "Garden planner" }));
    expect(open).toHaveBeenCalledWith("proj-123");
  });
});
