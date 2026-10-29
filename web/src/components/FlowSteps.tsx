import type { BoardState } from "../board";
import styles from "./FlowSteps.module.css";

interface Props {
  board: BoardState;
}

const STEPS = [
  { id: "plan", label: "Plan the queue", hint: "which pull requests collide, in what order" },
  { id: "work", label: "Work each pull request", hint: "forecast, fix, verify, act" },
  { id: "results", label: "Results", hint: "what merged, what is ready, what needs a person" },
] as const;

function current(board: BoardState): number {
  switch (board.run) {
    case "idle":
    case "waiting":
      return -1;
    case "running":
      return board.plan ? 1 : 0;
    case "finished":
      return 2;
  }
}

export function FlowSteps({ board }: Props) {
  const at = current(board);
  return (
    <ol className={styles.flow} aria-label="Run progress">
      {STEPS.map((s, i) => {
        const state = i < at ? "done" : i === at ? "current" : "next";
        return (
          <li
            key={s.id}
            className={styles.step}
            data-state={state}
            aria-current={state === "current" ? "step" : undefined}
          >
            <span className={styles.bar} />
            <span className={styles.label}>{s.label}</span>
            <span className={styles.hint}>{s.hint}</span>
          </li>
        );
      })}
    </ol>
  );
}
