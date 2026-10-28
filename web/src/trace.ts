import type { Action, Check, UpkeepEvent } from "./types";

export const MAX_TRACE = 400;

export type TracePhase = "plan" | "forecast" | "fix" | "verify" | "act";

export const TRACE_PHASES: readonly TracePhase[] = ["plan", "forecast", "fix", "verify", "act"];

interface TraceBase {
  seq: number;
  at: number;
  phase: TracePhase;
  attempt?: number;
  text: string;
}

export interface StepTrace extends TraceBase {
  kind: "step";
  name: string;
  status: string;
  detail: string;
  of?: number;
  stop?: string;
  turns?: number;
  edits?: number;
  files?: number;
  failure?: string;
}

export interface ModelTrace extends TraceBase {
  kind: "model";
  role: string;
  model: string;
  step: string;
  purpose: string;
  turn?: number;
  said: string;
  tools: string[];
  latencyMs: number;
  tokens: number;
  micros: number;
}

export interface ToolTrace extends TraceBase {
  kind: "tool";
  tool: string;
  ok: boolean;
  turn?: number;
  target: string;
  added?: number;
  removed?: number;
  result: string;
}

export interface SearchCallTrace extends TraceBase {
  kind: "search";
  provider: string;
  query: string;
  kept: number;
  results: number;
  latencyMs: number;
  micros: number;
  recorded: boolean;
  reused: boolean;
  error?: string;
  skipped?: string;
}

export interface VerifyTrace extends TraceBase {
  kind: "verify";
  passed: boolean;
  checks: Check[];
}

export interface ActionTrace extends TraceBase {
  kind: "action";
  action: Action;
}

export type TraceEntry =
  StepTrace | ModelTrace | SearchCallTrace | ToolTrace | VerifyTrace | ActionTrace;

export interface TraceGroup {
  attempt?: number;
  of?: number;
  status: string;
  stop?: string;
  failure?: string;
  turns?: number;
  edits?: number;
  files?: number;
  startedAt: number;
  endedAt?: number;
  micros: number;
  entries: TraceEntry[];
}

export interface TraceSection {
  phase: TracePhase;
  groups: TraceGroup[];
}

export interface TraceRow {
  entry: TraceEntry;
  durationMs?: number;
  open: boolean;
}

function record(v: unknown): Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v)
    ? (v as Record<string, unknown>)
    : {};
}

function str(v: unknown): string {
  return typeof v === "string" ? v : "";
}

function num(v: unknown): number {
  return typeof v === "number" && Number.isFinite(v) ? v : 0;
}

function count(v: unknown): number | undefined {
  return typeof v === "number" && Number.isFinite(v) ? v : undefined;
}

function size(v: unknown): number {
  return Array.isArray(v) ? v.length : 0;
}

function word(v: unknown): string | undefined {
  return typeof v === "string" && v !== "" ? v : undefined;
}

export function names(v: unknown): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : [];
}

export function toolTarget(d: Record<string, unknown>): string {
  const path = str(d.path);
  if (path) {
    return path;
  }
  const pkg = str(d.package);
  const symbol = str(d.symbol);
  return pkg && symbol ? `${pkg} ${symbol}` : pkg;
}

const STEP_PHASES: Record<string, TracePhase> = {
  plan: "plan",
  detect: "plan",
  forecast: "forecast",
  merge: "fix",
  format: "fix",
  conflict: "fix",
  repair: "fix",
  verify: "verify",
  explain: "act",
  diagnose: "act",
  run: "act",
};

const MODEL_PHASES: Record<string, TracePhase> = {
  forecast: "forecast",
  conflict: "fix",
  repair: "fix",
  explain: "act",
  diagnose: "act",
};

function stepPhase(name: string, trace: TraceEntry[]): TracePhase {
  if (name === "detect" && trace.some((e) => e.kind === "step" && e.name === "plan")) {
    return "fix";
  }
  return STEP_PHASES[name] ?? "fix";
}

