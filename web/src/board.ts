import { describePlan, overlapOf, readPlan, type CardOverlap } from "./plan";
import { appendTrace, names, toolTarget, traceEntry, type TraceEntry } from "./trace";
import type {
  Action,
  Check,
  Confidence,
  ConfidenceLevel,
  MergePlan,
  Outcome,
  PullRequest,
  PullView,
  UpkeepEvent,
} from "./types";

export type Phase = "queued" | "working" | "verified" | "done";
export type Connection = "connecting" | "live" | "lost";
export type RunStatus = "idle" | "waiting" | "running" | "finished";

export interface StepEntry {
  name: string;
  status: string;
  detail: string;
  at: number;
}

export interface Card {
  pr: PullRequest;
  phase: Phase;
  blocker?: string;
  behind?: boolean;
  overlap?: CardOverlap;
  step?: StepEntry;
  steps: StepEntry[];
  attempts: number;
  calls: number;
  toolCalls: number;
  micros: number;
  baselineMicros: number;
  checks?: Check[];
  verified?: boolean;
  actions: Action[];
  trace: TraceEntry[];
  outcome?: Outcome;
  reason?: string;
  summary?: string;
  flaky?: string[];
  confidence?: Confidence;
  startedAt?: number;
  finishedAt?: number;
  durationMs?: number;
}

export interface ModelTotals {
  role: string;
  models: string[];
  calls: number;
  promptTokens: number;
  completionTokens: number;
  latencyMs: number;
  micros: number;
  baselineMicros: number;
}

export interface Totals {
  calls: number;
  micros: number;
  baselineMicros: number;
  promptTokens: number;
  completionTokens: number;
}

export interface SearchTotals {
  provider: string;
  calls: number;
  kept: number;
  results: number;
  failed: number;
  skipped: number;
  recorded: boolean;
  lastQuery: string;
  lastProblem: string;
}

export interface RunSummary {
  counts: Record<string, number>;
  micros: number;
  baselineMicros: number;
  calls: number;
  error?: string;
}

export interface LogEntry {
  seq: number;
  kind: string;
  pr: number;
  text: string;
  at: number;
}

export interface BoardState {
  connection: Connection;
  session: string | null;
  version: string | null;
  run: RunStatus;
  position: number;
  cards: Record<number, Card>;
  order: number[];
  models: Record<string, ModelTotals>;
  totals: Totals;
  search: SearchTotals;
  notices: string[];
  closed: boolean;
  gaps: number;
  lastSeq: number;
  log: LogEntry[];
  summary: RunSummary | null;
  plan: MergePlan | null;
}

export const MAX_LOG = 120;
export const ROLE_ORDER = ["ultra", "super", "nano"] as const;

export const initialBoard: BoardState = {
  connection: "connecting",
  session: null,
  version: null,
  run: "idle",
  position: 0,
  cards: {},
  order: [],
  models: {},
  totals: { calls: 0, micros: 0, baselineMicros: 0, promptTokens: 0, completionTokens: 0 },
  search: {
    provider: "",
    calls: 0,
    kept: 0,
    results: 0,
    failed: 0,
    skipped: 0,
    recorded: false,
    lastQuery: "",
    lastProblem: "",
  },
  notices: [],
  closed: false,
  gaps: 0,
  lastSeq: 0,
  log: [],
  summary: null,
  plan: null,
};

export type BoardAction =
  | { type: "hello"; session: string; version: string }
  | { type: "ping" }
  | { type: "lost" }
  | { type: "gap" }
  | { type: "seed"; prs: PullView[]; run: RunStatus; position?: number }
  | { type: "reset" }
  | { type: "event"; event: UpkeepEvent };

function freshCard(pr: PullRequest): Card {
  return {
    pr,
    phase: "queued",
    steps: [],
    attempts: 0,
    calls: 0,
    toolCalls: 0,
    micros: 0,
    baselineMicros: 0,
    actions: [],
    trace: [],
  };
}

