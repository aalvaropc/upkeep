import type { UpkeepEvent } from "../events";
import styles from "./EventLog.module.css";

interface Props {
  events: UpkeepEvent[];
  session: string | null;
}

export function EventLog({ events, session }: Props) {
  return (
    <section className={styles.feed} aria-label="Event stream">
      <h2 className={styles.title}>Events</h2>
      <ol className={styles.events}>
        {events.map((e) => (
          <li key={e.seq}>
            #{e.seq} {e.kind}
            {e.pr ? ` · PR ${e.pr}` : ""}
          </li>
        ))}
        {session ? <li>connected to session {session}</li> : null}
      </ol>
    </section>
  );
}
