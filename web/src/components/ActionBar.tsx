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

function share(board: BoardState): number {
  if (board.run === "finished") {
    return 100;
  }
  const total = board.order.length;
  if (total === 0) {
    return 0;
  }
  return Math.round((columns(board).done.length / total) * 100);
}

function title(board: BoardState): string {
  const total = board.order.length;
  switch (board.run) {
    case "idle":
      return total > 0 ? `${total} pull requests can't merge.` : "Getting the queue ready.";
    case "waiting":
      return "Waiting for a slot";
    case "running":
      return "Working the queue";
    case "finished":
      return "The queue is worked";
  }
}

function RunningLine({ board, now }: { board: BoardState; now: number }) {
  const total = board.order.length;
  const done = columns(board).done.length;
  const cost = dollars(board.totals.micros);
  return (
    <>
      <span key={done} className={styles.tick}>
        {done} of {total} finished
      </span>
      {` · ${elapsed(board, now)} · `}
      <span key={cost} className={styles.tick}>
        {cost} so far
      </span>
    </>
  );
}

export function ActionBar({ board, busy, error, now, onStart, onRules }: Props) {
  const waiting = board.run === "waiting";
  const running = board.run === "running";
  const finished = board.run === "finished";
  const idle = board.run === "idle";
  const disabled = busy || waiting || running || (board.order.length === 0 && !finished);
  const label = waiting
    ? `Waiting for a slot`
    : running
      ? "Working"
      : finished
        ? "Run again"
        : "Work the queue";
  const icon = waiting ? "wait" : running ? "working" : finished ? "again" : "play";
  return (
    <section className={styles.bar} aria-label="Queue" data-run={board.run}>
      <div className={styles.heading}>
        <h1 className={styles.title}>
          {running ? <span className={styles.dot} aria-hidden="true" /> : null}
          {title(board)}
        </h1>
        {idle ? (
          <p className={styles.lead}>
            Upkeep plans the merge order, fixes what its policy allows with Nemotron, and proves
            every fix with the compiler and the tests.
          </p>
        ) : (
          <p className={styles.line}>
            {running ? <RunningLine board={board} now={now} /> : statusLine(board, now)}
          </p>
        )}
      </div>
      <div className={styles.actions}>
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
        <button type="button" className={styles.rules} onClick={onRules}>
          <Icon name="rules" className={styles.rulesIcon} />
          What Upkeep may do
        </button>
      </div>
      {idle ? null : (
        <svg
          className={styles.progress}
          viewBox="0 0 100 1"
          preserveAspectRatio="none"
          role="img"
          aria-label={`${share(board)} percent of the pull requests finished`}
        >
          <rect x="0" y="0" height="1" width={share(board)} />
        </svg>
      )}
      {error ? (
        <p className={styles.error} role="alert">
          <Icon name="alert" className={styles.errorIcon} />
          {error}
        </p>
      ) : null}
    </section>
  );
}
