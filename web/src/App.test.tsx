import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";

type Listener = (e: MessageEvent<string>) => void;

class FakeEventSource {
  static last: FakeEventSource | null = null;
  readonly url: string;
  onmessage: Listener | null = null;
  onerror: (() => void) | null = null;
  closed = false;
  private listeners = new Map<string, Listener[]>();

  constructor(url: string) {
    this.url = url;
    FakeEventSource.last = this;
  }

  addEventListener(type: string, listener: Listener) {
    this.listeners.set(type, [...(this.listeners.get(type) ?? []), listener]);
  }

  emit(type: string, data: unknown) {
    const e = new MessageEvent(type, { data: JSON.stringify(data) });
    if (type === "message") {
      this.onmessage?.(e);
    } else {
      this.listeners.get(type)?.forEach((listener) => listener(e));
    }
  }

  close() {
    this.closed = true;
  }
}

function source(): FakeEventSource {
  if (!FakeEventSource.last) {
    throw new Error("no event source was opened");
  }
  return FakeEventSource.last;
}

describe("App", () => {
  beforeEach(() => {
    FakeEventSource.last = null;
    vi.stubGlobal("EventSource", FakeEventSource);
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("shows the four columns and keeps the queue button disabled", () => {
    render(<App session="s1" />);
    for (const title of ["Blocked", "Working", "Verified", "Done"]) {
      expect(screen.getByRole("heading", { name: title })).toBeTruthy();
    }
    expect(screen.getByRole("button", { name: "Work the queue" }).hasAttribute("disabled")).toBe(
      true,
    );
  });

  it("subscribes to the session stream and shows its status and events", () => {
    render(<App session="s 1" />);
    expect(source().url).toBe("/api/sessions/s%201/events");
    expect(screen.getByRole("status").textContent).toBe("connecting");

    act(() => source().emit("hello", { session: "s 1", version: "v1.2.3", last: 0 }));
    expect(screen.getByRole("status").textContent).toBe("live · v1.2.3");

    act(() => source().emit("message", { seq: 4, kind: "pr.blocker", time: "", pr: 7 }));
    expect(screen.getByText("#4 pr.blocker · PR 7")).toBeTruthy();

    act(() => source().onerror?.());
    expect(screen.getByRole("status").textContent).toBe("reconnecting");
  });

  it("lists the three model tiers with their roles", () => {
    render(<App session="s1" />);
    for (const tier of ["Nano", "Super", "Ultra"]) {
      expect(screen.getByText(`Nemotron ${tier}`)).toBeTruthy();
    }
    expect(screen.getAllByText("no calls yet")).toHaveLength(3);
  });

  it("waits for the first event and then lists them", () => {
    render(<App session="s1" />);
    expect(screen.getByText("Waiting for the first event.")).toBeTruthy();
    act(() => source().emit("message", { seq: 1, kind: "run.started", time: "" }));
    expect(screen.queryByText("Waiting for the first event.")).toBeNull();
    expect(screen.getByText("#1 run.started")).toBeTruthy();
  });

  it("keeps a single live region and a skip link to the main landmark", () => {
    render(<App session="s1" />);
    expect(screen.getAllByRole("status")).toHaveLength(1);
    const skip = screen.getByRole("link", { name: "Skip to the board" });
    expect(skip.getAttribute("href")).toBe("#main");
    expect(screen.getByRole("main").id).toBe("main");
  });

  it("hides the decorative icons from assistive technology", () => {
    const { container } = render(<App session="s1" />);
    const icons = container.querySelectorAll("svg");
    expect(icons.length).toBeGreaterThan(0);
    for (const icon of icons) {
      expect(icon.getAttribute("aria-hidden")).toBe("true");
    }
  });

  it("closes the stream when it unmounts", () => {
    const { unmount } = render(<App session="s1" />);
    const opened = source();
    unmount();
    expect(opened.closed).toBe(true);
  });
});
