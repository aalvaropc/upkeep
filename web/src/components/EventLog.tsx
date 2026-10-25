import type { UpkeepEvent } from "../events";
import motion from "../motion.module.css";
import styles from "./EventLog.module.css";

interface Props {
  events: UpkeepEvent[];
  session: string | null;
}

export function EventLog({ events, session }: Props) {
  return (
    <section className={`${styles.feed} ${motion.enter}`} aria-label="Event stream">
      <header className={styles.head}>
        <h2 className={styles.title}>Events</h2>
        {session ? <code className={styles.session}>session {session}</code> : null}
      </header>
      {events.length === 0 ? (
        <p className={styles.empty}>Waiting for the first event.</p>
      ) : (
        <ol className={styles.events}>
          {events.map((e) => (
            <li key={e.seq} data-kind={e.kind}>
              #{e.seq} {e.kind}
              {e.pr ? ` · PR ${e.pr}` : ""}
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}
