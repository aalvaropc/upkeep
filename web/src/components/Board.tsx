import { Icon, type IconName } from "./Icon";
import motion from "../motion.module.css";
import styles from "./Board.module.css";

interface Column {
  id: string;
  title: string;
  meaning: string;
  empty: string;
  icon: IconName;
}

export const COLUMNS: readonly Column[] = [
  {
    id: "blocked",
    title: "Blocked",
    meaning: "Needs a change before it can merge",
    empty: "No pull requests yet.",
    icon: "blocked",
  },
  {
    id: "working",
    title: "Working",
    meaning: "Upkeep is fixing it now",
    empty: "Nothing in progress.",
    icon: "working",
  },
  {
    id: "verified",
    title: "Verified",
    meaning: "Passed every check, waiting for its action",
    empty: "Nothing verified yet.",
    icon: "verified",
  },
  {
    id: "done",
    title: "Done",
    meaning: "Merged, pushed or handed to a person",
    empty: "Nothing done yet.",
    icon: "done",
  },
];

export function Board() {
  return (
    <section id="board" className={styles.board} aria-label="Pull requests">
      {COLUMNS.map((column) => (
        <div key={column.id} className={`${styles.column} ${motion.enter}`} data-column={column.id}>
          <header className={styles.head}>
            <Icon name={column.icon} className={styles.icon} />
            <h2 className={styles.title}>{column.title}</h2>
            <span className={styles.count} aria-label="0 pull requests">
              0
            </span>
          </header>
          <p className={styles.meaning}>{column.meaning}</p>
          <p className={styles.empty}>{column.empty}</p>
        </div>
      ))}
    </section>
  );
}
