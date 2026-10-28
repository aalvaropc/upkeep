import type { MergePlan, PlanOverlap, PlanStale } from "./types";

export interface CardOverlap {
  conflictsWith: number[];
  sharesWith: number[];
  stale?: string;
}

function numbers(v: unknown): number[] {
  return Array.isArray(v)
    ? v.filter((n): n is number => typeof n === "number" && Number.isFinite(n))
    : [];
}

function strings(v: unknown): string[] {
  return Array.isArray(v) ? v.filter((s): s is string => typeof s === "string") : [];
}

function entries(v: unknown): Record<string, unknown>[] {
  return Array.isArray(v)
    ? v.filter((e): e is Record<string, unknown> => typeof e === "object" && e !== null)
    : [];
}

function overlaps(v: unknown): PlanOverlap[] {
  return entries(v)
    .filter((o) => typeof o.a === "number" && typeof o.b === "number")
    .map((o) => ({
      a: o.a as number,
      b: o.b as number,
      files: strings(o.files),
      conflict: o.conflict === true,
    }));
}

function stale(v: unknown): PlanStale[] {
  return entries(v)
    .filter((s) => typeof s.pr === "number")
    .map((s) => ({ pr: s.pr as number, because: typeof s.because === "string" ? s.because : "" }));
}

function reasons(v: unknown): Record<string, string> {
  const out: Record<string, string> = {};
  if (typeof v === "object" && v !== null && !Array.isArray(v)) {
    for (const [k, r] of Object.entries(v)) {
      if (typeof r === "string") {
        out[k] = r;
      }
    }
  }
  return out;
}

export function readPlan(d: Record<string, unknown>, previous: MergePlan | null): MergePlan {
  if (!Array.isArray(d.order) && previous) {
    return { ...previous, stale: stale(d.stale), checked: d.checked !== false };
  }
  return {
    order: numbers(d.order),
    overlaps: overlaps(d.overlaps),
    reasons: reasons(d.reasons),
    stale: stale(d.stale),
    checked: d.checked === true,
  };
}

export function describePlan(plan: MergePlan): string {
  if (plan.checked) {
    const n = plan.stale.length;
    if (n === 0) {
      return "checked again after the merges: every ready pull request still merges cleanly";
    }
    return `checked again after the merges: ${n} ready ${n === 1 ? "pull request no longer merges" : "pull requests no longer merge"} cleanly`;
  }
  const n = plan.overlaps.filter((o) => o.conflict).length;
  return `merge plan for ${plan.order.length} pull requests, ${n} conflicting ${n === 1 ? "pair" : "pairs"}`;
}

export function overlapOf(plan: MergePlan, number: number): CardOverlap | undefined {
  const conflictsWith: number[] = [];
  const sharesWith: number[] = [];
  for (const o of plan.overlaps) {
    if (o.a !== number && o.b !== number) {
      continue;
    }
    const other = o.a === number ? o.b : o.a;
    (o.conflict ? conflictsWith : sharesWith).push(other);
  }
  const went = plan.stale.find((s) => s.pr === number);
  if (conflictsWith.length === 0 && sharesWith.length === 0 && !went) {
    return undefined;
  }
  return { conflictsWith, sharesWith, stale: went?.because };
}

function list(prs: number[]): string {
  return prs.map((n) => `#${n}`).join(", ");
}

export function overlapLabel(o: CardOverlap): string {
  if (o.stale) {
    return "conflicts with main";
  }
  if (o.conflictsWith.length > 0) {
    return `conflicts with ${list(o.conflictsWith)}`;
  }
  return `shares files with ${list(o.sharesWith)}`;
}

export function overlapTone(o: CardOverlap): "conflict" | "shared" {
  return o.stale || o.conflictsWith.length > 0 ? "conflict" : "shared";
}