function seedFrom(views: PullView[]): { cards: Record<number, Card>; order: number[] } {
  const cards: Record<number, Card> = {};
  const order: number[] = [];
  for (const v of views) {
    const card = freshCard(v);
    if (v.state === "merged" || v.outcome) {
      card.phase = "done";
      card.outcome = v.outcome ?? "merged";
      card.blocker = v.blocker;
      card.actions = v.actions ?? [];
      card.durationMs = v.duration_ms;
      card.confidence = v.confidence;
    }
    cards[v.number] = card;
    order.push(v.number);
  }
  return { cards, order };
}

const LEVELS: ConfidenceLevel[] = ["high", "medium", "low"];

export function confidenceOf(v: unknown): Confidence | undefined {
  const c = record(v);
  const level = str(c.level) as ConfidenceLevel;
  if (!LEVELS.includes(level)) {
    return undefined;
  }
  const reasons = Array.isArray(c.reasons)
    ? c.reasons.filter((r): r is string => typeof r === "string")
    : [];
  return { level, reasons };
}

function num(v: unknown): number {
  return typeof v === "number" && Number.isFinite(v) ? v : 0;
}

function str(v: unknown): string {
  return typeof v === "string" ? v : "";
}

function record(v: unknown): Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v)
    ? (v as Record<string, unknown>)
    : {};
}

function stamp(event: UpkeepEvent): number {
  const t = Date.parse(event.time);
  return Number.isFinite(t) ? t : Date.now();
}

export function seconds(ms: number): string {
  if (ms < 1000) {
    return `${Math.round(ms)} ms`;
  }
  return `${(ms / 1000).toFixed(ms < 10_000 ? 1 : 0)} s`;
}

export function providerLabel(provider: string): string {
  return provider ? `${provider.charAt(0).toUpperCase()}${provider.slice(1)}` : "Web search";
}

export function quantity(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`;
}

function length(v: unknown): number {
  return Array.isArray(v) ? v.length : 0;
}

function describeSearch(d: Record<string, unknown>): string {
  const name = providerLabel(str(d.provider));
  if (str(d.skipped)) {
    return `${name} search skipped: ${str(d.skipped)}`;
  }
  if (str(d.error)) {
    return `${name} search failed: ${str(d.error)}`;
  }
  const kept = length(d.kept);
  const results = length(d.results);
  const found =
    results > 0
      ? `kept ${kept} of ${quantity(results, "page", "pages")}`
      : `kept ${quantity(kept, "page", "pages")}`;
  return `${name} search${d.recorded === true ? " (recorded)" : ""}: ${found} for “${str(d.query)}”`;
}

export function dollars(micros: number): string {
  const usd = micros / 1_000_000;
  if (usd === 0) {
    return "$0";
  }
  return usd < 0.1 ? `$${usd.toFixed(4)}` : `$${usd.toFixed(3)}`;
}

function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? "" : "s"}`;
}

const PURPOSES: Record<string, string> = {
  fix: "fix",
  resolve: "conflict resolution",
  resolve_retry: "conflict resolution, asked again",
  diagnose: "diagnosis",
  explain: "note",
  forecast: "forecast",
};

export function purposeLabel(purpose: string): string {
  return PURPOSES[purpose] ?? purpose;
}

function describeStep(d: Record<string, unknown>): string {
  const name = str(d.step);
  const status = str(d.status);
  const detail = str(d.detail);
  const attempt = num(d.attempt);
  if (name === "plan") {
    return `plan: ${detail}`;
  }
  if (name !== "repair" || attempt === 0) {
    return `${name} ${status}${detail ? `: ${detail}` : ""}`;
  }
  const of = num(d.of);
  if (status === "running") {
    return `repair attempt ${attempt}${of ? ` of ${of}` : ""} started`;
  }
  const work = `${plural(num(d.turns), "turn")}, ${plural(num(d.edits), "edit")} in ${plural(num(d.files), "file")}`;
  if (status === "done") {
    return `repair attempt ${attempt} passed after ${work}`;
  }
  const failure = str(d.failure);
  return `repair attempt ${attempt} failed: ${str(d.stop) || status} after ${work}${failure ? ` (${failure})` : ""}`;
}

