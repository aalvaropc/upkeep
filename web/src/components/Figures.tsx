import { columns, dollars, type BoardState } from "../board";
import styles from "./Figures.module.css";

interface Props {
  board: BoardState;
}

export function Figures({ board }: Props) {
  const done = columns(board).done;
  const count = (o: string) => done.filter((c) => c.outcome === o).length;
  const handed = count("needs_human") + count("failed");
  const total = board.order.length;
  const micros = board.summary?.micros ?? board.totals.micros;
  const baseline = board.summary?.baselineMicros ?? board.totals.baselineMicros;
  const calls = board.summary?.calls ?? board.totals.calls;
  const items = [
    { label: "Ready to merge", value: String(count("mergeable")), note: `of ${total} open` },
    { label: "Merged", value: String(count("merged")), note: "bot bumps the policy allows" },
    { label: "Needs a person", value: String(handed), note: "with a diagnosis" },
    {
      label: "Cost of this run",
      value: dollars(micros),
      note:
        baseline > micros
          ? `${calls} model calls · ${dollars(baseline)} at the largest model's price`
          : `${calls} model calls`,
    },
  ];
  return (
    <dl className={styles.figures} aria-label="This run">
      {items.map((i) => (
        <div key={i.label} className={styles.item}>
          <dt className={styles.label}>{i.label}</dt>
          <dd className={styles.value}>{i.value}</dd>
          <dd className={styles.note}>{i.note}</dd>
        </div>
      ))}
    </dl>
  );
}
