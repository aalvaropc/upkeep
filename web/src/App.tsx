import { useState } from "react";
import styles from "./App.module.css";
import { ActionBar } from "./components/ActionBar";
import { Activity } from "./components/Activity";
import { Agents } from "./components/Agents";
import { DetailPanel } from "./components/DetailPanel";
import { Figures } from "./components/Figures";
import { Icon } from "./components/Icon";
import { MergePlan } from "./components/MergePlan";
import { Onboarding } from "./components/Onboarding";
import { PolicyPanel } from "./components/PolicyPanel";
import { Queue } from "./components/Queue";
import { Sidebar } from "./components/Sidebar";
import { TopBar } from "./components/TopBar";
import { useNow } from "./useNow";
import { useSession } from "./useSession";

export function App() {
  const session = useSession();
  const { board } = session;
  const [open, setOpen] = useState<number | null>(null);
  const [rules, setRules] = useState(false);
  const now = useNow(board.run === "running");

  return (
    <>
      <a className={styles.skip} href="#main">
        Skip to the queue
      </a>
      <div className={styles.frame}>
        <Sidebar onRules={() => setRules(true)} />
        <div className={styles.work}>
          <TopBar connection={board.connection} version={board.version} info={session.info} />
          <main id="main" className={styles.sheet} tabIndex={-1}>
            <ActionBar
              board={board}
              busy={session.busy}
              error={session.error}
              now={now}
              onStart={() => void session.start()}
              onRules={() => setRules(true)}
            />
            {board.closed ? (
              <p className={styles.notice} role="note">
                <Icon name="alert" className={styles.noticeIcon} />
                This session ended. Press the button to start a new one.
              </p>
            ) : null}
            {board.notices.map((n) => (
              <p key={n} className={styles.notice} role="note">
                <Icon name="alert" className={styles.noticeIcon} />
                {n}
              </p>
            ))}
            {session.info?.mode === "replay" ? (
              <p className={styles.caption}>
                Replay: the model answers come from a recorded run of Nemotron on Nebius Token
                Factory. Git, the sandbox, the compiler and the tests run for real.
              </p>
            ) : null}
            {session.info?.mode === "live" ? (
              <p className={styles.caption}>
                Live: every model call goes to Nemotron on Nebius Token Factory now. When the daily
                budget is spent, runs replay a recording and say so.
              </p>
            ) : null}
            {board.run === "idle" ? <Onboarding /> : <Figures board={board} />}
            <div className={styles.layout}>
              <div className={styles.main}>
                <Queue board={board} now={now} onOpen={setOpen} />
                <Activity log={board.log} session={board.session} />
              </div>
              <div className={styles.rail}>
                <Agents board={board} />
                <MergePlan board={board} />
              </div>
            </div>
          </main>
        </div>
      </div>
      {open !== null ? (
        <DetailPanel
          number={open}
          card={board.cards[open]}
          load={session.detail}
          onClose={() => setOpen(null)}
        />
      ) : null}
      {rules ? <PolicyPanel info={session.info} onClose={() => setRules(false)} /> : null}
    </>
  );
}
