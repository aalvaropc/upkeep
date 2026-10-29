import type { IconName } from "./components/Icon";

const BLOCKERS: Record<string, string> = {
  behind: "behind main",
  conflict: "conflict",
  conflict_large: "large conflict",
  build: "build fails",
  test: "tests fail",
  flaky: "flaky test",
  format: "formatting",
  clean: "nothing blocks it",
  unsupported: "not a Go module",
};

export const ACTIONS: Record<string, { icon: IconName; label: string }> = {
  push: { icon: "push", label: "pushed" },
  patch: { icon: "patch", label: "patch attached" },
  merge: { icon: "merge", label: "merged" },
  comment: { icon: "patch", label: "diagnosis posted" },
};

export function blockerLabel(blocker: string | undefined): string {
  return blocker ? (BLOCKERS[blocker] ?? blocker) : "";
}

const STEPS: Record<string, string> = {
  detect: "Checking",
  plan: "Plan",
  forecast: "Reading the release notes",
  conflict: "Resolving the conflict",
  merge: "Merging main",
  format: "Formatting",
  repair: "Fixing",
  verify: "Verifying",
  explain: "Writing the note",
  diagnose: "Diagnosing",
  learned: "Reusing a learned fix",
  learn: "Remembering the fix",
};

export function stepLabel(name: string, status: string, detail: string): string {
  const base = STEPS[name] ?? name;
  const tail =
    status === "running"
      ? "…"
      : status === "done"
        ? " · done"
        : status === "failed"
          ? " · failed"
          : "";
  return `${base}${tail}${detail ? ` · ${detail}` : ""}`;
}
