import type { ReactNode } from "react";
import {
  dollars,
  outcomeLabel,
  providerLabel,
  purposeLabel,
  quantity,
  seconds,
  type Card,
} from "../board";
import {
  traceRows,
  traceSections,
  type LearnedTrace,
  type ModelTrace,
  type SearchCallTrace,
  type StepTrace,
  type ToolTrace,
  type TraceGroup,
  type TracePhase,
  type TraceRow,
  type VerifyTrace,
  type ActionTrace,
} from "../trace";
import { Icon, type IconName } from "./Icon";
import { stepLabel } from "../labels";
import detailStyles from "./DetailPanel.module.css";
import styles from "./TraceView.module.css";

interface Props {
  card: Card;
  onSummary: () => void;
}

const PHASES: Record<TracePhase, string> = {
  plan: "Plan",
  forecast: "Forecast",
  fix: "Fix",
  verify: "Verify",
  act: "Act",
};

const STEP_NAMES: Record<string, string> = {
  detect: "detect",
  plan: "plan",
  forecast: "forecast",
  merge: "merge",
  format: "gofmt",
  conflict: "conflict",
  verify: "verifier",
  explain: "note",
  learn: "remember",
  diagnose: "diagnosis",
  run: "run",
};

const STATUS: Record<string, { icon: IconName; word: string; tone: string }> = {
  running: { icon: "working", word: "running", tone: "warn" },
  done: { icon: "done", word: "done", tone: "accent" },
  failed: { icon: "blocked", word: "failed", tone: "danger" },
};

function Status({ status, live }: { status: string; live: boolean }) {
  const s = status === "running" && !live ? undefined : STATUS[status];
  if (!s) {
    return null;
  }
  return (
    <span className={styles.status} data-tone={s.tone}>
      <Icon name={s.icon} />
      {s.word}
    </span>
  );
}

function StepBody({ e, row, live }: { e: StepTrace; row: TraceRow; live: boolean }) {
  return (
    <>
      <span className={styles.what}>{STEP_NAMES[e.name] ?? e.name}</span>
      {e.name === "plan" ? null : <Status status={e.status} live={live && row.open} />}
      {e.detail ? <span className={styles.detail}>{e.detail}</span> : null}
    </>
  );
}

function ModelBody({ e, named }: { e: ModelTrace; named: boolean }) {
  return (
    <>
      <span className={detailStyles.role} data-role={e.role}>
        {e.role}
      </span>
      <span className={styles.what}>
        {e.purpose ? purposeLabel(e.purpose) : "call"}
        {e.turn ? ` · turn ${e.turn}` : ""}
      </span>
      {named && e.model ? <span className={styles.model}>{e.model}</span> : null}
      {e.said ? <q className={styles.said}>{e.said}</q> : null}
      {e.tools.length > 0 ? (
        <span className={styles.detail}>asks for {e.tools.join(", ")}</span>
      ) : e.purpose === "fix" ? (
        <span className={styles.detail}>no tool asked for</span>
      ) : null}
    </>
  );
}

function SearchBody({ e }: { e: SearchCallTrace }) {
  return (
    <>
      <span className={styles.tool}>
        <Icon name="search" />
        {providerLabel(e.provider)} search
      </span>
      {e.skipped ? (
        <span className={styles.status} data-tone="warn">
          <Icon name="wait" />
          skipped
        </span>
      ) : e.error ? (
        <span className={styles.status} data-tone="danger">
          <Icon name="blocked" />
          failed
        </span>
      ) : (
        <span className={styles.detail}>
          kept {e.results > 0 ? `${e.kept} of ` : ""}
          {quantity(e.results > 0 ? e.results : e.kept, "page", "pages")}
          {e.fresh > 0
            ? `, ${quantity(e.fresh, "new page", "new pages")}; the checked forecast is kept`
            : ""}
        </span>
      )}
      {e.query ? <q className={styles.said}>{e.query}</q> : null}
      {e.skipped || e.error ? <span className={styles.result}>{e.skipped ?? e.error}</span> : null}
    </>
  );
}

