import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import type {
  Detail,
  EventKind,
  Info,
  ModelCallEvent,
  PullView,
  Snapshot,
  StepEvent,
  ToolCallEvent,
} from "./types";

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

let seq = 0;
function emit(kind: EventKind, pr: number | undefined, data?: unknown) {
  seq += 1;
  act(() => source().emit("message", { seq, kind, time: "2026-10-25T12:00:00Z", pr, data }));
}

const view = (number: number, extra: Partial<PullView> = {}): PullView => ({
  number,
  title: `Pull request ${number}`,
  author: {
    login: number === 3 ? "dependabot[bot]" : "alice",
    kind: number === 3 ? "bot" : "person",
  },
  branch: `feat/${number}`,
  kind: "change",
  state: "open",
  ...extra,
});

const snapshot = (overrides: Partial<Snapshot> = {}): Snapshot => ({
  id: "abcdef123456",
  status: "idle",
  created: "2026-10-25T12:00:00Z",
  prs: [view(1), view(2, { labels: ["upkeep"] }), view(3)],
  ...overrides,
});

const info: Info = {
  mode: "replay",
  version: "v1",
  sessions: 1,
  prs_per_run: 4,
  runs_per_server: 2,
  daily_tokens: 0,
  tokens_today: 0,
  rules: ["Never force-pushes.", "Never merges a pull request opened by a person."],
};

const detail: Detail = {
  ...view(2),
  outcome: "mergeable",
  blocker: "build",
  attempts: 1,
  duration_ms: 38000,
  summary: "Renames the call so the package builds again.",
  failure: "go build ./... failed:\n./caller.go:3:27: undefined: Add",
  patch:
    "diff --git a/caller.go b/caller.go\n--- a/caller.go\n+++ b/caller.go\n@@ -1,3 +1,3 @@\n-func Total() int { return Add(1, 2) }\n+func Total() int { return Sum(1, 2) }\n",
  changes: [{ path: "caller.go", added: 1, deleted: 1 }],
  checklist: [
    { id: "build", label: "go build passes", pass: true },
    { id: "tests", label: "tests pass", pass: true },
  ],
  forecast: {
    module: "example.com/lib",
    from: "v1.0.0",
    to: "v2.0.0",
    summary: "",
    breaking: [
      { symbol: "lib.Add", description: "renamed to Sum" },
      { symbol: "lib.Gone", description: "removed" },
    ],
    dropped: 1,
    excerpt: "",
    excerpt_source: "",
    files: ["caller.go"],
    sources: ["https://github.com/example/lib/releases"],
  },
  confirmed: ["lib.Add"],
  missed: ["lib.Gone"],
  receipt: {
    lines: [
      {
        model: "nvidia/super",
        role: "super",
        calls: 4,
        prompt_tokens: 4000,
        completion_tokens: 400,
        reasoning_tokens: 0,
        latency_ms: 5000,
        micros: 1900,
        baseline_micros: 6200,
      },
    ],
    steps: [{ name: "detect", detail: "build", duration_ms: 1200, ok: true }],
    calls: 4,
    attempts: 1,
    micros: 1900,
    baseline_micros: 6200,
  },
};

interface Server {
  calls: { method: string; path: string }[];
  snapshot: Snapshot;
  failCreate?: { status: number; message: string };
  missing?: boolean;
}

function installFetch(server: Server) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (path: string, init?: RequestInit) => {
      const method = init?.method ?? "GET";
      server.calls.push({ method, path });
      const json = (body: unknown, status = 200, headers: Record<string, string> = {}) =>
        new Response(JSON.stringify(body), {
          status,
          headers: { "Content-Type": "application/json", ...headers },
        });
      if (path === "/api/info") {
        return json(info);
      }
      if (method === "POST" && path === "/api/sessions") {
        if (server.failCreate) {
          return json({ error: server.failCreate.message }, server.failCreate.status, {
            "Retry-After": "30",
          });
        }
        return json(server.snapshot, 201);
      }
      if (method === "GET" && path === `/api/sessions/${server.snapshot.id}`) {
        return server.missing ? json({ error: "unknown session" }, 404) : json(server.snapshot);
      }
      if (method === "POST" && path.endsWith("/run")) {
        return json({ position: 0 }, 202);
      }
      if (method === "GET" && path.includes("/prs/")) {
        return json(detail);
      }
      if (method === "DELETE") {
        return new Response(null, { status: 204 });
      }
      return json({ error: "not found" }, 404);
    }),
  );
}

