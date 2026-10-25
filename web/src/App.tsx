import styles from "./App.module.css";
import { Board } from "./components/Board";
import { EventLog } from "./components/EventLog";
import { Icon } from "./components/Icon";
import { Models } from "./components/Models";
import { StatusBadge } from "./components/StatusBadge";
import { useEventStream } from "./useEventStream";

interface Props {
  session: string;
}

export function App({ session }: Props) {
  const stream = useEventStream(session);

  return (
    <>
      <a className={styles.skip} href="#main">
        Skip to the board
      </a>
      <header className={styles.bar}>
        <div className={styles.brand}>
          <Icon name="mark" className={styles.mark} />
          <span className={styles.name}>Upkeep</span>
        </div>
        <p className={styles.tagline}>Keeps your pull requests mergeable.</p>
        <StatusBadge status={stream.status} version={stream.version} />
      </header>
      <main id="main" className={styles.main} tabIndex={-1}>
        <section className={styles.toolbar} aria-label="Queue">
          <button type="button" className={styles.work} disabled>
            <Icon name="play" className={styles.play} />
            Work the queue
          </button>
          <p className={styles.hint}>
            The queue runner arrives in a later build. This page already listens to the live event
            stream.
          </p>
        </section>
        <div className={styles.layout}>
          <Board />
          <div className={styles.rail}>
            <Models />
            <EventLog events={stream.events} session={stream.session} />
          </div>
        </div>
      </main>
    </>
  );
}
