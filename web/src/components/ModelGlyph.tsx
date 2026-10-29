import styles from "./ModelGlyph.module.css";

interface Props {
  level: 1 | 2 | 3;
  className?: string;
}

const BARS = [
  { x: 3.5, height: 8 },
  { x: 10, height: 13 },
  { x: 16.5, height: 18 },
];

export function ModelGlyph({ level, className }: Props) {
  return (
    <svg
      className={`${styles.glyph} ${className ?? ""}`}
      viewBox="0 0 24 24"
      width="1em"
      height="1em"
      aria-hidden="true"
      focusable="false"
    >
      {BARS.map((bar, i) => (
        <rect
          key={bar.x}
          className={styles.bar}
          data-lit={i >= 3 - level}
          x={bar.x}
          y={22 - bar.height}
          width="4"
          height={bar.height}
          rx="1"
        />
      ))}
    </svg>
  );
}