function newServer(overrides: Partial<Server> = {}): Server {
  return { calls: [], snapshot: snapshot(), ...overrides };
}

async function ready() {
  await screen.findByRole("button", { name: /Pull request 1/ });
  await waitFor(() => expect(FakeEventSource.last).not.toBeNull());
  await act(async () => {
    source().emit("hello", { session: "abcdef123456", version: "v1.2.3", last: 0 });
  });
}

function startedEvent() {
  emit("run.started", undefined, { prs: snapshot().prs });
}

describe("App", () => {
  beforeEach(() => {
    seq = 0;
    FakeEventSource.last = null;
    window.sessionStorage.clear();
    vi.stubGlobal("EventSource", FakeEventSource);
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("creates a session and shows every pull request as blocked", async () => {
    const server = newServer();
    installFetch(server);
    render(<App />);
    await ready();
    expect(
      server.calls.filter((c) => c.method === "POST" && c.path === "/api/sessions"),
    ).toHaveLength(1);
    expect(source().url).toBe("/api/sessions/abcdef123456/events");
    expect(window.sessionStorage.getItem("upkeep.session")).toBe("abcdef123456");
    expect(screen.getByRole("status").textContent).toBe("live · v1.2.3");
    expect(
      screen.getByText("3 pull requests are blocked. One click and Upkeep works through them."),
    ).toBeTruthy();
    expect(screen.getByRole("list", { name: "How it works" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Work the queue" }).hasAttribute("disabled")).toBe(
      false,
    );
    expect(await screen.findByText("Replay")).toBeTruthy();
  });

  it("keeps a single live region, a skip link and four labelled columns", async () => {
    installFetch(newServer());
    render(<App />);
    await ready();
    expect(screen.getAllByRole("status")).toHaveLength(1);
    expect(screen.getByRole("link", { name: "Skip to the board" }).getAttribute("href")).toBe(
      "#main",
    );
    for (const title of ["Blocked", "Working", "Verified", "Done"]) {
      expect(screen.getByRole("heading", { name: title })).toBeTruthy();
    }
  });

  it("starts a run and follows the pull requests across the board", async () => {
    const server = newServer();
    installFetch(server);
    render(<App />);
    await ready();
    fireEvent.click(screen.getByRole("button", { name: "Work the queue" }));
    await waitFor(() => expect(server.calls.some((c) => c.path.endsWith("/run"))).toBe(true));

    startedEvent();
    emit("pr.step", 1, { step: "detect", status: "running", detail: "" });
    emit("pr.blocker", 1, { blocker: "behind", behind: true });
    const working = screen.getByRole("heading", { name: "Working" }).closest("div") as HTMLElement;
    expect(within(working).getByRole("button", { name: /Pull request 1/ })).toBeTruthy();
    expect(within(working).getByText("behind main")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Working" }).hasAttribute("disabled")).toBe(true);

    emit("model.call", 1, {
      role: "super",
      model: "nvidia/super",
      latency_ms: 1200,
      prompt: 1000,
      completion: 100,
      micros: 400,
      baseline_micros: 1300,
    });
    expect(screen.getByText(/1 call · 1,100 tokens/)).toBeTruthy();

    emit("verify.result", 1, { passed: true, checks: [] });
    const verified = screen
      .getByRole("heading", { name: "Verified" })
      .closest("div") as HTMLElement;
    expect(within(verified).getByRole("button", { name: /Pull request 1/ })).toBeTruthy();

    emit("action", 1, { kind: "patch", status: "done", detail: "attached" });
    emit("pr.done", 1, {
      outcome: "mergeable",
      blocker: "behind",
      reason: "",
      attempts: 0,
      summary: "Merged main into the branch; nothing conflicted.",
      duration_ms: 4200,
      actions: [],
    });
    const done = screen.getByRole("heading", { name: "Done" }).closest("div") as HTMLElement;
    expect(within(done).getByText("Ready to merge")).toBeTruthy();
    expect(within(done).getByText("patch attached")).toBeTruthy();
    expect(within(done).getByText("Merged main into the branch; nothing conflicted.")).toBeTruthy();

    for (const n of [2, 3]) {
      emit("pr.done", n, {
        outcome: n === 3 ? "merged" : "needs_human",
        blocker: "clean",
        reason: "",
        attempts: 0,
        summary: "",
        duration_ms: 1000,
        actions: [],
      });
    }
    emit("run.finished", undefined, {
      counts: { mergeable: 1, needs_human: 1, merged: 1 },
      micros: 400,
      baseline_micros: 1300,
      calls: 1,
    });
    expect(screen.getByText("Done: 1 merged · 1 ready to merge · 1 needs a person.")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Run again" })).toBeTruthy();
  });

  it("shows the queue position while waiting for a slot", async () => {
    installFetch(newServer());
    render(<App />);
    await ready();
    emit("run.queued", undefined, { position: 2 });
    expect(
      screen.getByText("You are number 2 in line. Your run starts as soon as a slot opens."),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Waiting for a slot" }).hasAttribute("disabled"),
    ).toBe(true);
  });

  it("totals the models and compares the cost with the largest model's price", async () => {
    installFetch(newServer());
    render(<App />);
    await ready();
    startedEvent();
    emit("model.call", 1, {
      role: "super",
      model: "nvidia/super",
      latency_ms: 2000,
      prompt: 3000,
      completion: 200,
      micros: 1000,
      baseline_micros: 3200,
    });
    emit("model.call", 2, {
      role: "ultra",
      model: "nvidia/ultra",
      latency_ms: 1500,
      prompt: 1000,
      completion: 100,
      micros: 2000,
      baseline_micros: 2000,
    });
    const rail = screen.getByRole("region", { name: "Models" });
    expect(within(rail).getByText(/1 call · 3,200 tokens · 2\.0 s · \$0\.0010/)).toBeTruthy();
    expect(within(rail).getByText(/1 call · 1,100 tokens · 1\.5 s · \$0\.0020/)).toBeTruthy();
    expect(within(rail).getByText("no calls yet")).toBeTruthy();
    expect(within(rail).getByText("$0.0030")).toBeTruthy();
    expect(within(rail).getByText("$0.0052")).toBeTruthy();
    expect(
      within(rail).getByText(/not a claim that the largest model would have fixed these/),
    ).toBeTruthy();
  });

  it("opens the details of a pull request and closes them with Escape", async () => {
    installFetch(newServer());
    render(<App />);
    await ready();
    const card = screen.getByRole("button", { name: /Pull request 2/ });
    card.focus();
    fireEvent.click(card);
    const dialog = await screen.findByRole("dialog", { name: "#2 Pull request 2" });
    await within(dialog).findByText("Renames the call so the package builds again.");
    expect(within(dialog).getByText("Ready to merge")).toBeTruthy();
    expect(within(dialog).getByText("confirmed by the build")).toBeTruthy();
    expect(within(dialog).getByText("not seen in the build")).toBeTruthy();
    expect(within(dialog).getByText(/1 predicted change was dropped/)).toBeTruthy();
    expect(within(dialog).getByText("go build passes")).toBeTruthy();
    expect(within(dialog).getByText("nvidia/super")).toBeTruthy();
    expect(within(dialog).getByText("+func Total() int { return Sum(1, 2) }")).toBeTruthy();
    expect(
      within(dialog)
        .getByRole("link", { name: "github.com/example/lib/releases" })
        .getAttribute("rel"),
    ).toBe("noreferrer noopener");
    expect(document.activeElement).toBe(within(dialog).getByRole("button", { name: "Close" }));

    fireEvent.keyDown(document, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).toBe(card);
  });

  it("follows a running pull request in its trace and reloads the summary when it finishes", async () => {
    const server = newServer();
    installFetch(server);
    render(<App />);
    await ready();
    const original = globalThis.fetch;
    const running: Detail = { ...view(2), attempts: 0 };
    let finish: (r: Response) => void = () => undefined;
    let gets = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string, init?: RequestInit) => {
        if (!path.includes("/prs/")) {
          return original(path, init);
        }
        gets += 1;
        if (gets === 1) {
          return new Response(JSON.stringify(running), { status: 200 });
        }
        return new Promise<Response>((resolve) => {
          finish = resolve;
        });
      }),
    );
    startedEvent();
    emit("pr.step", 2, { step: "detect", status: "done", detail: "build" } satisfies StepEvent);
    emit("pr.step", 2, {
      step: "plan",
      status: "done",
      detail: "build fails: Super repairs it with 4 tools, up to 3 attempts, then 9 checks",
    } satisfies StepEvent);
    emit("pr.step", 2, {
      step: "repair",
      status: "running",
      detail: "attempt 1 of 3",
      attempt: 1,
      of: 3,
    } satisfies StepEvent);
    emit("model.call", 2, {
      model: "nvidia/super",
      role: "super",
      latency_ms: 1500,
      prompt: 2000,
      completion: 100,
      micros: 300,
      baseline_micros: 1000,
      step: "repair",
      purpose: "fix",
      attempt: 1,
      turn: 1,
      said: "The caller still uses Add.",
      tools: ["apply_edit", "run_checks"],
    } satisfies ModelCallEvent);

    fireEvent.click(screen.getByRole("button", { name: /Pull request 2/ }));
    const dialog = await screen.findByRole("dialog", { name: "#2 Pull request 2" });
    expect(within(dialog).getByRole("tab", { name: "Trace" }).getAttribute("aria-selected")).toBe(
      "true",
    );
    const panel = within(dialog).getByRole("tabpanel", { name: "Trace" });
    expect(panel.closest("[aria-live]")).toBeNull();
    expect(
      within(panel).getByText(
        "build fails: Super repairs it with 4 tools, up to 3 attempts, then 9 checks",
      ),
    ).toBeTruthy();
    expect(within(panel).getByRole("heading", { name: /Attempt 1 of 3/ })).toBeTruthy();
    expect(within(panel).getByText("The caller still uses Add.")).toBeTruthy();
    expect(within(panel).getByText("1.5 s · 2,100 tokens · $0.0003")).toBeTruthy();

    emit("tool.call", 2, {
      tool: "apply_edit",
      ok: true,
      detail: "apply_edit",
      attempt: 1,
      turn: 1,
      path: "caller.go",
      added: 1,
      removed: 1,
      result: "ok",
    } satisfies ToolCallEvent);
    expect(within(panel).getByText("caller.go")).toBeTruthy();
    expect(within(panel).getByText("apply_edit")).toBeTruthy();
    await waitFor(() => expect(gets).toBe(1));

    fireEvent.click(within(dialog).getByRole("tab", { name: "Summary" }));
    await within(dialog).findByText("Not worked yet");

    emit("tool.call", 2, {
      tool: "run_checks",
      ok: true,
      detail: "PASS",
      attempt: 1,
      turn: 1,
      result: "PASS",
    } satisfies ToolCallEvent);
    emit("pr.step", 2, {
      step: "repair",
      status: "done",
      detail: "attempt 1: checks passed after 1 turns, 1 edits in 1 files, 2.0s",
      attempt: 1,
      of: 3,
      stop: "checks passed",
      turns: 1,
      edits: 1,
      files: 1,
    } satisfies StepEvent);
    emit("verify.result", 2, { passed: true, checks: detail.checklist });
    emit("action", 2, { kind: "push", status: "done", detail: "pushed" });
    emit("pr.done", 2, {
      outcome: "mergeable",
      blocker: "build",
      reason: "",
      attempts: 1,
      summary: "Renames the call so the package builds again.",
      duration_ms: 38000,
      actions: [],
    });
    await waitFor(() => expect(gets).toBe(2));
    expect(within(dialog).getByText("Not worked yet")).toBeTruthy();
    expect(within(dialog).queryByText("Loading the details.")).toBeNull();

    await act(async () => finish(new Response(JSON.stringify(detail), { status: 200 })));
    await within(dialog).findByText("Renames the call so the package builds again.");
    expect(within(dialog).getByText("go build passes")).toBeTruthy();
    expect(within(dialog).queryByText("Not worked yet")).toBeNull();
  });

  it("moves between the trace and the summary with the keyboard", async () => {
    installFetch(newServer());
    render(<App />);
    await ready();
    startedEvent();
    emit("pr.step", 1, { step: "detect", status: "running", detail: "" });
    fireEvent.click(screen.getByRole("button", { name: /Pull request 1/ }));
    const dialog = await screen.findByRole("dialog", { name: "#1 Pull request 1" });
    const trace = within(dialog).getByRole("tab", { name: "Trace" });
    const summary = within(dialog).getByRole("tab", { name: "Summary" });
    expect(within(dialog).getByRole("tablist", { name: "Pull request details" })).toBeTruthy();
    expect([trace.tabIndex, summary.tabIndex]).toEqual([0, -1]);

    trace.focus();
    fireEvent.keyDown(trace, { key: "ArrowRight" });
    expect(summary.getAttribute("aria-selected")).toBe("true");
    expect(document.activeElement).toBe(summary);
    expect([trace.tabIndex, summary.tabIndex]).toEqual([-1, 0]);
    await within(dialog).findByText("Renames the call so the package builds again.");

    fireEvent.keyDown(summary, { key: "Home" });
    expect(document.activeElement).toBe(trace);
    fireEvent.keyDown(trace, { key: "End" });
    expect(document.activeElement).toBe(summary);
    fireEvent.keyDown(summary, { key: "ArrowRight" });
    expect(document.activeElement).toBe(trace);
    fireEvent.keyDown(trace, { key: "ArrowLeft" });
    expect(document.activeElement).toBe(summary);
    fireEvent.keyDown(summary, { key: "ArrowLeft" });
    expect(trace.getAttribute("aria-selected")).toBe("true");

    emit("pr.done", 1, {
      outcome: "mergeable",
      blocker: "behind",
      reason: "",
      attempts: 0,
      summary: "",
      duration_ms: 1000,
      actions: [],
    });
    const more = within(dialog).getByRole("button", { name: "See the summary" });
    expect(within(dialog).getByText(/Finished: Ready to merge/)).toBeTruthy();
    more.focus();
    fireEvent.click(more);
    expect(summary.getAttribute("aria-selected")).toBe("true");
    expect(document.activeElement).toBe(summary);
  });

  it("links only web addresses from the sources of a forecast", async () => {
    installFetch(newServer());
    render(<App />);
    await ready();
    const original = globalThis.fetch;
    const odd = {
      ...detail,
      forecast: {
        ...detail.forecast!,
        sources: [
          "javascript:alert(1)",
          "data:text/html,x",
          "https://example.com/notes",
          "HTTP://example.org/old",
        ],
      },
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string, init?: RequestInit) =>
        path.includes("/prs/")
          ? new Response(JSON.stringify(odd), { status: 200 })
          : original(path, init),
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: /Pull request 2/ }));
    const dialog = await screen.findByRole("dialog", { name: "#2 Pull request 2" });
    await within(dialog).findByText("confirmed by the build");
    const hrefs = within(dialog)
      .getAllByRole("link")
      .map((a) => a.getAttribute("href"));
    expect(hrefs).toEqual(["https://example.com/notes", "HTTP://example.org/old"]);
  });

  it("explains a detail that cannot be loaded", async () => {
    const server = newServer();
    installFetch(server);
    render(<App />);
    await ready();
    const original = globalThis.fetch;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (path: string, init?: RequestInit) =>
        path.includes("/prs/")
          ? new Response(JSON.stringify({ error: "unknown pull request" }), { status: 404 })
          : original(path, init),
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: /Pull request 1/ }));
    expect((await screen.findByRole("alert")).textContent).toBe("unknown pull request");
  });

  it("says what replay means and links to the documentation", async () => {
    installFetch(newServer());
    render(<App />);
    await ready();
    expect(
      await screen.findByText(/The model answers come from a recorded run of Nemotron/),
    ).toBeTruthy();
    const nav = screen.getByRole("navigation", { name: "Project links" });
    const links = within(nav).getAllByRole("link");
    expect(links.map((l) => l.textContent)).toEqual([
      "Source",
      "How it works",
      "Which model does what",
      "Limitations",
    ]);
    for (const link of links) {
      expect(link.getAttribute("rel")).toBe("noreferrer noopener");
    }
  });

  it("shows the rules the server runs with", async () => {
    installFetch(newServer());
    render(<App />);
    await ready();
    fireEvent.click(screen.getByRole("button", { name: "What Upkeep may do" }));
    const dialog = await screen.findByRole("dialog", { name: "What Upkeep may do" });
    expect(within(dialog).getByText("Never force-pushes.")).toBeTruthy();
    expect(
      within(dialog).getByText("Never merges a pull request opened by a person."),
    ).toBeTruthy();
  });

  it("reuses a remembered session and creates another when it is gone", async () => {
    window.sessionStorage.setItem("upkeep.session", "abcdef123456");
    const server = newServer();
    installFetch(server);
    render(<App />);
    await ready();
    expect(server.calls.some((c) => c.method === "POST" && c.path === "/api/sessions")).toBe(false);
    cleanup();

    const gone = newServer({ missing: true });
    installFetch(gone);
    render(<App />);
    await ready();
    expect(
      gone.calls.filter((c) => c.method === "POST" && c.path === "/api/sessions"),
    ).toHaveLength(1);
  });

  it("asks the visitor to come back when the demo is full", async () => {
    installFetch(newServer({ failCreate: { status: 503, message: "the demo is full" } }));
    render(<App />);
    expect((await screen.findByRole("alert")).textContent).toBe(
      "The demo is full right now. Try again in a minute.",
    );
  });

  it("starts a new session after the run, and when the session was closed", async () => {
    const server = newServer();
    installFetch(server);
    render(<App />);
    await ready();
    startedEvent();
    emit("run.finished", undefined, { counts: {}, micros: 0, baseline_micros: 0, calls: 0 });
    fireEvent.click(screen.getByRole("button", { name: "Run again" }));
    await waitFor(() => expect(server.calls.filter((c) => c.method === "DELETE")).toHaveLength(1));
    await waitFor(() =>
      expect(
        server.calls.filter((c) => c.method === "POST" && c.path === "/api/sessions"),
      ).toHaveLength(2),
    );

    await screen.findByRole("button", { name: "Work the queue" });
    emit("session.closed", undefined, {});
    expect(
      screen.getByText("This session ended. Press the button to start a new one."),
    ).toBeTruthy();
  });

  it("shows notices from the server once", async () => {
    installFetch(newServer());
    render(<App />);
    await ready();
    emit("notice", undefined, {
      message: "The live token budget for today is used up, so this run replays a recorded one.",
    });
    emit("notice", undefined, {
      message: "The live token budget for today is used up, so this run replays a recorded one.",
    });
    expect(screen.getAllByRole("note")).toHaveLength(1);
    expect(screen.getAllByRole("note")[0]?.textContent).toContain(
      "live token budget for today is used up",
    );
  });

  it("shows reconnecting when the stream drops and closes it on unmount", async () => {
    installFetch(newServer());
    const { unmount } = render(<App />);
    await ready();
    act(() => source().onerror?.());
    expect(screen.getByRole("status").textContent).toBe("reconnecting");
    const opened = source();
    unmount();
    expect(opened.closed).toBe(true);
  });
});
