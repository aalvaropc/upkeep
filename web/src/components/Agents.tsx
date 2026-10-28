import {
  dollars,
  providerLabel,
  quantity,
  ROLE_ORDER,
  seconds,
  type BoardState,
  type SearchTotals,
} from "../board";
import { Icon } from "./Icon";
import styles from "./Agents.module.css";

interface Props {
  board: BoardState;
}

const TIERS = {
  ultra: {
    name: "Nemotron Ultra",
    size: "550B",
    job: "Resolves small conflicts in one call and writes the diagnosis when a person is needed.",
  },
  super: {
    name: "Nemotron Super",
    size: "120B",
    job: "Fixes the code with tools, up to three attempts, and reads the release notes.",
  },
  nano: {
    name: "Nemotron Nano",
    size: "30B",
    job: "Writes the short note for the reviewer, checked against the change.",
  },
} as const;

function Bar({ value, max, tone }: { value: number; max: number; tone: string }) {
  const width = max > 0 ? Math.max(2, Math.round((value / max) * 100)) : 0;
  return (
    <svg
      className={styles.bar}
      viewBox="0 0 100 6"
      preserveAspectRatio="none"
      role="presentation"
      data-tone={tone}
    >
      <rect className={styles.track} x="0" y="0" width="100" height="6" rx="3" />
      <rect className={styles.fill} x="0" y="0" width={width} height="6" rx="3" />
    </svg>
  );
}

function searchMeta(s: SearchTotals): string {
  if (s.calls === 0 && s.skipped === 0) {
    return "no searches yet";
  }
  const parts = [
    quantity(s.calls, "search", "searches"),
    `${quantity(s.kept, "page", "pages")} kept`,
  ];
  if (s.failed > 0) {
    parts.push(`${s.failed} failed`);
  }
  if (s.skipped > 0) {
    parts.push(`${s.skipped} skipped`);
  }
  return parts.join(" · ");
}

function WebSearch({ search }: { search: SearchTotals }) {
  const active = search.calls > 0 || search.skipped > 0;
  return (
    <div
      className={`${styles.row} ${styles.search}`}
      data-role="search"
      data-active={active}
      role="group"
      aria-label="Web search"
    >
      <Icon name="search" className={styles.icon} />
      <div className={styles.body}>
        <p className={styles.name}>
          <span>Web search · {providerLabel(search.provider || "tavily")}</span>
          {search.recorded ? <span className={styles.size}>recorded</span> : null}
        </p>
        <p className={styles.job}>
          Finds the migration guide and changelog pages that Super reads before it fixes a
          dependency bump.
        </p>
        <p className={styles.meta}>{searchMeta(search)}</p>
        {search.lastQuery ? <p className={styles.query}>“{search.lastQuery}”</p> : null}
        {search.lastProblem ? <p className={styles.meta}>{search.lastProblem}</p> : null}
      </div>
    </div>
  );
}

export function Agents({ board }: Props) {
  const totals = board.totals;
  const maxMicros = Math.max(...ROLE_ORDER.map((r) => board.models[r]?.micros ?? 0), 1);
  const compare = Math.max(totals.micros, totals.baselineMicros, 1);
  return (
    <section className={styles.panel} aria-label="Models">
      <h2 className={styles.title}>Models</h2>
      <ul className={styles.list}>
        {ROLE_ORDER.map((role) => {
          const tier = TIERS[role];
          const m = board.models[role];
          return (
            <li key={role} className={styles.row} data-role={role} data-active={m !== undefined}>
              <Icon name="chip" className={styles.icon} />
              <div className={styles.body}>
                <p className={styles.name}>
                  <span>{tier.name}</span>
                  <span className={styles.size}>{tier.size}</span>
                </p>
                <p className={styles.job}>{tier.job}</p>
                {m ? (
                  <>
                    <Bar value={m.micros} max={maxMicros} tone={role} />
                    <p className={styles.meta}>
                      {m.calls} {m.calls === 1 ? "call" : "calls"} ·{" "}
                      {(m.promptTokens + m.completionTokens).toLocaleString("en-US")} tokens ·{" "}
                      {seconds(m.latencyMs)} · {dollars(m.micros)}
                    </p>
                  </>
                ) : (
                  <p className={styles.meta}>no calls yet</p>
                )}
              </div>
            </li>
          );
        })}
      </ul>
      <WebSearch search={board.search} />
      <div className={styles.compare}>
        <p className={styles.compareTitle}>Cost of this run</p>
        <div className={styles.line}>
          <span>Upkeep</span>
          <Bar value={totals.micros} max={compare} tone="cost" />
          <span className={styles.amount}>{dollars(totals.micros)}</span>
        </div>
        <div className={styles.line}>
          <span>Same tokens, largest model&rsquo;s price</span>
          <Bar value={totals.baselineMicros} max={compare} tone="baseline" />
          <span className={styles.amount}>{dollars(totals.baselineMicros)}</span>
        </div>
        <p className={styles.footnote}>
          A price comparison, not a claim that the largest model would have fixed these.{" "}
          {totals.micros > 0 && totals.baselineMicros > totals.micros
            ? `${(totals.baselineMicros / totals.micros).toFixed(1)} times less.`
            : ""}
        </p>
      </div>
    </section>
  );
}
