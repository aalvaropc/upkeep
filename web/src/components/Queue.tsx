import { useState } from "react";
import { dollars, outcomeLabel, seconds, type BoardState, type Card } from "../board";
import { ACTIONS, blockerLabel, stepLabel } from "../labels";
import { overlapLabel, overlapTone } from "../plan";
import { stages, stagesText } from "../progress";
import { Icon } from "./Icon";
import { Tabs } from "./Tabs";
import styles from "./Queue.module.css";

interface Props {
  board: BoardState;
  now: number;
  onOpen: (number: number) => void;
}

type Filter = "all" | "working" | "mergeable" | "merged" | "needs_human";

const FILTERS: { id: Filter; label: string; keep: (c: Card) => boolean }[] = [
  { id: "all", label: "All", keep: () => true },
  { id: "working", label: "In progress", keep: (c) => c.phase !== "done" },
  { id: "mergeable", label: "Ready to merge", keep: (c) => c.outcome === "mergeable" },
  { id: "merged", label: "Merged", keep: (c) => c.outcome === "merged" },
  {
    id: "needs_human",
    label: "Needs a person",
    keep: (c) => c.outcome === "needs_human" || c.outcome === "failed",
  },
];

export function queueOrder(board: BoardState): number[] {
  const planned = (board.plan?.order ?? []).filter((n) => board.cards[n]);
  const rest = board.order.filter((n) => !planned.includes(n));
  return [...planned, ...rest];
}

function clock(card: Card, now: number): string {
  if (card.phase === "done" && card.durationMs !== undefined) {
    return seconds(card.durationMs);
  }
  if (card.startedAt && card.phase !== "queued") {
    return `${Math.max(0, Math.round((now - card.startedAt) / 1000))} s`;
  }
  return "";
}

function progressText(card: Card): string {
  if (card.phase === "queued") {
    return "Waiting for its turn";
  }
  if (card.phase === "done") {
    const acts = card.actions
      .filter((a) => a.status === "done" && ACTIONS[a.kind])
      .map((a) => ACTIONS[a.kind]?.label);
    return acts.length > 0 ? acts.join(", ") : "nothing to push";
  }
  const step = card.step;
  return step ? stepLabel(step.name, step.status, "") : "Starting";
}

function Strip({ card }: { card: Card }) {
  const list = stages(card);
  return (
    <span className={styles.strip} role="img" aria-label={stagesText(list)}>
      {list.map((s) => (
        <span
          key={s.id}
          className={styles.stage}
          data-state={s.state}
          title={`${s.label}: ${s.state}`}
        >
          <span className={styles.stageBar} />
        </span>
      ))}
    </span>
  );
}

