import { useState } from "react";
import styles from "./App.module.css";
import { ActionBar } from "./components/ActionBar";
import { Activity } from "./components/Activity";
import { Agents } from "./components/Agents";
import { Board } from "./components/Board";
import { DetailPanel } from "./components/DetailPanel";
import { Icon } from "./components/Icon";
import { MergePlan } from "./components/MergePlan";
import { Onboarding } from "./components/Onboarding";
import { PolicyPanel } from "./components/PolicyPanel";
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
        Skip to the board
      </a>
      <TopBar connection={board.connection} version={board.version} info={session.info} />
      <main id="main" className={styles.main} tabIndex={-1}>
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
        <ActionBar
          board={board}
          busy={session.busy}
          error={session.error}
          now={now}
          onStart={() => void session.start()}
          onRules={() => setRules(true)}
        />
        {session.info?.mode === "replay" ? (
          <p className={styles.caption}>
            <strong>Replay.</strong> The model answers come from a recorded run of Nemotron on
            Nebius Token Factory. Git, the sandbox, the compiler and the tests run for real.
          </p>
        ) : null}
        {session.info?.mode === "live" ? (
          <p className={styles.caption}>
            <strong>Live.</strong> Every model call goes to Nemotron on Nebius Token Factory now.
            When the daily budget is spent, runs replay a recording and say so.
          </p>
        ) : null}
        {board.run === "idle" ? <Onboarding /> : null}
        <div className={styles.layout}>
          <Board board={board} now={now} onOpen={setOpen} />
          <div className={styles.rail}>
            <Agents board={board} />
            <MergePlan board={board} />
            <Activity log={board.log} session={board.session} />
          </div>
        </div>
      </main>
      <footer className={styles.footer}>
        <span>Upkeep · MIT · Nemotron on Nebius Token Factory</span>
        <nav aria-label="Project links">
          <a href="https://github.com/aalvaropc/upkeep" target="_blank" rel="noreferrer noopener">
            Source
          </a>
          <a
            href="https://github.com/aalvaropc/upkeep/blob/main/docs/architecture.md"
            target="_blank"
            rel="noreferrer noopener"
          >
            How it works
          </a>
          <a
            href="https://github.com/aalvaropc/upkeep/blob/main/docs/models.md"
            target="_blank"
            rel="noreferrer noopener"
          >
            Which model does what
          </a>
          <a
            href="https://github.com/aalvaropc/upkeep/blob/main/docs/limitations.md"
            target="_blank"
            rel="noreferrer noopener"
          >
            Limitations
          </a>
        </nav>
      </footer>
      {open !== null ? (
        <DetailPanel number={open} load={session.detail} onClose={() => setOpen(null)} />
      ) : null}
      {rules ? <PolicyPanel info={session.info} onClose={() => setRules(false)} /> : null}
    </>
  );
}