function describeModel(d: Record<string, unknown>): string {
  const purpose = str(d.purpose);
  const turn = num(d.turn);
  const attempt = num(d.attempt);
  const tools = names(d.tools);
  const where = turn > 0 ? ` (${attempt > 0 ? `attempt ${attempt}, ` : ""}turn ${turn})` : "";
  const asked = tools.length > 0 ? `: asks for ${tools.join(", ")}` : "";
  return `${str(d.role) || "model"} ${purpose ? purposeLabel(purpose) : "call"}${where}, ${seconds(num(d.latency_ms))}, ${dollars(num(d.micros))}${asked}`;
}

function describeTool(d: Record<string, unknown>): string {
  const tool = str(d.tool);
  const ok = d.ok === true;
  const result = str(d.result);
  const target = toolTarget(d);
  if (!result && !target) {
    return `tool ${tool} ${ok ? "ok" : "failed"}: ${str(d.detail)}`;
  }
  if (!ok) {
    const verb = tool === "apply_edit" ? "refused" : "failed";
    return `${tool} ${verb}${target ? ` on ${target}` : ""}: ${result || str(d.detail)}`;
  }
  switch (tool) {
    case "apply_edit":
      return `Super edited ${target} (+${num(d.added)} −${num(d.removed)})`;
    case "read_file":
      return `Super read ${target}${result ? ` (${result})` : ""}`;
    case "go_doc":
      return `Super looked up ${target}${result ? `: ${result}` : ""}`;
    case "run_checks":
      return `run_checks: ${result}`;
    default:
      return `${tool}${target ? ` ${target}` : ""}${result ? `: ${result}` : ""}`;
  }
}

function describe(event: UpkeepEvent, card: Card | undefined): string {
  const d = record(event.data);
  switch (event.kind) {
    case "run.queued":
      return `waiting for a free slot, position ${num(d.position)}`;
    case "run.started":
      return "run started";
    case "run.plan":
      return describePlan(readPlan(d, null));
    case "pr.blocker":
      return `blocked by ${str(d.blocker) || "nothing"}`;
    case "pr.step":
      return describeStep(d);
    case "model.call":
      return describeModel(d);
    case "search.call":
      return describeSearch(d);
    case "tool.call":
      return describeTool(d);
    case "verify.result":
      return d.passed === true ? "every check passed" : "a check failed";
    case "action":
      return `${str(d.kind)} ${str(d.status)}${str(d.detail) ? `: ${str(d.detail)}` : ""}`;
    case "receipt":
      return `receipt: ${num(d.calls)} calls, ${dollars(num(d.micros))}`;
    case "pr.done":
      return `finished: ${str(d.outcome) || "no outcome"}${card?.reason ? ` (${card.reason})` : ""}`;
    case "notice":
      return str(d.message);
    case "run.finished":
      return d.error ? `run stopped: ${str(d.error)}` : "run finished";
    case "session.closed":
      return "session closed";
    default:
      return event.kind;
  }
}

function updateCard(
  state: BoardState,
  number: number,
  change: (c: Card) => Card,
): Record<number, Card> {
  const current = state.cards[number];
  if (!current) {
    return state.cards;
  }
  return { ...state.cards, [number]: change(current) };
}

