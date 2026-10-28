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
import { overlapLabel, overlapTone } from "./plan";
import type { EventKind, PullRequest, PullView } from "./types";

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

  it("counts gaps and resets", () => {
    const state = run([{ type: "gap" }, { type: "gap" }]);
    expect(state.gaps).toBe(2);
    expect(run([{ type: "reset" }], run([started(1)]))).toEqual(initialBoard);
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

describe("merge plan", () => {
  const planned = {
    order: [6, 7, 1, 5],
    overlaps: [
      { a: 5, b: 6, files: ["go.mod", "go.sum"], conflict: false },
      { a: 5, b: 7, files: ["go.mod", "go.sum"], conflict: true },
      { a: 6, b: 7, files: ["go.mod", "go.sum"], conflict: false },
    ],
    reasons: {
      "5": "Conflicts with #7 in go.mod, go.sum: merge it after #7 and update its branch.",
    },
    stale: [],
    checked: false,
  };

  it("keeps the plan and marks the cards that share files", () => {
    const state = run([started(1, 5, 6, 7), event("run.plan", undefined, planned)]);
    expect(state.plan?.order).toEqual([6, 7, 1, 5]);
    expect(state.plan?.reasons["5"]).toContain("merge it after #7");
    expect(state.cards[5]?.overlap).toEqual({ conflictsWith: [7], sharesWith: [6] });
    expect(overlapLabel(state.cards[5]!.overlap!)).toBe("conflicts with #7");
    expect(overlapLabel(state.cards[6]!.overlap!)).toBe("shares files with #5, #7");
    expect(overlapTone(state.cards[6]!.overlap!)).toBe("shared");
    expect(state.cards[1]?.overlap).toBeUndefined();
    expect(state.log[0]?.text).toBe("merge plan for 4 pull requests, 1 conflicting pair");
  });

  it("merges the check after the run into the plan", () => {
    const first = run([started(1, 5, 6, 7), event("run.plan", undefined, planned)]);
    const checked = run(
      [
        event("run.plan", undefined, {
          ...planned,
          stale: [{ pr: 5, because: "conflicts with main after #7 merged: go.mod, go.sum" }],
          checked: true,
        }),
      ],
      first,
    );
    expect(checked.plan?.checked).toBe(true);
    expect(checked.plan?.stale).toEqual([
      { pr: 5, because: "conflicts with main after #7 merged: go.mod, go.sum" },
    ]);
    expect(checked.cards[5]?.overlap?.stale).toBe(
      "conflicts with main after #7 merged: go.mod, go.sum",
    );
    expect(overlapLabel(checked.cards[5]!.overlap!)).toBe("conflicts with main");
    expect(overlapTone(checked.cards[5]!.overlap!)).toBe("conflict");
    expect(checked.log[0]?.text).toBe(
      "checked again after the merges: 1 ready pull request no longer merges cleanly",
    );

    const staleOnly = run(
      [event("run.plan", undefined, { stale: [{ pr: 1, because: "conflicts with main" }] })],
      first,
    );
    expect(staleOnly.plan?.order).toEqual([6, 7, 1, 5]);
    expect(staleOnly.plan?.checked).toBe(true);
    expect(staleOnly.cards[1]?.overlap).toEqual({
      conflictsWith: [],
      sharesWith: [],
      stale: "conflicts with main",
    });
  });

  it("reads a malformed plan as an empty one", () => {
    const state = run([
      started(1),
      event("run.plan", undefined, { order: [1, "x"], overlaps: [null, { a: "1" }], stale: "no" }),
    ]);
    expect(state.plan).toEqual({
      order: [1],
      overlaps: [],
      reasons: {},
      stale: [],
      checked: false,
    });
    expect(state.cards[1]?.overlap).toBeUndefined();
  });
});