function Row({ card, now, onOpen }: { card: Card; now: number; onOpen: (n: number) => void }) {
  const { pr } = card;
  const done = card.phase === "done";
  const login =
    pr.author.kind === "bot" ? pr.author.login.replace(/\[bot\]$/, "") : pr.author.login;
  return (
    <tr className={styles.row} data-phase={card.phase} data-outcome={card.outcome ?? ""}>
      <td className={styles.pull}>
        <button
          type="button"
          className={styles.open}
          onClick={() => onOpen(pr.number)}
          aria-label={`Pull request ${pr.number}: ${pr.title}`}
        >
          <span className={styles.title}>
            <code className={styles.number}>#{pr.number}</code>
            {pr.title}
          </span>
          <span className={styles.meta}>
            <Icon name={pr.author.kind === "bot" ? "bot" : "person"} className={styles.metaIcon} />
            {login}
            {pr.bump ? <span className={styles.bump}>{pr.bump.level} bump</span> : null}
            {(pr.labels ?? []).map((l) => (
              <span key={l} className={styles.label}>
                {l}
              </span>
            ))}
          </span>
        </button>
        {done && card.summary ? <p className={styles.summary}>{card.summary}</p> : null}
      </td>
      <td className={styles.blocker}>
        {card.blocker ? blockerLabel(card.blocker) : <span className={styles.dash}>–</span>}
        {card.overlap ? (
          <span className={styles.overlap} data-tone={overlapTone(card.overlap)}>
            <Icon
              name={overlapTone(card.overlap) === "conflict" ? "alert" : "merge"}
              className={styles.inlineIcon}
            />
            {overlapLabel(card.overlap)}
          </span>
        ) : null}
      </td>
      <td className={styles.progress}>
        <Strip card={card} />
        <span className={styles.caption} data-active={card.phase === "working"}>
          {progressText(card)}
          {card.attempts > 0
            ? ` · ${card.attempts} ${card.attempts === 1 ? "attempt" : "attempts"}`
            : ""}
        </span>
      </td>
      <td className={styles.result}>
        {done ? (
          <span className={styles.outcome} data-outcome={card.outcome}>
            <Icon
              name={
                card.outcome === "merged"
                  ? "done"
                  : card.outcome === "needs_human"
                    ? "person"
                    : card.outcome === "failed"
                      ? "alert"
                      : "verified"
              }
              className={styles.inlineIcon}
            />
            {outcomeLabel(card.outcome)}
          </span>
        ) : (
          <span className={styles.dash}>–</span>
        )}
        {done && card.confidence ? (
          <span
            className={styles.confidence}
            data-level={card.confidence.level}
            title={card.confidence.reasons.join("; ")}
          >
            <Icon name="gauge" className={styles.inlineIcon} />
            {card.confidence.level} merge confidence
          </span>
        ) : null}
        {card.learned?.status === "reused" ? (
          <span
            className={styles.learned}
            title={`Reused the fix learned from #${card.learned.from}`}
          >
            <Icon name="learned" className={styles.inlineIcon} />
            learned fix
          </span>
        ) : null}
      </td>
      <td className={styles.cost}>
        <span className={styles.figure}>{card.calls > 0 ? dollars(card.micros) : "$0"}</span>
        <span className={styles.sub}>
          {card.calls} {card.calls === 1 ? "call" : "calls"}
        </span>
        {clock(card, now) ? <span className={styles.sub}>{clock(card, now)}</span> : null}
      </td>
    </tr>
  );
}

function Table({
  cards,
  now,
  onOpen,
}: {
  cards: Card[];
  now: number;
  onOpen: (n: number) => void;
}) {
  if (cards.length === 0) {
    return <p className={styles.empty}>No pull request is in this state.</p>;
  }
  return (
    <table className={styles.table}>
      <thead>
        <tr>
          <th scope="col">Pull request</th>
          <th scope="col">Blocker</th>
          <th scope="col">
            Progress
            <span className={styles.legend}>check, forecast, fix, verify, act</span>
          </th>
          <th scope="col">Result</th>
          <th scope="col" className={styles.right}>
            Cost
          </th>
        </tr>
      </thead>
      <tbody>
        {cards.map((card) => (
          <Row key={card.pr.number} card={card} now={now} onOpen={onOpen} />
        ))}
      </tbody>
    </table>
  );
}

export function Queue({ board, now, onOpen }: Props) {
  const [filter, setFilter] = useState<Filter>("all");
  const ordered = queueOrder(board)
    .map((n) => board.cards[n])
    .filter((c): c is Card => Boolean(c));
  const finished = ordered.length > 0 && ordered.every((c) => c.phase === "done");
  return (
    <section
      id="board"
      className={styles.queue}
      aria-label="Pull requests"
      data-finished={finished}
    >
      <p className={styles.order}>
        {board.plan
          ? "In the merge order Upkeep planned"
          : "In number order until the plan is ready"}
        <span className={styles.count}>{ordered.length} open pull requests</span>
      </p>
      <Tabs
        label="Pull requests by state"
        selected={filter}
        onSelect={(id) => setFilter(id as Filter)}
        tabs={FILTERS.map((f) => {
          const cards = ordered.filter(f.keep);
          return {
            id: f.id,
            label: `${f.label} ${cards.length}`,
            content: <Table cards={cards} now={now} onOpen={onOpen} />,
          };
        })}
      />
    </section>
  );
}
