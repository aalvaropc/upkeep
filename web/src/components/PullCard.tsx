import { dollars, outcomeLabel, seconds, type Card } from "../board";
import { overlapLabel, overlapTone } from "../plan";
import { Icon, type IconName } from "./Icon";
import styles from "./PullCard.module.css";

interface Props {
  card: Card;
  now: number;
  onOpen: (number: number) => void;
}

const BLOCKERS: Record<string, string> = {
  behind: "behind main",
  conflict: "conflict",
  conflict_large: "large conflict",
  build: "build fails",
  test: "tests fail",
  flaky: "flaky test",
  format: "formatting",
  clean: "nothing blocks it",
};

const ACTIONS: Record<string, { icon: IconName; label: string }> = {
  push: { icon: "push", label: "pushed" },
  patch: { icon: "patch", label: "patch attached" },
  merge: { icon: "merge", label: "merged" },
  comment: { icon: "patch", label: "diagnosis posted" },
};

export function blockerLabel(blocker: string | undefined): string {
  return blocker ? (BLOCKERS[blocker] ?? blocker) : "";
}

const STEPS: Record<string, string> = {
  detect: "Checking",
  plan: "Plan",
  forecast: "Reading the release notes",
  conflict: "Resolving the conflict",
  merge: "Merging main",
  format: "Formatting",
  repair: "Fixing",
  verify: "Verifying",
  explain: "Writing the note",
  diagnose: "Diagnosing",
  learned: "Reusing a learned fix",
  learn: "Remembering the fix",
};

export function stepLabel(name: string, status: string, detail: string): string {
  const base = STEPS[name] ?? name;
  const tail =
    status === "running"
      ? "…"
      : status === "done"
        ? " · done"
        : status === "failed"
          ? " · failed"
          : "";
  return `${base}${tail}${detail ? ` · ${detail}` : ""}`;
}

function stepText(card: Card): string {
  if (card.phase === "queued") {
    return "Waiting for its turn";
  }
  const step = card.step;
  return step ? stepLabel(step.name, step.status, step.detail) : "Starting";
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

export function PullCard({ card, now, onOpen }: Props) {
  const { pr } = card;
  const done = card.phase === "done";
  const lines = done ? card.summary : "";
  return (
    <button
      type="button"
      className={styles.card}
      data-phase={card.phase}
      data-outcome={card.outcome ?? ""}
      onClick={() => onOpen(pr.number)}
      aria-label={`Pull request ${pr.number}: ${pr.title}`}
    >
      <span className={styles.top}>
        <span className={styles.number}>#{pr.number}</span>
        <span
          className={styles.who}
          title={pr.author.kind === "bot" ? "Opened by a bot" : "Opened by a person"}
        >
          <Icon name={pr.author.kind === "bot" ? "bot" : "person"} className={styles.whoIcon} />
          {pr.author.login}
        </span>
        {pr.bump ? <span className={styles.bump}>{pr.bump.level}</span> : null}
        <span className={styles.clock}>{clock(card, now)}</span>
      </span>
      <span className={styles.title}>{pr.title}</span>
      <span className={styles.chips}>
        {card.blocker ? <span className={styles.blocker}>{blockerLabel(card.blocker)}</span> : null}
        {card.learned?.status === "reused" ? (
          <span
            className={styles.learned}
            title={`Reused the fix learned from #${card.learned.from}`}
          >
            <Icon name="learned" className={styles.learnedIcon} />
            learned fix
          </span>
        ) : null}
        {card.overlap ? (
          <span className={styles.overlap} data-tone={overlapTone(card.overlap)}>
            <Icon
              name={overlapTone(card.overlap) === "conflict" ? "alert" : "merge"}
              className={styles.overlapIcon}
            />
            {overlapLabel(card.overlap)}
          </span>
        ) : null}
        {(pr.labels ?? []).map((l) => (
          <span key={l} className={styles.label}>
            {l}
          </span>
        ))}
      </span>
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
            className={styles.outcomeIcon}
          />
          {outcomeLabel(card.outcome)}
        </span>
      ) : (
        <span className={styles.step} data-active={card.phase === "working"}>
          {card.phase === "working" ? <span className={styles.pulse} aria-hidden="true" /> : null}
          {stepText(card)}
        </span>
      )}
      {lines ? <span className={styles.note}>{lines}</span> : null}
      {done && card.actions.length > 0 ? (
        <span className={styles.actions}>
          {card.actions
            .filter((a) => a.status === "done" && ACTIONS[a.kind])
            .map((a) => (
              <span key={a.kind} className={styles.action}>
                <Icon name={ACTIONS[a.kind]?.icon ?? "patch"} className={styles.actionIcon} />
                {ACTIONS[a.kind]?.label}
              </span>
            ))}
          {card.actions.some((a) => a.status === "failed") ? (
            <span className={styles.action} data-failed="true">
              <Icon name="alert" className={styles.actionIcon} />
              an action failed
            </span>
          ) : null}
        </span>
      ) : null}
      {card.calls > 0 ? (
        <span className={styles.foot}>
          {card.calls} model calls · {dollars(card.micros)}
        </span>
      ) : null}
    </button>
  );
}
