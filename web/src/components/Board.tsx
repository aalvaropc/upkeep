import { columns, type BoardState, type Card } from "../board";
import { Icon, type IconName } from "./Icon";
import { PullCard } from "./PullCard";
import styles from "./Board.module.css";

interface Props {
  board: BoardState;
  now: number;
  onOpen: (number: number) => void;
}

interface ColumnSpec {
  id: "queued" | "working" | "verified" | "done";
  tone: string;
  title: string;
  meaning: string;
  empty: string;
  icon: IconName;
}

export const COLUMNS: readonly ColumnSpec[] = [
  {
    id: "queued",
    tone: "blocked",
    title: "Blocked",
    meaning: "Needs a change before it can merge",
    empty: "Nothing waiting.",
    icon: "blocked",
  },
  {
    id: "working",
    tone: "working",
    title: "Working",
    meaning: "Upkeep is fixing it now",
    empty: "Nothing in progress.",
    icon: "working",
  },
  {
    id: "verified",
    tone: "verified",
    title: "Verified",
    meaning: "Passed every check, waiting for its action",
    empty: "Nothing verified yet.",
    icon: "verified",
  },
  {
    id: "done",
    tone: "done",
    title: "Done",
    meaning: "Merged, pushed or handed to a person",
    empty: "Nothing done yet.",
    icon: "done",
  },
];

export function Board({ board, now, onOpen }: Props) {
  const cols = columns(board);
  const finished =
    cols.done.length > 0 &&
    cols.queued.length === 0 &&
    cols.working.length === 0 &&
    cols.verified.length === 0;
  return (
    <section
      id="board"
      className={styles.board}
      data-finished={finished}
      aria-label="Pull requests"
    >
      {COLUMNS.map((column) => {
        const cards: Card[] = cols[column.id];
        return (
          <div
            key={column.id}
            className={styles.column}
            data-column={column.tone}
            data-empty={cards.length === 0}
          >
            <header className={styles.head}>
              <Icon name={column.icon} className={styles.icon} />
              <h2 className={styles.title}>{column.title}</h2>
              <span className={styles.count} aria-label={`${cards.length} pull requests`}>
                {cards.length}
              </span>
            </header>
            <p className={styles.meaning}>{column.meaning}</p>
            {cards.length === 0 ? (
              <p className={styles.empty}>{column.empty}</p>
            ) : (
              <ul className={styles.list}>
                {cards.map((card) => (
                  <li key={card.pr.number}>
                    <PullCard card={card} now={now} onOpen={onOpen} />
                  </li>
                ))}
              </ul>
            )}
          </div>
        );
      })}
    </section>
  );
}
