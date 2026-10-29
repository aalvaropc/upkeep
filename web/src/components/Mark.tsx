import styles from "./Mark.module.css";

interface Props {
  className?: string;
  running?: boolean;
  intro?: boolean;
}

const BARS = [
  { x: 1.5, height: 7 },
  { x: 6, height: 10 },
  { x: 10.5, height: 13 },
  { x: 15, height: 16 },
  { x: 19.5, height: 19 },
];

export function Mark({ className, running = false, intro = false }: Props) {
  return (
    <svg
      className={`${styles.mark} ${className ?? ""}`}
      data-running={running}
      data-intro={intro}
      viewBox="0 0 24 24"
      width="1em"
      height="1em"
      aria-hidden="true"
      focusable="false"
    >
      {BARS.map((bar) => (
        <rect
          key={bar.x}
          className={styles.bar}
          x={bar.x}
          y={22 - bar.height}
          width="3"
          height={bar.height}
          rx="1.5"
        />
      ))}
    </svg>
  );
}
