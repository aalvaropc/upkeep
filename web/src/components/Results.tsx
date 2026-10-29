import { dollars, type BoardState, type Card } from "../board";
import { queueOrder } from "../focus";
import { ACTIONS } from "../labels";
import { Figures } from "./Figures";
import { Icon, type IconName } from "./Icon";
import styles from "./Results.module.css";

interface Props {
  board: BoardState;
  onOpen: (number: number) => void;
}

const GROUPS: { id: string; title: string; icon: IconName; keep: (c: Card) => boolean }[] = [
  { id: "merged", title: "Merged", icon: "done", keep: (c) => c.outcome === "merged" },
  {
    id: "ready",
    title: "Ready to merge",
    icon: "verified",
    keep: (c) => c.outcome === "mergeable",
  },
  {
    id: "person",
    title: "Needs a person",
    icon: "person",
    keep: (c) => c.outcome === "needs_human" || c.outcome === "failed",
  },
];

function did(card: Card): string {
  const acts = card.actions
    .filter((a) => a.status === "done" && ACTIONS[a.kind])
    .map((a) => ACTIONS[a.kind]?.label);
  return acts.join(", ");
}

export function Results({ board, onOpen }: Props) {
  const ordered = queueOrder(board)
    .map((n) => board.cards[n])
    .filter((c): c is Card => Boolean(c));
  const stale = new Set((board.plan?.stale ?? []).map((s) => s.pr));
  return (
    <section className={styles.results} aria-label="Results">
      <Figures board={board} />

      {board.plan ? (
        <div className={styles.block}>
          <h2 className={styles.heading}>The merge order Upkeep planned</h2>
          <ol className={styles.line}>
            {ordered.map((c) => (
              <li
                key={c.pr.number}
                className={styles.stop}
                data-outcome={c.outcome ?? ""}
                data-stale={stale.has(c.pr.number)}
              >
                <button
                  type="button"
                  className={styles.stopButton}
                  onClick={() => onOpen(c.pr.number)}
                >
                  <code>#{c.pr.number}</code>
                </button>
              </li>
            ))}
          </ol>
          {(board.plan.stale ?? []).map((s) => (
            <p key={s.pr} className={styles.warning}>
              <Icon name="alert" className={styles.icon} />#{s.pr} went stale: {s.because}. Merge it
              after updating its branch.
            </p>
          ))}
        </div>
      ) : null}

      <div className={styles.groups}>
        {GROUPS.map((g) => {
          const cards = ordered.filter(g.keep);
          return (
            <div key={g.id} className={styles.group} data-group={g.id}>
              <h2 className={styles.heading}>
                <Icon name={g.icon} className={styles.icon} />
                {g.title} <span className={styles.n}>{cards.length}</span>
              </h2>
              {cards.length === 0 ? (
                <p className={styles.none}>None</p>
              ) : (
                <ul className={styles.items}>
                  {cards.map((c) => (
                    <li key={c.pr.number}>
                      <button
                        type="button"
                        className={styles.item}
                        onClick={() => onOpen(c.pr.number)}
                      >
                        <span className={styles.itemTop}>
                          <code className={styles.num}>#{c.pr.number}</code>
                          <span className={styles.itemTitle}>{c.pr.title}</span>
                        </span>
                        <span className={styles.itemSub}>
                          {c.summary || did(c) || "No change needed"}
                          {c.calls > 0 ? ` · ${dollars(c.micros)}` : ""}
                          {c.learned?.status === "reused" ? " · learned fix" : ""}
                        </span>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          );
        })}
      </div>
    </section>
  );
}