function ToolBody({ e }: { e: ToolTrace }) {
  return (
    <>
      <span className={styles.tool}>
        <Icon name="tool" />
        {e.tool}
      </span>
      {e.target ? <code className={styles.target}>{e.target}</code> : null}
      {e.ok && e.added !== undefined ? (
        <span className={styles.lines}>
          <span className={styles.add}>+{e.added}</span>{" "}
          <span className={styles.del}>&minus;{e.removed ?? 0}</span>
        </span>
      ) : null}
      {e.ok ? null : (
        <span className={styles.status} data-tone="danger">
          <Icon name="blocked" />
          {e.tool === "apply_edit" ? "refused" : "failed"}
        </span>
      )}
      {e.result && !(e.ok && e.tool === "apply_edit" && e.result === "ok") ? (
        <span className={styles.result} data-pass={e.result === "PASS"}>
          {e.result}
        </span>
      ) : null}
    </>
  );
}

export function learnedLine(e: LearnedTrace): string {
  switch (e.status) {
    case "done":
      return `Reused the fix learned from #${e.from}: 0 model calls (the first time cost ${dollars(e.savedMicros)})`;
    case "failed":
      return "the learned fix no longer applied; repairing normally";
    default:
      return `trying the fix learned from #${e.from}`;
  }
}

function LearnedBody({ e, row, live }: { e: LearnedTrace; row: TraceRow; live: boolean }) {
  return (
    <>
      <span className={styles.tool}>
        <Icon name="learned" />
        learned fix
      </span>
      {e.status === "failed" ? (
        <span className={styles.status} data-tone="warn">
          <Icon name="alert" />
          stale
        </span>
      ) : (
        <Status status={e.status} live={live && row.open} />
      )}
      <span className={styles.detail}>{learnedLine(e)}</span>
      {e.status === "failed" && e.detail ? <span className={styles.result}>{e.detail}</span> : null}
    </>
  );
}

function VerifyBody({ e }: { e: VerifyTrace }) {
  const failed = e.checks.filter((c) => !c.pass);
  return (
    <>
      <span className={styles.status} data-tone={e.passed ? "accent" : "danger"}>
        <Icon name={e.passed ? "verified" : "blocked"} />
        {e.passed ? "passed" : "failed"}
      </span>
      <span className={styles.detail}>
        {e.passed
          ? `${e.checks.length} of ${e.checks.length} checks passed`
          : `${failed.length} of ${e.checks.length} checks failed: ${failed.map((c) => c.label).join("; ")}`}
      </span>
    </>
  );
}

function ActionBody({ e }: { e: ActionTrace }) {
  return (
    <>
      <span className={styles.what}>{e.action.kind}</span>
      <Status status={e.action.status} live={false} />
      {e.action.detail ? <span className={styles.detail}>{e.action.detail}</span> : null}
    </>
  );
}

function tone(row: TraceRow, live: boolean): string {
  const e = row.entry;
  switch (e.kind) {
    case "step":
      return e.status === "failed"
        ? "danger"
        : e.status === "running" && live
          ? "warn"
          : e.name === "plan"
            ? "accent"
            : "muted";
    case "model":
      return "info";
    case "search":
      return e.error ? "danger" : e.skipped ? "warn" : "info";
    case "tool":
      return e.ok ? "info" : "danger";
    case "learned":
      return e.status === "done"
        ? "accent"
        : e.status === "failed" || (e.status === "running" && live)
          ? "warn"
          : "muted";
    case "verify":
      return e.passed ? "accent" : "danger";
    case "action":
      return e.action.status === "failed" ? "danger" : "accent";
    default:
      return "muted";
  }
}

function meta(row: TraceRow): string {
  const e = row.entry;
  if (e.kind === "model") {
    return `${seconds(e.latencyMs)} · ${e.tokens.toLocaleString("en-US")} tokens · ${dollars(e.micros)}`;
  }
  if (e.kind === "search") {
    if (e.skipped) {
      return "";
    }
    const when = e.recorded ? "recorded" : seconds(e.latencyMs);
    return e.micros > 0 ? `${when} · ${dollars(e.micros)}` : when;
  }
  return row.durationMs !== undefined ? seconds(row.durationMs) : "";
}

function Row({ row, live, named }: { row: TraceRow; live: boolean; named: boolean }) {
  const e = row.entry;
  const text = e.text;
  let body: ReactNode;
  switch (e.kind) {
    case "step":
      body = <StepBody e={e} row={row} live={live} />;
      break;
    case "model":
      body = <ModelBody e={e} named={named} />;
      break;
    case "search":
      body = <SearchBody e={e} />;
      break;
    case "tool":
      body = <ToolBody e={e} />;
      break;
    case "learned":
      body = <LearnedBody e={e} row={row} live={live} />;
      break;
    case "verify":
      body = <VerifyBody e={e} />;
      break;
    case "action":
      body = <ActionBody e={e} />;
      break;
    default:
      body = <span className={styles.detail}>{text}</span>;
  }
  const time = meta(row);
  return (
    <li data-tone={tone(row, live && row.open)} data-kind={e.kind}>
      <span className={styles.body}>{body}</span>
      {time ? <span className={styles.meta}>{time}</span> : null}
    </li>
  );
}