export function traceEntry(
  event: UpkeepEvent,
  at: number,
  trace: TraceEntry[],
  text: string,
): TraceEntry | null {
  const d = record(event.data);
  const base = { seq: event.seq, at, text, attempt: count(d.attempt) };
  switch (event.kind) {
    case "pr.step": {
      const name = str(d.step);
      return {
        ...base,
        kind: "step",
        phase: stepPhase(name, trace),
        name,
        status: str(d.status),
        detail: str(d.detail),
        of: count(d.of),
        stop: word(d.stop),
        turns: count(d.turns),
        edits: count(d.edits),
        files: count(d.files),
        failure: word(d.failure),
      };
    }
    case "model.call": {
      const step = str(d.step);
      const role = str(d.role) || "super";
      return {
        ...base,
        kind: "model",
        phase: MODEL_PHASES[step] ?? (role === "nano" ? "act" : "fix"),
        role,
        model: str(d.model),
        step,
        purpose: str(d.purpose),
        turn: count(d.turn),
        said: str(d.said),
        tools: names(d.tools),
        latencyMs: num(d.latency_ms),
        tokens: num(d.prompt) + num(d.completion),
        micros: num(d.micros),
      };
    }
    case "search.call":
      return {
        ...base,
        kind: "search",
        phase: "forecast",
        provider: str(d.provider),
        query: str(d.query),
        kept: size(d.kept),
        results: size(d.results),
        latencyMs: num(d.latency_ms),
        micros: num(d.micros),
        recorded: d.recorded === true,
        reused: d.reused === true,
        error: word(d.error),
        skipped: word(d.skipped),
      };
    case "tool.call":
      return {
        ...base,
        kind: "tool",
        phase: "fix",
        tool: str(d.tool),
        ok: d.ok === true,
        turn: count(d.turn),
        target: toolTarget(d),
        added: count(d.added),
        removed: count(d.removed),
        result: str(d.result) || str(d.detail),
      };
    case "verify.result":
      return {
        ...base,
        kind: "verify",
        phase: "verify",
        passed: d.passed === true,
        checks: Array.isArray(d.checks) ? (d.checks as Check[]) : [],
      };
    case "action":
      return {
        ...base,
        kind: "action",
        phase: "act",
        action: { kind: str(d.kind), status: str(d.status), detail: str(d.detail) },
      };
    default:
      return null;
  }
}

export function appendTrace(trace: TraceEntry[], entry: TraceEntry | null): TraceEntry[] {
  return entry ? [...trace, entry].slice(-MAX_TRACE) : trace;
}

export function traceSections(trace: TraceEntry[]): TraceSection[] {
  const sections = new Map<TracePhase, TraceGroup[]>();
  for (const entry of trace) {
    const groups = sections.get(entry.phase) ?? [];
    const attempt = entry.phase === "fix" ? entry.attempt : undefined;
    let group = groups[groups.length - 1];
    if (!group || group.attempt !== attempt) {
      group = {
        attempt,
        status: attempt ? "running" : "",
        startedAt: entry.at,
        micros: 0,
        entries: [],
      };
      groups.push(group);
    }
    group.entries.push(entry);
    if (entry.kind === "model") {
      group.micros += entry.micros;
    }
    if (entry.kind === "step" && entry.name === "repair" && attempt) {
      group.of = entry.of ?? group.of;
      if (entry.status !== "running") {
        group.status = entry.status;
        group.stop = entry.stop;
        group.failure = entry.failure;
        group.turns = entry.turns;
        group.edits = entry.edits;
        group.files = entry.files;
        group.endedAt = entry.at;
      }
    }
    sections.set(entry.phase, groups);
  }
  return TRACE_PHASES.filter((p) => sections.has(p)).map((phase) => ({
    phase,
    groups: sections.get(phase) ?? [],
  }));
}

export function traceRows(group: TraceGroup): TraceRow[] {
  const rows: TraceRow[] = [];
  const pending = new Map<string, number>();
  for (const entry of group.entries) {
    if (entry.kind === "step") {
      if (group.attempt && entry.name === "repair") {
        continue;
      }
      const at = pending.get(entry.name);
      const started = at === undefined ? undefined : rows[at];
      if (entry.status === "running") {
        if (at !== undefined && started) {
          rows[at] = { ...started, open: false };
        }
        pending.set(entry.name, rows.length);
        rows.push({ entry, open: true });
        continue;
      }
      if (at !== undefined && started) {
        rows[at] = { entry, durationMs: entry.at - started.entry.at, open: false };
        pending.delete(entry.name);
        continue;
      }
    }
    rows.push({ entry, open: false });
  }
  return rows;
}
