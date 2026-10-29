import { describe, expect, it } from "vitest";
import { boardReducer, initialBoard, type BoardAction, type BoardState } from "./board";
import { nextFocus, workingCards } from "./focus";
import type { EventKind, PullRequest } from "./types";

const pr = (number: number): PullRequest => ({
  number,
  title: `Pull request ${number}`,
  author: { login: "alice", kind: "person" },
  branch: `feat/${number}`,
  kind: "change",
});

let seq = 0;
function at(second: number): string {
  return `2026-10-25T12:00:${String(second).padStart(2, "0")}Z`;
}

function event(
  kind: EventKind,
  prNumber: number | undefined,
  data?: unknown,
  second = 0,
): BoardAction {
  seq += 1;
  return { type: "event", event: { seq, kind, time: at(second), pr: prNumber, data } };
}

function run(actions: BoardAction[], from: BoardState = initialBoard): BoardState {
  return actions.reduce(boardReducer, from);
}

const step = (n: number, second: number, name = "detect") =>
  event("pr.step", n, { step: name, status: "running", detail: "" }, second);

const started = (...numbers: number[]) => event("run.started", undefined, { prs: numbers.map(pr) });

describe("nextFocus", () => {
  it("picks the pull request that started first", () => {
    const board = run([started(1, 2, 3), step(2, 1), step(1, 2), step(3, 2)]);
    expect(nextFocus(board, null)).toBe(2);
  });

  it("stays on the current pull request while it works", () => {
    const board = run([
      started(1, 2),
      step(1, 1),
      step(2, 2),
      step(2, 3, "forecast"),
      step(2, 4, "fix"),
    ]);
    expect(nextFocus(board, 2)).toBe(2);
    expect(nextFocus(board, 1)).toBe(1);
  });

  it("moves on when the current one is done", () => {
    const board = run([
      started(1, 2),
      step(1, 1),
      step(2, 2),
      event("pr.done", 1, { outcome: "merged" }, 3),
    ]);
    expect(nextFocus(board, 1)).toBe(2);
  });

  it("keeps the last one when nothing works", () => {
    const board = run([started(1, 2), step(1, 1), event("pr.done", 1, { outcome: "merged" }, 2)]);
    expect(workingCards(board)).toHaveLength(0);
    expect(nextFocus(board, 1)).toBe(1);
    expect(nextFocus(board, null)).toBeNull();
  });
});
