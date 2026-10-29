export type AuthorKind = "person" | "bot";

export type Outcome = "merged" | "mergeable" | "needs_human" | "failed" | "";

export interface Bump {
  module: string;
  from: string;
  to: string;
  level: "patch" | "minor" | "major";
}

export interface PullRequest {
  number: number;
  title: string;
  author: { login: string; kind: AuthorKind };
  labels?: string[];
  base?: string;
  branch: string;
  kind: "change" | "dependency";
  bump?: Bump;
}

export interface Action {
  kind: string;
  status: string;
  detail?: string;
}

export interface Comment {
  kind: string;
  body: string;
  at: string;
}

export type ConfidenceLevel = "high" | "medium" | "low";

export interface Confidence {
  level: ConfidenceLevel;
  reasons: string[];
}

export interface Learned {
  from: number;
  saved_calls: number;
  saved_micros: number;
}

export interface PullView extends PullRequest {
  state: "open" | "merged";
  merge_sha?: string;
  comments?: Comment[];
  outcome?: Outcome;
  blocker?: string;
  actions?: Action[];
  duration_ms?: number;
  confidence?: Confidence;
  learned?: Learned;
}

export interface Snapshot {
  id: string;
  status: "idle" | "waiting" | "running" | "finished";
  position?: number;
  run_id?: string;
  created: string;
  prs: PullView[];
}

export interface Check {
  id: string;
  label: string;
  pass: boolean;
  detail?: string;
}

export interface Change {
  path: string;
  added: number;
  deleted: number;
}

export interface Breaking {
  symbol: string;
  description: string;
}

export interface SearchResult {
  title: string;
  url: string;
  score: number;
  kept: boolean;
}

export interface SearchTrace {
  provider: string;
  query: string;
  results?: SearchResult[];
  kept?: string[];
  latency_ms?: number;
  micros?: number;
  error?: string;
  at?: string;
  recorded?: boolean;
  reused?: boolean;
  new?: string[];
  skipped?: string;
}

export interface Forecast {
  module: string;
  from: string;
  to: string;
  summary: string;
  breaking: Breaking[];
  dropped: number;
  excerpt: string;
  excerpt_source: string;
  files: string[];
  sources: string[];
  fetched_at?: string;
  search?: SearchTrace;
  releases?: string[];
  releases_error?: string;
}

export interface ReceiptLine {
  model: string;
  role: string;
  calls: number;
  prompt_tokens: number;
  completion_tokens: number;
  reasoning_tokens: number;
  latency_ms: number;
  micros: number;
  baseline_micros: number;
}

export interface ReceiptStep {
  name: string;
  detail?: string;
  duration_ms: number;
  ok: boolean;
}

export interface ReceiptSearch {
  provider: string;
  query: string;
  results: number;
  kept: number;
  latency_ms: number;
  micros: number;
  error?: string;
  recorded: boolean;
  reused: boolean;
}

export interface Receipt {
  lines: ReceiptLine[] | null;
  steps: ReceiptStep[] | null;
  searches?: ReceiptSearch[] | null;
  calls: number;
  attempts: number;
  micros: number;
  baseline_micros: number;
}

export interface ResolvedBlock {
  path: string;
  text: string;
}

export interface Detail extends PullView {
  reason?: string;
  summary?: string;
  failure?: string;
  diagnosis?: string;
  attempts: number;
  patch?: string;
  merged_base?: boolean;
  resolution?: ResolvedBlock[];
  changes?: Change[];
  checklist?: Check[];
  flaky?: string[];
  forecast?: Forecast;
  confirmed?: string[];
  missed?: string[];
  receipt?: Receipt;
}

export interface Info {
  mode: "live" | "record" | "replay";
  version: string;
  sessions: number;
  prs_per_run: number;
  runs_per_server: number;
  daily_tokens: number;
  tokens_today: number;
  search_daily_limit?: number;
  searches_today?: number;
  rules?: string[];
}

export type EventKind =
  | "run.queued"
  | "run.started"
  | "run.plan"
  | "pr.blocker"
  | "pr.step"
  | "model.call"
  | "search.call"
  | "tool.call"
  | "verify.result"
  | "action"
  | "receipt"
  | "pr.done"
  | "notice"
  | "run.finished"
  | "session.closed";

export interface PlanOverlap {
  a: number;
  b: number;
  files: string[];
  conflict: boolean;
}

export interface PlanStale {
  pr: number;
  because: string;
}

export interface MergePlan {
  order: number[];
  overlaps: PlanOverlap[];
  reasons: Record<string, string>;
  stale: PlanStale[];
  checked: boolean;
}

export interface UpkeepEvent {
  seq: number;
  kind: EventKind;
  time: string;
  run?: string;
  pr?: number;
  data?: unknown;
}

export interface StepEvent {
  step: string;
  status: string;
  detail?: string;
  attempt?: number;
  of?: number;
  stop?: string;
  turns?: number;
  edits?: number;
  files?: number;
  failure?: string;
}

export interface ModelCallEvent {
  model: string;
  role: string;
  latency_ms: number;
  prompt: number;
  completion: number;
  reasoning?: number;
  micros: number;
  baseline_micros: number;
  step?: string;
  purpose?: string;
  attempt?: number;
  turn?: number;
  said?: string;
  tools?: string[];
}

export interface ToolCallEvent {
  tool: string;
  ok: boolean;
  detail: string;
  attempt?: number;
  turn?: number;
  path?: string;
  package?: string;
  symbol?: string;
  added?: number;
  removed?: number;
  result?: string;
}
