import styles from "./DiffView.module.css";

interface Props {
  patch: string;
}

type Kind = "meta" | "hunk" | "add" | "del" | "ctx";

export function kindOf(line: string): Kind {
  if (
    line.startsWith("diff ") ||
    line.startsWith("index ") ||
    line.startsWith("+++") ||
    line.startsWith("---") ||
    line.startsWith("new file") ||
    line.startsWith("deleted file")
  ) {
    return "meta";
  }
  if (line.startsWith("@@")) {
    return "hunk";
  }
  if (line.startsWith("+")) {
    return "add";
  }
  if (line.startsWith("-")) {
    return "del";
  }
  return "ctx";
}

export function DiffView({ patch }: Props) {
  const lines = patch.replace(/\n$/, "").split("\n");
  return (
    <pre className={styles.diff} tabIndex={0} aria-label="Patch">
      {lines.map((line, i) => (
        <span key={i} className={styles.line} data-kind={kindOf(line)}>
          {line || " "}
        </span>
      ))}
    </pre>
  );
}
