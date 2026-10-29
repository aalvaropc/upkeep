import { outcomeLabel, type BoardState, type Card } from "../board";
import { queueOrder } from "../focus";
import { blockerLabel, stepLabel } from "../labels";
import { overlapLabel, overlapTone } from "../plan";
import { stages, stagesText } from "../progress";
import { Icon, type IconName } from "./Icon";
import styles from "./QueueList.module.css";

interface Props {
  board: BoardState;
  focus: number | null;
  onFocus: (number: number) => void;
}

function outcomeIcon(card: Card): IconName {
  switch (card.outcome) {
    case "merged":
      return "done";
    case "needs_human":
      return "person";
    case "failed":
      return "alert";
    default:
      return "verified";
  }
}

function status(card: Card): string {
  if (card.phase === "done") {
    return outcomeLabel(card.outcome);
  }
  if (card.phase === "queued") {
    return card.blocker ? blockerLabel(card.blocker) : "waiting";
  }
  return card.step ? stepLabel(card.step.name, card.step.status, "") : "checking";
}

export function QueueList({ board, focus, onFocus }: Props) {
  const cards = queueOrder(board)
    .map((n) => board.cards[n])
    .filter((c): c is Card => Boolean(c));
  const finished = cards.length > 0 && cards.every((c) => c.phase === "done");
  return (
    <section
      id="board"
      className={styles.queue}
      aria-label="Pull requests"
      data-finished={finished}
    >
      <header className={styles.head}>
        <h2 className={styles.title}>Queue</h2>
        <span className={styles.count}>
          {board.plan ? "in merge order" : "by number"} · {cards.length}
        </span>
      </header>
      <ol className={styles.list}>
        {cards.map((card) => {
          const list = stages(card);
          const live = card.phase === "working" || card.phase === "verified";
          return (
            <li key={card.pr.number}>
              <button
                type="button"
                className={styles.row}
                data-phase={card.phase}
                data-outcome={card.outcome ?? ""}
                aria-pressed={focus === card.pr.number}
                aria-label={`Pull request ${card.pr.number}: ${card.pr.title}`}
                onClick={() => onFocus(card.pr.number)}
              >
                <span className={styles.top}>
                  <code className={styles.number}>#{card.pr.number}</code>
                  <span className={styles.name}>{card.pr.title}</span>
                  {card.phase === "done" ? (
                    <Icon name={outcomeIcon(card)} className={styles.outcome} />
                  ) : live ? (
                    <span className={styles.pulse} aria-hidden="true" />
                  ) : null}
                </span>
                <span className={styles.bottom}>
                  <span className={styles.dots} role="img" aria-label={stagesText(list)}>
                    {list.map((s) => (
                      <span key={s.id} className={styles.dotStage} data-state={s.state} />
                    ))}
                  </span>
                  <span className={styles.status}>{status(card)}</span>
                  {live && card.blocker ? (
                    <span className={styles.blocker}>{blockerLabel(card.blocker)}</span>
                  ) : null}
                  <span className={styles.chips}>
                    {card.overlap ? (
                      <span
                        className={styles.chip}
                        data-tone={overlapTone(card.overlap)}
                        title={overlapLabel(card.overlap)}
                      >
                        <span className={styles.sr}>{overlapLabel(card.overlap)}</span>
                        <Icon
                          name={overlapTone(card.overlap) === "conflict" ? "alert" : "merge"}
                          className={styles.chipIcon}
                        />
                      </span>
                    ) : null}
                    {card.learned?.status === "reused" ? (
                      <span
                        className={styles.chip}
                        data-tone="learned"
                        title={`Reused the fix learned from #${card.learned.from}`}
                      >
                        <span
                          className={styles.sr}
                          title={`Reused the fix learned from #${card.learned.from}`}
                        >
                          learned fix
                        </span>
                        <Icon name="learned" className={styles.chipIcon} />
                      </span>
                    ) : null}
                    {card.phase === "done" && card.confidence ? (
                      <span className={styles.chip} title={card.confidence.reasons.join("; ")}>
                        <span
                          className={styles.sr}
                          data-level={card.confidence.level}
                          title={card.confidence.reasons.join("; ")}
                        >
                          {card.confidence.level} merge confidence
                        </span>
                        <span
                          className={styles.pip}
                          data-level={card.confidence.level}
                          aria-hidden="true"
                        />
                      </span>
                    ) : null}
                  </span>
                </span>
              </button>
            </li>
          );
        })}
      </ol>
    </section>
  );
}