function applyEvent(state: BoardState, event: UpkeepEvent): BoardState {
  const d = record(event.data);
  const at = stamp(event);
  const pr = event.pr ?? 0;
  let next: BoardState = { ...state, lastSeq: Math.max(state.lastSeq, event.seq) };

  switch (event.kind) {
    case "run.queued":
      next = { ...next, run: "waiting", position: num(d.position) };
      break;
    case "run.started": {
      const prs = Array.isArray(d.prs) ? (d.prs as PullRequest[]) : [];
      const cards = { ...next.cards };
      const order = [...next.order];
      for (const p of prs) {
        if (!cards[p.number]) {
          cards[p.number] = freshCard(p);
          order.push(p.number);
        }
      }
      order.sort((a, b) => a - b);
      next = { ...next, run: "running", position: 0, cards, order, summary: null };
      break;
    }
    case "run.plan": {
      const plan = readPlan(d, next.plan);
      const cards: Record<number, Card> = {};
      for (const [key, card] of Object.entries(next.cards)) {
        cards[Number(key)] = { ...card, overlap: overlapOf(plan, card.pr.number) };
      }
      next = { ...next, plan, cards };
      break;
    }
    case "pr.blocker":
      next = {
        ...next,
        cards: updateCard(next, pr, (c) => ({
          ...c,
          blocker: str(d.blocker),
          behind: d.behind === true,
        })),
      };
      break;
    case "pr.step": {
      const entry: StepEntry = {
        name: str(d.step),
        status: str(d.status),
        detail: str(d.detail),
        at,
      };
      next = {
        ...next,
        cards: updateCard(next, pr, (c) => ({
          ...c,
          phase: c.phase === "queued" ? "working" : c.phase,
          step: entry,
          steps: [...c.steps, entry].slice(-40),
          startedAt: c.startedAt ?? at,
          attempts:
            entry.name === "repair" && entry.status === "running" ? c.attempts + 1 : c.attempts,
        })),
      };
      break;
    }
    case "model.call": {
      const role = str(d.role) || "super";
      const prev = next.models[role] ?? {
        role,
        models: [],
        calls: 0,
        promptTokens: 0,
        completionTokens: 0,
        latencyMs: 0,
        micros: 0,
        baselineMicros: 0,
      };
      const model = str(d.model);
      const totals: ModelTotals = {
        ...prev,
        models: model && !prev.models.includes(model) ? [...prev.models, model] : prev.models,
        calls: prev.calls + 1,
        promptTokens: prev.promptTokens + num(d.prompt),
        completionTokens: prev.completionTokens + num(d.completion),
        latencyMs: prev.latencyMs + num(d.latency_ms),
        micros: prev.micros + num(d.micros),
        baselineMicros: prev.baselineMicros + num(d.baseline_micros),
      };
      next = {
        ...next,
        models: { ...next.models, [role]: totals },
        totals: {
          calls: next.totals.calls + 1,
          micros: next.totals.micros + num(d.micros),
          baselineMicros: next.totals.baselineMicros + num(d.baseline_micros),
          promptTokens: next.totals.promptTokens + num(d.prompt),
          completionTokens: next.totals.completionTokens + num(d.completion),
        },
        cards: updateCard(next, pr, (c) => ({
          ...c,
          phase: c.phase === "queued" ? "working" : c.phase,
          calls: c.calls + 1,
          micros: c.micros + num(d.micros),
          baselineMicros: c.baselineMicros + num(d.baseline_micros),
        })),
      };
      break;
    }
    case "search.call": {
      const prev = next.search;
      const skipped = str(d.skipped);
      const error = str(d.error);
      next = {
        ...next,
        search: {
          provider: str(d.provider) || prev.provider,
          calls: skipped ? prev.calls : prev.calls + 1,
          kept: prev.kept + length(d.kept),
          results: prev.results + length(d.results),
          failed: prev.failed + (error ? 1 : 0),
          skipped: prev.skipped + (skipped ? 1 : 0),
          recorded: d.recorded === true,
          lastQuery: str(d.query) || prev.lastQuery,
          lastProblem: skipped || error,
        },
      };
      break;
    }
    case "tool.call":
      next = {
        ...next,
        cards: updateCard(next, pr, (c) => ({ ...c, toolCalls: c.toolCalls + 1 })),
      };
      break;
    case "verify.result": {
      const checks = Array.isArray(d.checks) ? (d.checks as Check[]) : undefined;
      const passed = d.passed === true;
      next = {
        ...next,
        cards: updateCard(next, pr, (c) => ({
          ...c,
          checks,
          verified: passed,
          phase: passed && c.phase !== "done" ? "verified" : c.phase,
        })),
      };
      break;
    }
    case "action": {
      const action: Action = { kind: str(d.kind), status: str(d.status), detail: str(d.detail) };
      next = {
        ...next,
        cards: updateCard(next, pr, (c) => ({ ...c, actions: [...c.actions, action] })),
      };
      break;
    }
    case "receipt":
      next = {
        ...next,
        cards: updateCard(next, pr, (c) => ({
          ...c,
          attempts: Math.max(c.attempts, num(d.attempts)),
          micros: Math.max(c.micros, num(d.micros)),
          baselineMicros: Math.max(c.baselineMicros, num(d.baseline_micros)),
        })),
      };
      break;
    case "pr.done":
      next = {
        ...next,
        cards: updateCard(next, pr, (c) => ({
          ...c,
          phase: "done",
          outcome: str(d.outcome) as Outcome,
          blocker: str(d.blocker) || c.blocker,
          reason: str(d.reason),
          summary: str(d.summary),
          flaky: Array.isArray(d.flaky) ? (d.flaky as string[]) : undefined,
          actions:
            Array.isArray(d.actions) && c.actions.length === 0
              ? (d.actions as Action[])
              : c.actions,
          attempts: Math.max(c.attempts, num(d.attempts)),
          durationMs: num(d.duration_ms),
          confidence: confidenceOf(d.confidence),
          finishedAt: at,
        })),
      };
      break;
    case "notice": {
      const message = str(d.message);
      if (message && !next.notices.includes(message)) {
        next = { ...next, notices: [...next.notices, message] };
      }
      break;
    }
    case "run.finished":
      next = {
        ...next,
        run: "finished",
        position: 0,
        summary: {
          counts: record(d.counts) as Record<string, number>,
          micros: num(d.micros),
          baselineMicros: num(d.baseline_micros),
          calls: num(d.calls),
          error: typeof d.error === "string" ? d.error : undefined,
        },
      };
      break;
    case "session.closed":
      next = { ...next, closed: true };
      break;
  }

  const text = describe(event, next.cards[pr]);
  const entry: LogEntry = { seq: event.seq, kind: event.kind, pr, text, at };
  const card = next.cards[pr];
  const traced = card ? traceEntry(event, at, card.trace, text) : null;
  if (card && traced) {
    next = {
      ...next,
      cards: { ...next.cards, [pr]: { ...card, trace: appendTrace(card.trace, traced) } },
    };
  }
  return { ...next, log: [entry, ...next.log].slice(0, MAX_LOG) };
}

