import { useEffect, useRef, type ReactNode } from "react";
import { dollars, outcomeLabel, type Card } from "../board";
import { blockerLabel, stepLabel } from "../labels";
import { stages, stagesText } from "../progress";
import type { TraceEntry } from "../trace";
import { Icon } from "./Icon";
import { TraceView } from "./TraceView";
import styles from "./Stage.module.css";

interface Props {
  card: Card;
  pinned: boolean;
  onFollow: () => void;
  onOpen: (number: number) => void;
  lanes?: ReactNode;
}

const ROLES: Record<string, string> = {
  ultra: "Nemotron Ultra",
  super: "Nemotron Super",
  nano: "Nemotron Nano",
};

function now(entry: TraceEntry | undefined): { tone: string; text: string } | null {
  if (!entry) {
    return null;
  }
  switch (entry.kind) {
    case "model": {
      const who = ROLES[entry.role] ?? entry.model;
      const turn = entry.turn ? ` · turn ${entry.turn}` : "";
      return {
        tone: entry.role,
        text: `${who} answered${turn}${entry.said ? `: “${entry.said}”` : ""}`,
      };
    }
    case "tool":
      return {
        tone: "tool",
        text: `${entry.tool}${entry.target ? ` ${entry.target}` : ""}${entry.ok ? "" : " failed"}${entry.result ? ` → ${entry.result}` : ""}`,
      };
    case "search":
      return {
        tone: "search",
        text: `Tavily searched “${entry.query}” and kept ${entry.kept} pages`,
      };
    case "verify":
      return {
        tone: entry.passed ? "done" : "failed",
        text: entry.passed ? "Every check passed" : "A check failed",
      };
    case "action":
      return { tone: "done", text: `${entry.action.kind} ${entry.action.status}` };
    case "learned":
      return { tone: "learned", text: entry.detail || "Reusing a fix learned earlier" };
    case "step":
      return { tone: "step", text: stepLabel(entry.name, entry.status, entry.detail) };
  }
}

export function Stage({ card, pinned, onFollow, onOpen, lanes }: Props) {
  const feed = useRef<HTMLDivElement | null>(null);
  const live = card.phase === "working" || card.phase === "verified";
  const list = stages(card);
  const latest = now(card.trace[card.trace.length - 1]);
  const count = card.trace.length;

  useEffect(() => {
    const el = feed.current;
    if (el && live && typeof el.scrollTo === "function") {
      el.scrollTo({ top: el.scrollHeight });
    }
  }, [count, live]);

  return (
    <section className={styles.stage} aria-label="Live stage" data-live={live}>
      {lanes}
      <header className={styles.head}>
        <p className={styles.mode}>
          {live ? <span className={styles.dot} aria-hidden="true" /> : null}
          {live ? (pinned ? "Following" : "Now working") : outcomeLabel(card.outcome) || "Waiting"}
          {pinned ? (
            <button type="button" className={styles.follow} onClick={onFollow}>
              Follow the agent
            </button>
          ) : null}
        </p>
        <h2 className={styles.title}>
          <code className={styles.number}>#{card.pr.number}</code> {card.pr.title}
        </h2>
        <p className={styles.meta}>
          <Icon
            name={card.pr.author.kind === "bot" ? "bot" : "person"}
            className={styles.metaIcon}
          />
          {card.pr.author.login}
          {card.pr.bump
            ? ` · ${card.pr.bump.module} ${card.pr.bump.from} → ${card.pr.bump.to}`
            : ""}
          {card.blocker ? ` · ${blockerLabel(card.blocker)}` : ""}
          {card.calls > 0 ? ` · ${card.calls} model calls · ${dollars(card.micros)}` : ""}
        </p>
      </header>

      <ol className={styles.track} aria-label={stagesText(list)}>
        {list.map((s) => (
          <li key={s.id} className={styles.step} data-state={s.state}>
            <span className={styles.bar} />
            <span className={styles.label}>{s.label}</span>
          </li>
        ))}
      </ol>

      {latest && live ? (
        <p className={styles.now} data-tone={latest.tone} aria-live="off">
          {latest.text}
        </p>
      ) : null}

      <div className={styles.feed} ref={feed}>
        <TraceView card={card} onSummary={() => onOpen(card.pr.number)} />
      </div>

      <footer className={styles.foot}>
        <button type="button" className={styles.open} onClick={() => onOpen(card.pr.number)}>
          Open the full detail of #{card.pr.number}
        </button>
      </footer>
    </section>
  );
}
