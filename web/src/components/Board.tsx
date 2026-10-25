import styles from "./Board.module.css";

export const COLUMNS = [
  { id: "blocked", title: "Blocked", empty: "No pull requests yet." },
  { id: "working", title: "Working", empty: "Nothing in progress." },
  { id: "verified", title: "Verified", empty: "Nothing verified yet." },
  { id: "done", title: "Done", empty: "Nothing done yet." },
] as const;

export function Board() {
  return (
    <section id="board" className={styles.board} aria-label="Pull requests">
      {COLUMNS.map((column) => (
        <div key={column.id} className={styles.column} data-column={column.id}>
          <h2 className={styles.title}>{column.title}</h2>
          <p className={styles.empty}>{column.empty}</p>
        </div>
      ))}
    </section>
  );
}
