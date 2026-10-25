import type { LogEntry } from "../board";
import styles from "./Activity.module.css";

interface Props {
  log: LogEntry[];
  session: string | null;
}

const SHOWN = 14;

const TONES: Record<string, string> = {
  "pr.blocker": "danger",
  "pr.step": "warn",
  "model.call": "info",
  "tool.call": "info",
  "verify.result": "info",
  receipt: "info",
  action: "accent",
  "pr.done": "accent",
  "run.finished": "accent",
  notice: "warn",
};

export function Activity({ log, session }: Props) {
  return (
    <section className={styles.feed} aria-label="Activity">
      <header className={styles.head}>
        <h2 className={styles.title}>Activity</h2>
        {session ? <code className={styles.session}>session {session.slice(0, 8)}</code> : null}
      </header>
      {log.length === 0 ? (
        <p className={styles.empty}>Waiting for the first event.</p>
      ) : (
        <ol className={styles.events}>
          {log.slice(0, SHOWN).map((e) => (
            <li key={e.seq} data-tone={TONES[e.kind] ?? "muted"}>
              <span className={styles.who}>{e.pr ? `#${e.pr}` : "run"}</span>
              <span className={styles.text}>{e.text}</span>
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
