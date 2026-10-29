import { Icon } from "./Icon";
import styles from "./HeroConsole.module.css";

type Tone = "muted" | "accent" | "info" | "search";

interface Line {
  tone: Tone;
  what: string;
  tag?: string;
  detail?: string;
  quote?: string;
  added?: number;
  removed?: number;
  result?: string;
  meta?: string;
  end?: boolean;
}

const STAGES = ["Check", "Forecast", "Fix", "Verify", "Act"];

const LINES: Line[] = [
  { tone: "muted", what: "detect", detail: "build fails", meta: "1.5 s" },
  {
    tone: "accent",
    what: "plan",
    detail: "Super repairs it with 4 tools, up to 3 attempts, then 9 checks",
  },
  {
    tone: "search",
    what: "Tavily search",
    detail: "kept 4 pages",
    quote: "github.com/golang-jwt/jwt/v5 migration guide",
  },
  { tone: "info", tag: "super", what: "forecast", meta: "1.9 s · 6,190 tokens · $0.0019" },
  {
    tone: "accent",
    what: "forecast",
    detail: "the build confirmed 1 of 2 predicted changes",
  },
  {
    tone: "info",
    tag: "super",
    what: "fix · turn 1",
    quote: "We need to update token.go to use RegisteredClaims",
    meta: "4.4 s · $0.0014",
  },
  { tone: "info", what: "apply_edit", detail: "auth/token.go", added: 1, removed: 1 },
  { tone: "info", what: "apply_edit", detail: "auth/token.go", added: 6, removed: 1 },
  { tone: "info", what: "apply_edit", detail: "auth/token.go", added: 5, removed: 5 },
  { tone: "info", what: "apply_edit", detail: "auth/token.go", added: 4, removed: 4 },
  { tone: "info", what: "run_checks", result: "PASS" },
  { tone: "accent", what: "verifier", detail: "9 of 9 checks passed", meta: "765 ms" },
  { tone: "accent", what: "Ready to merge", detail: "7 model calls · $0.0064", end: true },
];

const LABEL =
  "A looping replay of the recorded trace of a jwt major upgrade: Tavily finds the migration guide, Nemotron Super edits auth/token.go in four turns, the compiler and the tests pass, and the pull request is ready to merge for $0.0064.";

export function HeroConsole() {
  return (
    <figure className={styles.console} role="img" aria-label={LABEL}>
      <header className={styles.head}>
        <p className={styles.mode}>
          <span className={styles.dot} />
          Now working
          <span className={styles.pill}>Recorded run</span>
        </p>
        <p className={styles.title}>
          <span className={styles.number}>#5</span> Update github.com/golang-jwt/jwt/v4 to v5.3.1
        </p>
        <p className={styles.meta}>renovate[bot] · v4.5.2 → v5.3.1 · build fails</p>
      </header>
      <ol className={styles.track}>
        {STAGES.map((stage, i) => (
          <li key={stage} className={styles.stage}>
            <span className={styles.bar}>
              <span className={`${styles.fill} ${styles[`b${i + 1}`]}`} />
            </span>
            <span className={styles.label}>{stage}</span>
          </li>
        ))}
      </ol>
      <ol className={styles.lines}>
        {LINES.map((line, i) => (
          <li
            key={`${line.what}-${i}`}
            className={`${styles.line} ${styles[`l${i + 1}`]}`}
            data-tone={line.tone}
            data-end={line.end === true}
          >
            {line.end ? <Icon name="verified" className={styles.endIcon} /> : null}
            {line.tag ? <span className={styles.tag}>{line.tag}</span> : null}
            <span className={styles.what}>{line.what}</span>
            {line.detail ? <code className={styles.detail}>{line.detail}</code> : null}
            {line.added !== undefined ? (
              <span className={styles.diff}>
                <span className={styles.add}>+{line.added}</span>{" "}
                <span className={styles.del}>&minus;{line.removed ?? 0}</span>
              </span>
            ) : null}
            {line.result ? <span className={styles.result}>{line.result}</span> : null}
            {line.quote ? <q className={styles.quote}>{line.quote}</q> : null}
            {line.meta ? <span className={styles.metaText}>{line.meta}</span> : null}
          </li>
        ))}
      </ol>
    </figure>
  );
}
