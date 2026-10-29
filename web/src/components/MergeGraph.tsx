import { outcomeLabel, type BoardState, type Card } from "../board";
import { queueOrder } from "../focus";
import type { MergePlan } from "../types";
import styles from "./MergeGraph.module.css";

interface Props {
  board: BoardState;
  focus: number | null;
  onFocus: (number: number) => void;
}

type State = "waiting" | "working" | "merged" | "ready" | "person" | "stale" | "failed";

const WORDS: Record<State, string> = {
  waiting: "",
  working: "",
  merged: "merged",
  ready: "ready",
  person: "person",
  stale: "stale",
  failed: "failed",
};

function stateOf(card: Card | undefined, stale: boolean): State {
  if (!card || card.phase === "queued") {
    return "waiting";
  }
  if (card.phase !== "done") {
    return "working";
  }
  switch (card.outcome) {
    case "merged":
      return "merged";
    case "mergeable":
      return stale ? "stale" : "ready";
    case "needs_human":
      return "person";
    default:
      return "failed";
  }
}

function finished(state: State): boolean {
  return state !== "waiting" && state !== "working";
}

function label(n: number, position: number, card: Card | undefined, state: State): string {
  const what =
    state === "waiting"
      ? "waiting"
      : state === "working"
        ? "working"
        : state === "stale"
          ? "went stale"
          : outcomeLabel(card?.outcome).toLowerCase();
  return `#${n} in merge order, position ${position}: ${what}`;
}

function Notes({ plan, focus }: { plan: MergePlan | null; focus: number | null }) {
  if (plan === null) {
    return <p className={styles.note}>Ordered once every pull request is cloned.</p>;
  }
  const reason = focus !== null ? plan.reasons[String(focus)] : undefined;
  const shared = focus !== null ? plan.overlaps.filter((o) => o.a === focus || o.b === focus) : [];
  return (
    <>
      {plan.stale.map((s) => (
        <p key={s.pr} className={styles.warning}>
          <span>
            <strong>#{s.pr} is no longer ready to merge:</strong> it {s.because}.
          </span>
        </p>
      ))}
      {plan.stale.length === 0 && plan.checked ? (
        <p className={styles.note}>
          Every pull request that is ready to merge still merges into main.
        </p>
      ) : null}
      {reason ? <p className={styles.note}>{reason}</p> : null}
      {shared.map((o) => (
        <p key={`${o.a}-${o.b}`} className={styles.note}>
          <span>
            #{o.a} and #{o.b} both change {o.files.join(", ")}
          </span>{" "}
          <span className={styles.verdict} data-conflict={o.conflict}>
            {o.conflict ? "conflict" : "merge cleanly"}
          </span>
        </p>
      ))}
    </>
  );
}

export function MergeGraph({ board, focus, onFocus }: Props) {
  const plan = board.plan;
  const order = queueOrder(board);
  const count = Math.max(order.length, 1);
  const stale = new Set((plan?.stale ?? []).map((s) => s.pr));
  const states = order.map((n) => stateOf(board.cards[n], stale.has(n)));
  const x = (n: number): number => (order.indexOf(n) + 0.5) * 100;
  return (
    <section className={styles.graph} aria-label="Merge order">
      <header className={styles.head}>
        <h2 className={styles.title}>Merge order</h2>
        {plan && plan.overlaps.length > 0 ? (
          <span className={styles.hint}>Arcs join pull requests that change the same files.</span>
        ) : null}
      </header>
      <svg
        className={styles.arcs}
        viewBox={`0 0 ${count * 100} 32`}
        preserveAspectRatio="none"
        aria-hidden="true"
        focusable="false"
      >
        {(plan?.overlaps ?? []).map((o) =>
          order.includes(o.a) && order.includes(o.b) ? (
            <path
              key={`${o.a}-${o.b}`}
              className={styles.arc}
              data-conflict={o.conflict}
              d={`M ${x(o.a)} 32 C ${x(o.a)} 0 ${x(o.b)} 0 ${x(o.b)} 32`}
            />
          ) : null,
        )}
      </svg>
      <ol className={styles.nodes} aria-label="Suggested merge order">
        {order.map((n, i) => {
          const card = board.cards[n];
          const state = states[i] ?? "waiting";
          return (
            <li key={n} className={styles.item} data-state={state} data-reached={finished(state)}>
              <button
                type="button"
                className={styles.node}
                aria-pressed={focus === n}
                aria-label={label(n, i + 1, card, state)}
                onClick={() => onFocus(n)}
              >
                <span className={styles.ring}>#{n}</span>
                <span className={styles.word}>{WORDS[state]}</span>
              </button>
            </li>
          );
        })}
      </ol>
      <div className={styles.notes}>
        <Notes plan={plan} focus={focus} />
      </div>
    </section>
  );
}
