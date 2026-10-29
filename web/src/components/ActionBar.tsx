import { columns, dollars, outcomeLabel, type BoardState } from "../board";
import { Icon } from "./Icon";
import styles from "./ActionBar.module.css";

interface Props {
  board: BoardState;
  busy: boolean;
  error: string | null;
  now: number;
  onStart: () => void;
  onRules: () => void;
}

function elapsed(board: BoardState, now: number): string {
  const starts = Object.values(board.cards).map((c) => c.startedAt ?? now);
  if (starts.length === 0) {
    return "";
  }
  const seconds = Math.max(0, Math.round((now - Math.min(...starts)) / 1000));
  return `${seconds} s`;
}

export function statusLine(board: BoardState, now: number): string {
  const total = board.order.length;
  const cols = columns(board);
  switch (board.run) {
    case "idle":
      return total === 0
        ? "Getting the demo repository ready."
        : `${total} pull requests are blocked. One click and Upkeep works through them.`;
    case "waiting":
      return `You are number ${board.position} in line. Your run starts as soon as a slot opens.`;
    case "running":
      return `${cols.done.length} of ${total} finished · ${elapsed(board, now)} · ${dollars(board.totals.micros)} so far`;
    case "finished": {
      if (board.summary?.error) {
        return `The run stopped: ${board.summary.error}`;
      }
      const parts = (["merged", "mergeable", "needs_human", "failed"] as const)
        .map((o) => ({ o, n: cols.done.filter((c) => c.outcome === o).length }))
        .filter((p) => p.n > 0)
        .map((p) => `${p.n} ${outcomeLabel(p.o).toLowerCase()}`);
      return `Done: ${parts.join(" · ")}.`;
    }
  }
}

function finding(board: BoardState): string {
  const stale = board.plan?.stale ?? [];
  if (board.run !== "finished" || stale.length === 0) {
    return "";
  }
  return stale.map((s) => `#${s.pr} needs one more pass: ${s.because}.`).join(" ");
}

export function ActionBar({ board, busy, error, now, onStart, onRules }: Props) {
  const waiting = board.run === "waiting";
  const running = board.run === "running";
  const finished = board.run === "finished";
  const disabled = busy || waiting || running || (board.order.length === 0 && !finished);
  const label = waiting
    ? `Waiting for a slot`
    : running
      ? "Working"
      : finished
        ? "Run again"
        : "Work the queue";
  const icon = waiting ? "wait" : running ? "working" : finished ? "again" : "play";
  const extra = finding(board);
  return (
    <section className={styles.bar} aria-label="Queue">
      <div className={styles.heading}>
        <h1 className={styles.title}>Pull request queue</h1>
        <p className={styles.line}>{statusLine(board, now)}</p>
        {extra ? <p className={styles.finding}>{extra}</p> : null}
      </div>
      <div className={styles.actions}>
        <button type="button" className={styles.rules} onClick={onRules}>
          <Icon name="rules" className={styles.rulesIcon} />
          What Upkeep may do
        </button>
        <button
          type="button"
          className={styles.work}
          onClick={onStart}
          disabled={disabled}
          data-busy={running || waiting}
        >
          <Icon name={icon} className={styles.icon} />
          {label}
        </button>
      </div>
      {error ? (
        <p className={styles.error} role="alert">
          <Icon name="alert" className={styles.errorIcon} />
          {error}
        </p>
      ) : null}
    </section>
  );
}