function Rows({ group, live }: { group: TraceGroup; live: boolean }) {
  const rows = traceRows(group);
  if (rows.length === 0) {
    return null;
  }
  let last = "";
  const named = new Set<number>();
  for (const r of rows) {
    if (r.entry.kind === "model" && r.entry.model !== last) {
      named.add(r.entry.seq);
      last = r.entry.model;
    }
  }
  return (
    <ol className={styles.events}>
      {rows.map((r) => (
        <Row key={r.entry.seq} row={r} live={live} named={named.has(r.entry.seq)} />
      ))}
    </ol>
  );
}

const ATTEMPT: Record<string, { icon: IconName; word: string }> = {
  running: { icon: "working", word: "working" },
  done: { icon: "done", word: "passed" },
  failed: { icon: "blocked", word: "failed" },
};

function Attempt({ group, live }: { group: TraceGroup; live: boolean }) {
  const status = group.status === "running" && !live ? "" : group.status;
  const s = ATTEMPT[status];
  const facts = [
    group.turns !== undefined ? `${group.turns} ${group.turns === 1 ? "turn" : "turns"}` : "",
    group.endedAt !== undefined ? seconds(group.endedAt - group.startedAt) : "",
    group.micros > 0 ? dollars(group.micros) : "",
  ].filter(Boolean);
  return (
    <li className={styles.attempt} data-status={status}>
      <div className={styles.attemptHead}>
        <h4 className={styles.attemptTitle}>
          {s ? <Icon name={s.icon} /> : null}
          Attempt {group.attempt}
          {group.of ? ` of ${group.of}` : ""}
          {s ? <span className={styles.attemptWord}>{s.word}</span> : null}
        </h4>
        {facts.length > 0 ? <span className={styles.meta}>{facts.join(" · ")}</span> : null}
        {group.status === "failed" && (group.stop || group.failure) ? (
          <p className={styles.why}>
            {group.stop ? <span>{group.stop}</span> : null}
            {group.failure ? <code className={styles.failure}>{group.failure}</code> : null}
          </p>
        ) : null}
      </div>
      <Rows group={group} live={live} />
    </li>
  );
}

export function TraceView({ card, onSummary }: Props) {
  const live = card.phase === "working" || card.phase === "verified";
  const sections = traceSections(card.trace);
  return (
    <div className={styles.trace}>
      {sections.length === 0 ? (
        <p className={styles.empty}>
          {card.phase === "done"
            ? "This pull request finished before the page was opened, so there is no live trace. The summary has the result."
            : card.phase === "queued"
              ? "Waiting for its turn. Each model call, tool call and attempt appears here as Upkeep works."
              : "Starting."}
        </p>
      ) : (
        sections.map((section) => (
          <section key={section.phase} className={styles.phase}>
            <h3 className={styles.heading}>{PHASES[section.phase]}</h3>
            {section.groups.some((g) => g.attempt) ? (
              <ol className={styles.attempts}>
                {section.groups.map((g) =>
                  g.attempt ? (
                    <Attempt key={g.entries[0]?.seq} group={g} live={live} />
                  ) : (
                    <li key={g.entries[0]?.seq} className={styles.loose}>
                      <Rows group={g} live={live} />
                    </li>
                  ),
                )}
              </ol>
            ) : (
              section.groups.map((g) => <Rows key={g.entries[0]?.seq} group={g} live={live} />)
            )}
          </section>
        ))
      )}
      {live && card.step ? (
        <p className={styles.now}>
          <Icon name="working" />
          {stepLabel(card.step.name, card.step.status, card.step.detail)}
        </p>
      ) : null}
      {card.phase === "done" ? (
        <p className={styles.end} data-outcome={card.outcome ?? ""}>
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
          />
          <span>
            Finished: {outcomeLabel(card.outcome) || "no outcome"}
            {card.reason ? ` · ${card.reason}` : ""}.
          </span>
          <button type="button" className={styles.link} onClick={onSummary}>
            See the summary
          </button>
        </p>
      ) : null}
    </div>
  );
}
