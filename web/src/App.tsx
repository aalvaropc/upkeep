import { useState } from "react";
import styles from "./App.module.css";
import { ActionBar } from "./components/ActionBar";
import { Agents } from "./components/Agents";
import { DetailPanel } from "./components/DetailPanel";
import { FlowSteps } from "./components/FlowSteps";
import { Icon } from "./components/Icon";
import { MergePlan } from "./components/MergePlan";
import { Onboarding } from "./components/Onboarding";
import { PolicyPanel } from "./components/PolicyPanel";
import { QueueList } from "./components/QueueList";
import { Results } from "./components/Results";
import { Stage } from "./components/Stage";
import { TopBar } from "./components/TopBar";
import { liveCard } from "./focus";
import { useNow } from "./useNow";
import { useSession } from "./useSession";

const DOCS = [
  {
    href: "https://github.com/aalvaropc/upkeep/blob/main/docs/architecture.md",
    label: "How it works",
  },
  {
    href: "https://github.com/aalvaropc/upkeep/blob/main/docs/models.md",
    label: "Which model does what",
  },
  {
    href: "https://github.com/aalvaropc/upkeep/blob/main/docs/evals/live.md",
    label: "Live evaluation",
  },
  {
    href: "https://github.com/aalvaropc/upkeep/blob/main/docs/limitations.md",
    label: "Limitations",
  },
  { href: "https://github.com/aalvaropc/upkeep", label: "Source" },
];

export function App() {
  const session = useSession();
  const { board } = session;
  const [open, setOpen] = useState<number | null>(null);
  const [pinned, setPinned] = useState<number | null>(null);
  const [rules, setRules] = useState(false);
  const now = useNow(board.run === "running");

  const followed = pinned !== null ? board.cards[pinned] : undefined;
  const focus = followed ?? (board.run === "running" ? liveCard(board) : null);
  const finished = board.run === "finished";

  let center;
  if (board.run === "idle" && !followed) {
    center = <Onboarding />;
  } else if (finished && !followed) {
    center = <Results board={board} onOpen={setOpen} />;
  } else if (focus) {
    center = (
      <Stage
        key={focus.pr.number}
        card={focus}
        pinned={pinned !== null}
        onFollow={() => setPinned(null)}
        onOpen={setOpen}
      />
    );
  } else {
    center = (
      <p className={styles.waiting}>
        Cloning the pull requests and planning the merge order. The first one appears here in a
        moment.
      </p>
    );
  }

  return (
    <>
      <a className={styles.skip} href="#main">
        Skip to the queue
      </a>
      <TopBar connection={board.connection} version={board.version} info={session.info} />
      <main id="main" className={styles.main} tabIndex={-1}>
        <ActionBar
          board={board}
          busy={session.busy}
          error={session.error}
          now={now}
          onStart={() => {
            setPinned(null);
            void session.start();
          }}
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
            Replay: the model answers come from a recorded run of Nemotron on Nebius Token Factory.
            Git, the sandbox, the compiler and the tests run for real.
          </p>
        ) : null}
        {session.info?.mode === "live" ? (
          <p className={styles.caption}>
            Live: every model call goes to Nemotron on Nebius Token Factory now. When the daily
            budget is spent, runs replay a recording and say so.
          </p>
        ) : null}
        {board.run === "idle" ? null : <FlowSteps board={board} />}
        <div className={styles.layout} data-run={board.run}>
          <QueueList
            board={board}
            focus={followed ? pinned : (focus?.pr.number ?? null)}
            onFocus={(n) => setPinned(n === pinned ? null : n)}
          />
          <div className={styles.center}>
            {finished && followed ? (
              <button type="button" className={styles.back} onClick={() => setPinned(null)}>
                <Icon name="done" className={styles.backIcon} />
                Back to the results
              </button>
            ) : null}
            {center}
          </div>
          <div className={styles.rail}>
            <Agents board={board} />
            {finished && !followed ? null : <MergePlan board={board} />}
          </div>
        </div>
      </main>
      <footer className={styles.footer}>
        <span>Upkeep · NVIDIA Nemotron on Nebius Token Factory · Tavily · MIT</span>
        <nav aria-label="Project links" className={styles.links}>
          {DOCS.map((d) => (
            <a key={d.href} href={d.href} target="_blank" rel="noreferrer noopener">
              {d.label}
            </a>
          ))}
        </nav>
      </footer>
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