export function boardReducer(state: BoardState, action: BoardAction): BoardState {
  switch (action.type) {
    case "hello":
      return { ...state, connection: "live", session: action.session, version: action.version };
    case "ping":
      return state.connection === "live" ? state : { ...state, connection: "live" };
    case "lost":
      return { ...state, connection: "lost" };
    case "gap":
      return { ...state, gaps: state.gaps + 1 };
    case "reset":
      return { ...initialBoard };
    case "seed": {
      if (state.run === "running" || state.run === "finished") {
        return state;
      }
      const { cards, order } = seedFrom(action.prs);
      return { ...state, cards, order, run: action.run, position: action.position ?? 0 };
    }
    case "event":
      if (action.event.seq <= state.lastSeq && action.event.seq !== 0) {
        return state;
      }
      return applyEvent(state, action.event);
  }
}

export interface Columns {
  queued: Card[];
  working: Card[];
  verified: Card[];
  done: Card[];
}

export function columns(state: BoardState): Columns {
  const out: Columns = { queued: [], working: [], verified: [], done: [] };
  for (const n of state.order) {
    const card = state.cards[n];
    if (card) {
      out[card.phase].push(card);
    }
  }
  return out;
}

export function outcomeLabel(outcome: Outcome | undefined): string {
  switch (outcome) {
    case "merged":
      return "Merged";
    case "mergeable":
      return "Ready to merge";
    case "needs_human":
      return "Needs a person";
    case "failed":
      return "Could not finish";
    default:
      return "";
  }
}
