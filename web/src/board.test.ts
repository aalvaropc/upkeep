import { describe, expect, it } from "vitest";
import {
  boardReducer,
  columns,
  dollars,
  initialBoard,
  outcomeLabel,
  seconds,
  type BoardAction,
  type BoardState,
} from "./board";
import { MAX_TRACE, traceRows, traceSections } from "./trace";
import type { EventKind, ModelCallEvent, PullRequest, PullView, ToolCallEvent } from "./types";

const pr = (number: number, extra: Partial<PullRequest> = {}): PullRequest => ({
  number,
  title: `Pull request ${number}`,
  author: { login: "alice", kind: "person" },
  branch: `feat/${number}`,
  kind: "change",
  ...extra,
});

let seq = 0;
function event(kind: EventKind, prNumber: number | undefined, data?: unknown): BoardAction {
  seq += 1;
  return { type: "event", event: { seq, kind, time: "2026-10-25T12:00:00Z", pr: prNumber, data } };
}

function run(actions: BoardAction[], from: BoardState = initialBoard): BoardState {
  return actions.reduce(boardReducer, from);
}

function started(...numbers: number[]): BoardAction {
  return event("run.started", undefined, { prs: numbers.map((n) => pr(n)) });
}

describe("board reducer", () => {
  it("starts empty and connecting, then goes live", () => {
    expect(initialBoard.connection).toBe("connecting");
    const live = run([{ type: "hello", session: "s1", version: "v1" }]);
    expect(live).toMatchObject({ connection: "live", session: "s1", version: "v1" });
    expect(run([{ type: "lost" }], live).connection).toBe("lost");
    expect(run([{ type: "ping" }], run([{ type: "lost" }], live)).connection).toBe("live");
  });

  it("seeds the cards from a snapshot before a run starts and ignores a late one", () => {
    const views: PullView[] = [
      { ...pr(2), state: "open" },
      { ...pr(1), state: "open" },
    ];
    const seeded = run([{ type: "seed", prs: views, run: "idle" }]);
    expect(seeded.order).toEqual([2, 1]);
    expect(seeded.cards[1]?.phase).toBe("queued");
    const running = run([started(1, 2)], seeded);
    expect(running.run).toBe("running");
    expect(boardReducer(running, { type: "seed", prs: [], run: "idle" })).toBe(running);
  });

  it("marks pull requests that already finished when it seeds a reloaded session", () => {
    const views: PullView[] = [
      {
        ...pr(6),
        state: "merged",
        outcome: "merged",
        blocker: "clean",
        actions: [{ kind: "merge", status: "done" }],
      },
    ];
    const seeded = run([{ type: "seed", prs: views, run: "finished" }]);
    expect(seeded.cards[6]).toMatchObject({ phase: "done", outcome: "merged", blocker: "clean" });
    expect(seeded.run).toBe("finished");
  });

  it("creates the cards in number order when a run starts", () => {
    const state = run([started(3, 1, 2)]);
    expect(state.order).toEqual([1, 2, 3]);
    expect(columns(state).queued).toHaveLength(3);
  });

  it("moves a card to working at its first step and keeps the latest step", () => {
    const state = run([
      started(1, 2),
      event("pr.step", 1, { step: "detect", status: "running", detail: "" }),
    ]);
    expect(columns(state).working.map((c) => c.pr.number)).toEqual([1]);
    expect(columns(state).queued.map((c) => c.pr.number)).toEqual([2]);
    expect(state.cards[1]?.step).toMatchObject({ name: "detect", status: "running" });
    expect(state.cards[1]?.startedAt).toBe(Date.parse("2026-10-25T12:00:00Z"));
  });

  it("counts repair attempts from their running steps", () => {
    const state = run([
      started(1),
      event("pr.step", 1, { step: "repair", status: "running", detail: "attempt 1 of 3" }),
      event("pr.step", 1, { step: "repair", status: "failed", detail: "attempt 1" }),
      event("pr.step", 1, { step: "repair", status: "running", detail: "attempt 2 of 3" }),
    ]);
    expect(state.cards[1]?.attempts).toBe(2);
    expect(state.cards[1]?.steps).toHaveLength(3);
  });

  it("totals model calls per role, per card and overall", () => {
    const state = run([
      started(1, 2),
      event("model.call", 1, {
        role: "super",
        model: "nvidia/super",
        latency_ms: 1200,
        prompt: 1000,
        completion: 100,
        micros: 400,
        baseline_micros: 1300,
      }),
      event("model.call", 1, {
        role: "super",
        model: "nvidia/super",
        latency_ms: 800,
        prompt: 500,
        completion: 50,
        micros: 200,
        baseline_micros: 650,
      }),
      event("model.call", 2, {
        role: "ultra",
        model: "nvidia/ultra",
        latency_ms: 2000,
        prompt: 2000,
        completion: 200,
        micros: 3000,
        baseline_micros: 3000,
      }),
    ]);
    expect(state.models.super).toMatchObject({
      calls: 2,
      promptTokens: 1500,
      completionTokens: 150,
      latencyMs: 2000,
      micros: 600,
      baselineMicros: 1950,
      models: ["nvidia/super"],
    });
    expect(state.models.ultra?.calls).toBe(1);
    expect(state.totals).toEqual({
      calls: 3,
      micros: 3600,
      baselineMicros: 4950,
      promptTokens: 3500,
      completionTokens: 350,
    });
    expect(state.cards[1]).toMatchObject({
      calls: 2,
      micros: 600,
      baselineMicros: 1950,
      phase: "working",
    });
  });

  it("verifies a card only when every check passed", () => {
    const checks = [{ id: "build", label: "go build passes", pass: true }];
    const passed = run([
      started(1),
      event("pr.step", 1, { step: "detect", status: "done", detail: "" }),
      event("verify.result", 1, { passed: true, checks }),
    ]);
    expect(passed.cards[1]).toMatchObject({ phase: "verified", verified: true, checks });
    const failed = run([
      started(1),
      event("pr.step", 1, { step: "detect", status: "done", detail: "" }),
      event("verify.result", 1, { passed: false, checks }),
    ]);
    expect(failed.cards[1]).toMatchObject({ phase: "working", verified: false });
  });

  it("finishes a card with its outcome, note, actions and duration", () => {
    const state = run([
      started(4),
      event("action", 4, { kind: "push", status: "done", detail: "pushed abc" }),
      event("receipt", 4, { calls: 7, micros: 1900, baseline_micros: 6200, attempts: 1 }),
      event("pr.done", 4, {
        outcome: "mergeable",
        blocker: "build",
        reason: "",
        attempts: 1,
        summary: "Renames the call.",
        flaky: ["TestA"],
        actions: [],
        duration_ms: 38900,
      }),
    ]);
    expect(state.cards[4]).toMatchObject({
      phase: "done",
      outcome: "mergeable",
      blocker: "build",
      summary: "Renames the call.",
      flaky: ["TestA"],
      durationMs: 38900,
      attempts: 1,
      micros: 1900,
      baselineMicros: 6200,
    });
    expect(state.cards[4]?.actions).toEqual([
      { kind: "push", status: "done", detail: "pushed abc" },
    ]);
  });

  it("records the blocker and whether the branch is behind", () => {
    const state = run([started(1), event("pr.blocker", 1, { blocker: "behind", behind: true })]);
    expect(state.cards[1]).toMatchObject({ blocker: "behind", behind: true });
  });

  it("tracks waiting, running and finished runs", () => {
    const waiting = run([event("run.queued", undefined, { position: 2 })]);
    expect(waiting).toMatchObject({ run: "waiting", position: 2 });
    const finished = run(
      [
        started(1),
        event("run.finished", undefined, {
          counts: { merged: 1 },
          micros: 3600,
          baseline_micros: 9000,
          calls: 4,
        }),
      ],
      waiting,
    );
    expect(finished).toMatchObject({
      run: "finished",
      position: 0,
      summary: { counts: { merged: 1 }, micros: 3600, baselineMicros: 9000, calls: 4 },
    });
    const stopped = run([event("run.finished", undefined, { error: "no key" })]);
    expect(stopped.summary?.error).toBe("no key");
  });

  it("keeps notices once each and notes a closed session", () => {
    const state = run([
      event("notice", undefined, { message: "Replaying." }),
      event("notice", undefined, { message: "Replaying." }),
      event("session.closed", undefined, {}),
    ]);
    expect(state.notices).toEqual(["Replaying."]);
    expect(state.closed).toBe(true);
  });

  it("ignores events it has already seen and keeps a bounded newest-first log", () => {
    const first = event("run.queued", undefined, { position: 1 });
    const once = run([first]);
    expect(boardReducer(once, first)).toBe(once);
    let many = initialBoard;
    for (let i = 0; i < 200; i++) {
      many = boardReducer(
        many,
        event("tool.call", 1, { tool: "read_file", ok: true, detail: "x" }),
      );
    }
    expect(many.log).toHaveLength(120);
    expect(many.log[0]?.seq).toBeGreaterThan(many.log[119]?.seq ?? 0);
  });

  it("ignores events for pull requests it does not know", () => {
    const state = run([event("pr.step", 99, { step: "detect", status: "running", detail: "" })]);
    expect(state.cards[99]).toBeUndefined();
    expect(state.log).toHaveLength(1);
  });

  it("describes events in plain words", () => {
    const state = run([
      started(1),
      event("pr.step", 1, { step: "repair", status: "running", detail: "attempt 1 of 3" }),
      event("model.call", 1, { role: "super", latency_ms: 1500, micros: 250 }),
    ]);
    expect(state.log.map((l) => l.text)).toEqual([
      "super call, 1.5 s, $0.0003",
      "repair running: attempt 1 of 3",
      "run started",
    ]);
  });

  it("totals web searches apart from the model calls and describes them", () => {
    const state = run([
      started(1, 2),
      event("search.call", 1, {
        provider: "tavily",
        query: "example.com/lib migration guide",
        results: [
          { title: "Guide", url: "https://example.com/lib/guide", score: 0.9, kept: true },
          { title: "Repo", url: "https://example.com/lib", score: 0.7, kept: true },
          { title: "Other", url: "https://example.com/other", score: 0.2, kept: false },
        ],
        kept: ["https://example.com/lib/guide", "https://example.com/lib"],
        latency_ms: 900,
        recorded: false,
        reused: false,
      }),
      event("search.call", 2, {
        provider: "tavily",
        query: "example.com/other v1.2.0 release notes changelog",
        results: [],
        kept: [],
        latency_ms: 300,
        recorded: false,
        reused: false,
        error: "tavily returned HTTP 432",
      }),
    ]);
    expect(state.search).toEqual({
      provider: "tavily",
      calls: 2,
      kept: 2,
      results: 3,
      failed: 1,
      skipped: 0,
      recorded: false,
      lastQuery: "example.com/other v1.2.0 release notes changelog",
      lastProblem: "tavily returned HTTP 432",
    });
    expect(state.totals.calls).toBe(0);
    expect(state.models).toEqual({});
    expect(state.log.slice(0, 2).map((l) => l.text)).toEqual([
      "Tavily search failed: tavily returned HTTP 432",
      "Tavily search: kept 2 of 3 pages for “example.com/lib migration guide”",
    ]);
  });

  it("counts a skipped search apart and marks a recorded one", () => {
    const state = run([
      started(1, 2),
      event("search.call", 1, {
        provider: "tavily",
        query: "q",
        kept: [],
        skipped: "the daily search limit is reached",
      }),
      event("search.call", 2, {
        provider: "tavily",
        query: "github.com/golang-jwt/jwt/v5 migration guide",
        kept: ["a", "b", "c", "d"],
        recorded: true,
      }),
    ]);
    expect(state.search).toMatchObject({ calls: 1, skipped: 1, kept: 4, recorded: true });
    expect(state.search.lastProblem).toBe("");
    expect(state.log.slice(0, 2).map((l) => l.text)).toEqual([
      "Tavily search (recorded): kept 4 pages for “github.com/golang-jwt/jwt/v5 migration guide”",
      "Tavily search skipped: the daily search limit is reached",
    ]);
    expect(run([{ type: "reset" }], state).search).toEqual(initialBoard.search);
  });

  it("counts gaps and resets", () => {
    const state = run([{ type: "gap" }, { type: "gap" }]);
    expect(state.gaps).toBe(2);
    expect(run([{ type: "reset" }], run([started(1)]))).toEqual(initialBoard);
  });
});

