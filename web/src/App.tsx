import styles from "./App.module.css";
import { Board } from "./components/Board";
import { EventLog } from "./components/EventLog";
import { StatusBadge } from "./components/StatusBadge";
import { useEventStream } from "./useEventStream";

interface Props {
  session: string;
}

export function App({ session }: Props) {
  const stream = useEventStream(session);

  return (
    <>
      <header className={styles.bar}>
        <div className={styles.brand}>
          <span className={styles.mark} aria-hidden="true" />
          <span>Upkeep</span>
        </div>
        <p className={styles.tagline}>Keeps your pull requests mergeable.</p>
        <StatusBadge status={stream.status} version={stream.version} />
      </header>
      <main className={styles.main}>
        <section className={styles.toolbar} aria-label="Queue">
          <button type="button" className={styles.work} disabled>
            Work the queue
          </button>
          <p className={styles.hint}>
            The queue runner arrives in a later build. This page already listens to the live event
            stream.
          </p>
        </section>
        <Board />
        <EventLog events={stream.events} session={stream.session} />
      </main>
    </>
  );
}
