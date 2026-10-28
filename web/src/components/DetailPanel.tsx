import { useEffect, useState } from "react";
import { dollars, outcomeLabel, seconds } from "../board";
import type { Detail, Forecast } from "../types";
import { DiffView } from "./DiffView";
import { Icon } from "./Icon";
import { Overlay } from "./Overlay";
import { blockerLabel } from "./PullCard";
import styles from "./DetailPanel.module.css";

interface Props {
  number: number;
  load: (number: number) => Promise<Detail>;
  onClose: () => void;
}

const MAX_FAILURE = 3000;

export function isWebAddress(address: string): boolean {
  return /^https?:\/\//i.test(address);
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <section className={styles.section}>
      <h3 className={styles.heading}>{title}</h3>
      {children}
    </section>
  );
}

function ForecastView({
  forecast,
  confirmed,
  missed,
}: {
  forecast: Forecast;
  confirmed: string[];
  missed: string[];
}) {
  return (
    <>
      <p className={styles.lead}>
        Super read the release notes and the migration guide for {forecast.module} {forecast.from}{" "}
        to {forecast.to} before any fix, and predicted what would break. The compiler then said
        whether it did.
      </p>
      {forecast.breaking.length === 0 ? (
        <p className={styles.muted}>
          It predicted nothing specific. The fix started from the build output alone.
        </p>
      ) : (
        <ul className={styles.predictions}>
          {forecast.breaking.map((b) => {
            const seen = confirmed.includes(b.symbol);
            const unseen = missed.includes(b.symbol);
            return (
              <li key={b.symbol} className={styles.prediction}>
                <code>{b.symbol}</code>
                <span className={styles.change}>{b.description}</span>
                {seen ? (
                  <span className={styles.seen}>
                    <Icon name="done" /> confirmed by the build
                  </span>
                ) : unseen ? (
                  <span className={styles.unseen}>not seen in the build</span>
                ) : null}
              </li>
            );
          })}
        </ul>
      )}
      {forecast.dropped > 0 ? (
        <p className={styles.muted}>
          {forecast.dropped} predicted {forecast.dropped === 1 ? "change was" : "changes were"}{" "}
          dropped because the name does not appear in the sources.
        </p>
      ) : null}
      {forecast.sources.some(isWebAddress) ? (
        <ul className={styles.sources}>
          {forecast.sources
            .filter(isWebAddress)
            .slice(0, 4)
            .map((s) => (
              <li key={s}>
                <a href={s} target="_blank" rel="noreferrer noopener">
                  {s.replace(/^https?:\/\//, "")}
                </a>
              </li>
            ))}
        </ul>
      ) : null}
    </>
  );
}

function Body({ d }: { d: Detail }) {
  const lines = d.receipt?.lines ?? [];
  const steps = d.receipt?.steps ?? [];
  const saved =
    d.receipt && d.receipt.baseline_micros > d.receipt.micros
      ? d.receipt.baseline_micros / Math.max(d.receipt.micros, 1)
      : 0;
  return (
    <div className={styles.body}>
      <div className={styles.summary}>
        <span className={styles.outcome} data-outcome={d.outcome}>
          {outcomeLabel(d.outcome) || "Not worked yet"}
        </span>
        {d.blocker ? <span className={styles.chip}>{blockerLabel(d.blocker)}</span> : null}
        {d.duration_ms ? <span className={styles.chip}>{seconds(d.duration_ms)}</span> : null}
        {d.attempts > 0 ? (
          <span className={styles.chip}>
            {d.attempts} {d.attempts === 1 ? "attempt" : "attempts"}
          </span>
        ) : null}
        {d.receipt && d.receipt.calls > 0 ? (
          <span className={styles.chip}>
            {d.receipt.calls} model calls · {dollars(d.receipt.micros)}
          </span>
        ) : null}
      </div>

      {d.summary ? (
        <Section title="What changed">
          <p className={styles.lead}>{d.summary}</p>
          {d.learned ? (
            <p className={styles.muted}>
              Reused the fix learned from #{d.learned.from} in the previous run: 0 model calls (the
              first time cost {dollars(d.learned.saved_micros)}).
            </p>
          ) : null}
        </Section>
      ) : null}

      {d.reason && d.outcome !== "mergeable" && d.outcome !== "merged" ? (
        <Section title="Why it stopped">
          <p className={styles.lead}>{d.reason}</p>
        </Section>
      ) : null}

      {d.diagnosis ? (
        <Section title="Diagnosis for a person">
          <p className={styles.lead}>{d.diagnosis}</p>
        </Section>
      ) : null}

      {d.actions && d.actions.length > 0 ? (
        <Section title="What Upkeep did">
          <ul className={styles.actions}>
            {d.actions.map((a) => (
              <li key={a.kind} data-status={a.status}>
                <Icon name={a.status === "done" ? "verified" : "alert"} />
                <span>
                  <strong>{a.kind}</strong> {a.detail}
                </span>
              </li>
            ))}
          </ul>
        </Section>
      ) : null}

      {d.forecast ? (
        <Section title="Forecast">
          <ForecastView
            forecast={d.forecast}
            confirmed={d.confirmed ?? []}
            missed={d.missed ?? []}
          />
        </Section>
      ) : null}

      {d.failure ? (
        <Section title="What was failing">
          <details className={styles.details}>
            <summary>Build or test output</summary>
            <pre className={styles.pre} tabIndex={0}>
              {d.failure.length > MAX_FAILURE
                ? `${d.failure.slice(0, MAX_FAILURE)}\n... (cut)`
                : d.failure}
            </pre>
          </details>
        </Section>
      ) : null}

      {d.patch ? (
        <Section title="The change">
          {d.changes && d.changes.length > 0 ? (
            <ul className={styles.files}>
              {d.changes.map((c) => (
                <li key={c.path}>
                  <code>{c.path}</code>
                  <span className={styles.add}>+{c.added}</span>
                  <span className={styles.del}>-{c.deleted}</span>
                </li>
              ))}
            </ul>
          ) : null}
          <DiffView patch={d.patch} />
        </Section>
      ) : null}

      {d.flaky && d.flaky.length > 0 ? (
        <Section title="Flaky tests">
          <p className={styles.lead}>
            {d.flaky.join(", ")} failed in the full run and passed when run alone. Upkeep reported{" "}
            {d.flaky.length === 1 ? "it" : "them"} and did not touch{" "}
            {d.flaky.length === 1 ? "it" : "them"}; a failure that can pass on a rerun is not a fix
            to hide.
          </p>
        </Section>
      ) : null}

      {d.checklist && d.checklist.length > 0 ? (
        <Section title="Verifier">
          <ul className={styles.checks}>
            {d.checklist.map((c) => (
              <li key={c.id} data-pass={c.pass}>
                <Icon name={c.pass ? "done" : "blocked"} />
                <span>
                  {c.label}
                  {c.detail ? <em> · {c.detail}</em> : null}
                </span>
              </li>
            ))}
          </ul>
        </Section>
      ) : null}

      {d.receipt ? (
        <Section title="Receipt">
          {lines.length === 0 ? (
            <p className={styles.muted}>No model was called; the checks were enough.</p>
          ) : (
            <div className={styles.tableWrap}>
              <table className={styles.table}>
                <thead>
                  <tr>
                    <th scope="col">Model</th>
                    <th scope="col">Calls</th>
                    <th scope="col">Tokens</th>
                    <th scope="col">Time</th>
                    <th scope="col">Cost</th>
                  </tr>
                </thead>
                <tbody>
                  {lines.map((l) => (
                    <tr key={l.model}>
                      <th scope="row">
                        <span className={styles.role} data-role={l.role}>
                          {l.role}
                        </span>{" "}
                        <span className={styles.modelName}>{l.model}</span>
                      </th>
                      <td>{l.calls}</td>
                      <td>{(l.prompt_tokens + l.completion_tokens).toLocaleString("en-US")}</td>
                      <td>{seconds(l.latency_ms)}</td>
                      <td>{dollars(l.micros)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          {saved > 0 ? (
            <p className={styles.muted}>
              The same tokens at the largest model&rsquo;s price:{" "}
              {dollars(d.receipt.baseline_micros)} ({saved.toFixed(1)} times as much). A price
              comparison, not a claim that it would have fixed this.
            </p>
          ) : null}
          {steps.length > 0 ? (
            <ol className={styles.steps}>
              {steps.map((s, i) => (
                <li key={i} data-ok={s.ok}>
                  <span className={styles.stepName}>{s.name}</span>
                  <span className={styles.stepDetail}>{s.detail}</span>
                  <span className={styles.stepTime}>{seconds(s.duration_ms)}</span>
                </li>
              ))}
            </ol>
          ) : null}
        </Section>
      ) : null}
    </div>
  );
}

export function DetailPanel({ number, load, onClose }: Props) {
  const [detail, setDetail] = useState<Detail | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setDetail(null);
    setError(null);
    load(number).then(
      (d) => {
        if (!cancelled) {
          setDetail(d);
        }
      },
      (err: unknown) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "The details could not be loaded.");
        }
      },
    );
    return () => {
      cancelled = true;
    };
  }, [load, number]);

  const who = detail
    ? `${detail.author.login} · ${detail.branch}${detail.bump ? ` · ${detail.bump.module} ${detail.bump.from} to ${detail.bump.to} (${detail.bump.level})` : ""}`
    : undefined;
  return (
    <Overlay
      side
      title={detail ? `#${detail.number} ${detail.title}` : `Pull request #${number}`}
      subtitle={who}
      onClose={onClose}
    >
      {error ? (
        <p className={styles.error} role="alert">
          {error}
        </p>
      ) : detail ? (
        <Body d={detail} />
      ) : (
        <p className={styles.muted}>Loading the details.</p>
      )}
    </Overlay>
  );
}
