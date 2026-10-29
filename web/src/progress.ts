import type { Card } from "./board";

export type StageId = "check" | "forecast" | "fix" | "verify" | "act";
export type StageState = "pending" | "running" | "done" | "failed" | "skipped" | "handed";

export interface Stage {
  id: StageId;
  label: string;
  state: StageState;
}

export const STAGES: { id: StageId; label: string }[] = [
  { id: "check", label: "Check" },
  { id: "forecast", label: "Forecast" },
  { id: "fix", label: "Fix" },
  { id: "verify", label: "Verify" },
  { id: "act", label: "Act" },
];

const STAGE_OF: Record<string, StageId> = {
  detect: "check",
  plan: "check",
  forecast: "forecast",
  conflict: "fix",
  merge: "fix",
  format: "fix",
  repair: "fix",
  learned: "fix",
  diagnose: "fix",
  verify: "verify",
  explain: "act",
  learn: "act",
};

const ORDER: StageId[] = STAGES.map((s) => s.id);

export function stages(card: Card): Stage[] {
  const last: Partial<Record<StageId, string>> = {};
  for (const step of card.steps) {
    const stage = STAGE_OF[step.name];
    if (stage) {
      last[stage] = step.status;
    }
  }
  const reached = ORDER.reduce((max, id, i) => (last[id] ? i : max), -1);
  const done = card.phase === "done";
  const handed = card.outcome === "needs_human" || card.outcome === "failed";
  return STAGES.map(({ id, label }, i) => {
    let state: StageState = "pending";
    const status = last[id];
    if (status === "running") {
      state = done ? "done" : "running";
    } else if (status === "failed") {
      state = "failed";
    } else if (status) {
      state = "done";
    } else if (i < reached || done) {
      state = "skipped";
    }
    if (id === "fix" && card.steps.some((s) => s.name === "diagnose")) {
      state = "handed";
    }
    if (id === "act" && done) {
      state = handed
        ? "handed"
        : card.actions.some((a) => a.status === "done")
          ? "done"
          : "skipped";
    }
    return { id, label, state };
  });
}

const WORDS: Record<StageState, string> = {
  pending: "not started",
  running: "running",
  done: "done",
  failed: "failed",
  skipped: "not needed",
  handed: "handed to a person",
};

export function stagesText(list: Stage[]): string {
  return list.map((s) => `${s.label} ${WORDS[s.state]}`).join(", ");
}
