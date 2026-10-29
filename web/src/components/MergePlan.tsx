import { outcomeLabel, type BoardState, type Card } from "../board";
import { Icon, type IconName } from "./Icon";
import styles from "./MergePlan.module.css";

interface Props {
  board: BoardState;
}

interface Status {
  tone: string;
  icon: IconName;
  label: string;
}

function status(card: Card | undefined, stale: boolean): Status {
  if (!card || card.phase === "queued") {
    return { tone: "muted", icon: "wait", label: "Waiting" };
  }
  if (card.phase !== "done") {
    return { tone: "warn", icon: "working", label: "Working" };
  }
  switch (card.outcome) {
    case "merged":
      return { tone: "accent", icon: "done", label: outcomeLabel("merged") };
    case "mergeable":
      return stale
        ? { tone: "warn", icon: "alert", label: "Went stale" }
        : { tone: "info", icon: "verified", label: outcomeLabel("mergeable") };
    case "needs_human":
      return { tone: "warn", icon: "person", label: outcomeLabel("needs_human") };
    default:
      return { tone: "danger", icon: "alert", label: outcomeLabel("failed") };
  }
}

function free(reason: string | undefined): boolean {
  return Boolean(reason?.startsWith("Shares no files"));
}

export function MergePlan({ board }: Props) {
  const plan = board.plan;
  return (
    <section className={styles.panel} aria-label="Merge plan">
      <h2 className={styles.title}>Merge plan</h2>
      {plan === null ? (
        <p className={styles.empty}>
          Once every pull request is cloned, Upkeep compares the files they change and suggests an
          order to merge them in.
        </p>
      ) : (
        <>
          <ol className={styles.order} aria-label="Suggested merge order">
            {plan.order.map((n, i) => {
              const card = board.cards[n];
              const stale = plan.stale.some((s) => s.pr === n);
              const s = status(card, stale);
              return (
                <li key={n} className={styles.step}>
                  <span className={styles.position} aria-hidden="true">
                    {i + 1}
                  </span>
                  <p className={styles.head}>
                    <span className={styles.number}>#{n}</span>
                    <span className={styles.name} title={card?.pr.title}>
                      {card?.pr.title ?? ""}
                    </span>
                  </p>
                  <span className={styles.status} data-tone={s.tone}>
                    <Icon name={s.icon} className={styles.statusIcon} />
                    {s.label}
                  </span>
                  {plan.reasons[String(n)] && !free(plan.reasons[String(n)]) ? (
                    <p className={styles.reason}>{plan.reasons[String(n)]}</p>
                  ) : null}
                </li>
              );
            })}
          </ol>
          {plan.order.some((n) => free(plan.reasons[String(n)])) ? (
            <p className={styles.reason}>
              {plan.order
                .filter((n) => free(plan.reasons[String(n)]))
                .map((n) => `#${n}`)
                .join(", ")}{" "}
              share no files with another open pull request, so they can merge in any order.
            </p>
          ) : null}
          {plan.overlaps.length > 0 ? (
            <div className={styles.section}>
              <h3 className={styles.subtitle}>Shared files</h3>
              <ul className={styles.overlaps}>
                {plan.overlaps.map((o) => (
                  <li key={`${o.a}-${o.b}`} className={styles.overlap}>
                    <span className={styles.pair}>
                      #{o.a} and #{o.b} both change {o.files.join(", ")}
                    </span>
                    <span className={styles.verdict} data-conflict={o.conflict}>
                      <Icon name={o.conflict ? "alert" : "merge"} className={styles.statusIcon} />
                      {o.conflict ? "conflict" : "merge cleanly"}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          {plan.stale.length > 0 ? (
            <div className={styles.section}>
              <h3 className={styles.subtitle}>After the run</h3>
              {plan.stale.map((s) => (
                <p key={s.pr} className={styles.warning}>
                  <Icon name="alert" className={styles.warningIcon} />
                  <span>
                    <strong>#{s.pr} is no longer ready to merge:</strong> it {s.because}.
                  </span>
                </p>
              ))}
            </div>
          ) : plan.checked ? (
            <div className={styles.section}>
              <h3 className={styles.subtitle}>After the run</h3>
              <p className={styles.fine}>
                <Icon name="done" className={styles.warningIcon} />
                <span>Every pull request that is ready to merge still merges into main.</span>
              </p>
            </div>
          ) : null}
        </>
      )}
    </section>
  );
}