const superCall = (attempt: number, turn: number, tools: string[]): ModelCallEvent => ({
  model: "nvidia/super",
  role: "super",
  latency_ms: 1000,
  prompt: 1000,
  completion: 100,
  micros: 300,
  baseline_micros: 1000,
  step: "repair",
  purpose: "fix",
  attempt,
  turn,
  said: "Checking the caller.",
  tools,
});

const checks = (attempt: number, turn: number, result: string): ToolCallEvent => ({
  tool: "run_checks",
  ok: true,
  detail: "go build ./... failed:",
  attempt,
  turn,
  result,
});

describe("trace", () => {
  it("records each step, model call, tool call, verdict and action of a card", () => {
    const state = run([
      started(1),
      event("pr.step", 1, { step: "detect", status: "running", detail: "" }),
      event("pr.step", 1, { step: "detect", status: "done", detail: "build" }),
      event("pr.blocker", 1, { blocker: "build", behind: false }),
      event("pr.step", 1, {
        step: "plan",
        status: "done",
        detail: "build fails: Super repairs it with 4 tools, up to 3 attempts, then 9 checks",
      }),
      event("pr.step", 1, { step: "repair", status: "running", attempt: 1, of: 3 }),
      event("model.call", 1, superCall(1, 1, ["apply_edit"])),
      event("tool.call", 1, {
        tool: "apply_edit",
        ok: true,
        detail: "apply_edit",
        attempt: 1,
        turn: 1,
        path: "auth/token.go",
        added: 3,
        removed: 1,
        result: "ok",
      } satisfies ToolCallEvent),
      event("pr.step", 1, { step: "detect", status: "running", detail: "" }),
      event("verify.result", 1, { passed: true, checks: [] }),
      event("receipt", 1, { calls: 1, micros: 300, baseline_micros: 1000, attempts: 1 }),
      event("action", 1, { kind: "push", status: "done", detail: "pushed abc" }),
    ]);
    const trace = state.cards[1]?.trace ?? [];
    expect(trace.map((e) => `${e.phase}:${e.kind}`)).toEqual([
      "plan:step",
      "plan:step",
      "plan:step",
      "fix:step",
      "fix:model",
      "fix:tool",
      "fix:step",
      "verify:verify",
      "act:action",
    ]);
    expect(trace[5]).toMatchObject({
      kind: "tool",
      target: "auth/token.go",
      added: 3,
      removed: 1,
      attempt: 1,
    });
    expect(trace[4]).toMatchObject({ kind: "model", tokens: 1100, tools: ["apply_edit"] });
    expect(state.log.map((l) => l.text)).toContain("Super edited auth/token.go (+3 −1)");
  });

  it("groups the fix by attempt and puts each failure in its attempt", () => {
    const state = run([
      started(2),
      event("pr.step", 2, { step: "plan", status: "done", detail: "build fails" }),
      event("pr.step", 2, {
        step: "repair",
        status: "running",
        detail: "attempt 1 of 3",
        attempt: 1,
        of: 3,
      }),
      event("model.call", 2, superCall(1, 1, ["run_checks"])),
      event("tool.call", 2, checks(1, 1, "auth/token.go:24: undefined: jwt.StandardClaims")),
      event("model.call", 2, superCall(1, 2, [])),
      event("pr.step", 2, {
        step: "repair",
        status: "failed",
        detail: "attempt 1: turn limit after 2 turns, 0 edits in 0 files, 4.1s",
        attempt: 1,
        of: 3,
        stop: "turn limit",
        turns: 2,
        edits: 0,
        files: 0,
        failure: "auth/token.go:24: undefined: jwt.StandardClaims",
      }),
      event("pr.step", 2, { step: "repair", status: "running", attempt: 2, of: 3 }),
      event("model.call", 2, superCall(2, 1, ["apply_edit", "run_checks"])),
      event("tool.call", 2, checks(2, 1, "PASS")),
      event("pr.step", 2, {
        step: "repair",
        status: "done",
        attempt: 2,
        of: 3,
        stop: "checks passed",
        turns: 1,
        edits: 1,
        files: 1,
      }),
      event("pr.step", 2, { step: "detect", status: "running", detail: "" }),
      event("pr.step", 2, { step: "detect", status: "done", detail: "clean" }),
    ]);
    const sections = traceSections(state.cards[2]?.trace ?? []);
    expect(sections.map((s) => s.phase)).toEqual(["plan", "fix"]);
    const fix = sections[1]?.groups ?? [];
    expect(fix.map((g) => g.attempt)).toEqual([1, 2, undefined]);
    expect(fix[0]).toMatchObject({
      status: "failed",
      of: 3,
      stop: "turn limit",
      turns: 2,
      failure: "auth/token.go:24: undefined: jwt.StandardClaims",
      micros: 600,
    });
    expect(fix[1]).toMatchObject({ status: "done", turns: 1, edits: 1, micros: 300 });
    expect(fix[0] && traceRows(fix[0]).map((r) => r.entry.kind)).toEqual([
      "model",
      "tool",
      "model",
    ]);
    const recheck = fix[2] ? traceRows(fix[2]) : [];
    expect(recheck).toHaveLength(1);
    expect(recheck[0]).toMatchObject({ open: false, durationMs: 0 });
    expect(recheck[0]?.entry).toMatchObject({ kind: "step", status: "done", detail: "clean" });
    expect(state.log.map((l) => l.text)).toEqual(
      expect.arrayContaining([
        "repair attempt 1 failed: turn limit after 2 turns, 0 edits in 0 files (auth/token.go:24: undefined: jwt.StandardClaims)",
        "repair attempt 2 passed after 1 turn, 1 edit in 1 file",
        "run_checks: auth/token.go:24: undefined: jwt.StandardClaims",
        "super fix (attempt 2, turn 1), 1.0 s, $0.0003: asks for apply_edit, run_checks",
        "plan: build fails",
      ]),
    );
  });

  it("closes a step that started again without finishing", () => {
    const state = run([
      started(3),
      event("pr.step", 3, { step: "plan", status: "done", detail: "a small conflict" }),
      event("pr.step", 3, { step: "conflict", status: "running", detail: "1 hunks" }),
      event("pr.step", 3, { step: "conflict", status: "running", detail: "asking again" }),
      event("pr.step", 3, { step: "conflict", status: "done", detail: "1 hunks in 1 files" }),
    ]);
    const fix = traceSections(state.cards[3]?.trace ?? []).find((s) => s.phase === "fix");
    const rows = fix?.groups[0] ? traceRows(fix.groups[0]) : [];
    expect(rows.map((r) => [r.entry.text, r.open])).toEqual([
      ["conflict running: 1 hunks", false],
      ["conflict done: 1 hunks in 1 files", false],
    ]);
  });

  it("keeps only the newest entries of a long trace", () => {
    let state = run([started(1)]);
    for (let i = 0; i < MAX_TRACE + 50; i++) {
      state = boardReducer(state, event("tool.call", 1, checks(1, i + 1, "PASS")));
    }
    const trace = state.cards[1]?.trace ?? [];
    expect(trace).toHaveLength(MAX_TRACE);
    expect(trace[0]?.seq).toBe((trace[MAX_TRACE - 1]?.seq ?? 0) - MAX_TRACE + 1);
    expect(state.cards[1]?.toolCalls).toBe(MAX_TRACE + 50);
  });

  it("describes older events without the new fields as before", () => {
    const state = run([
      started(1),
      event("tool.call", 1, { tool: "read_file", ok: true, detail: "read_file" }),
    ]);
    expect(state.log[0]?.text).toBe("tool read_file ok: read_file");
    expect(state.cards[1]?.trace[0]).toMatchObject({ kind: "tool", result: "read_file" });
  });
});

describe("formatting", () => {
  it("formats durations", () => {
    expect(seconds(480)).toBe("480 ms");
    expect(seconds(1500)).toBe("1.5 s");
    expect(seconds(38900)).toBe("39 s");
  });

  it("formats money", () => {
    expect(dollars(0)).toBe("$0");
    expect(dollars(1900)).toBe("$0.0019");
    expect(dollars(23_900)).toBe("$0.0239");
    expect(dollars(123_000)).toBe("$0.123");
  });

  it("names outcomes", () => {
    expect(outcomeLabel("merged")).toBe("Merged");
    expect(outcomeLabel("mergeable")).toBe("Ready to merge");
    expect(outcomeLabel("needs_human")).toBe("Needs a person");
    expect(outcomeLabel("failed")).toBe("Could not finish");
    expect(outcomeLabel(undefined)).toBe("");
  });
});
