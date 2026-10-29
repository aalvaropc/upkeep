import type { BoardState } from "../board";
import styles from "./FlowSteps.module.css";

interface Props {
  board: BoardState;
}

const STEPS = [
  { id: "plan", label: "Plan the queue", hint: "collisions and order" },
  { id: "work", label: "Work each pull request", hint: "forecast, fix, verify, act" },
  { id: "results", label: "Results", hint: "merged, ready, needs a person" },
] as const;

function share(board: BoardState): number {
  const total = board.order.length;
  if (total === 0) {
    return 0;
  }
  const done = board.order.filter((n) => board.cards[n]?.phase === "done").length;
  return Math.round((done / total) * 100);
}

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
  const progress = share(board);
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
            <span className={styles.bar}>
              <svg
                className={styles.fill}
                viewBox="0 0 100 1"
                preserveAspectRatio="none"
                aria-hidden="true"
                focusable="false"
              >
                <rect
                  x="0"
                  y="0"
                  height="1"
                  width={
                    state === "done" ? 100 : state === "next" ? 0 : s.id === "work" ? progress : 100
                  }
                />
              </svg>
            </span>
            <span className={styles.text}>
              <span className={styles.label}>{s.label}</span>
              <span className={styles.hint}>{s.hint}</span>
            </span>
          </li>
        );
      })}
    </ol>
  );
}
