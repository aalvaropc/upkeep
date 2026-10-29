import type { BoardState } from "../board";
import { workingCards } from "../focus";
import { stepLabel } from "../labels";
import { stages, stagesText } from "../progress";
import styles from "./Lanes.module.css";

interface Props {
  board: BoardState;
  focus: number | null;
  onFocus: (number: number) => void;
}

export function Lanes({ board, focus, onFocus }: Props) {
  const cards = workingCards(board);
  if (cards.length === 0) {
    return null;
  }
  return (
    <div className={styles.lanes} role="group" aria-label="In parallel">
      <span className={styles.title}>In parallel</span>
      <ul className={styles.list}>
        {cards.map((card) => (
          <li key={card.pr.number} className={styles.item}>
            <button
              type="button"
              className={styles.lane}
              aria-pressed={focus === card.pr.number}
              aria-label={`Follow pull request ${card.pr.number}`}
              onClick={() => onFocus(card.pr.number)}
            >
              <code className={styles.number}>#{card.pr.number}</code>
              <span
                key={card.step ? `${card.step.name}:${card.step.status}` : "checking"}
                className={styles.step}
              >
                {card.step ? stepLabel(card.step.name, card.step.status, "") : "checking"}
              </span>
              <span className={styles.dots} role="img" aria-label={stagesText(stages(card))}>
                {stages(card).map((s) => (
                  <span key={s.id} className={styles.dot} data-state={s.state} />
                ))}
              </span>
            </button>
          </li>
        ))}
      </ul>
    </div>
  );
}
