import { useEffect, useState } from "react";
import { dollars, outcomeLabel, providerLabel, quantity, seconds, type Card } from "../board";
import type { Detail, Forecast, PullRequest } from "../types";
import { DiffView } from "./DiffView";
import { Icon } from "./Icon";
import { Overlay } from "./Overlay";
import { blockerLabel } from "./PullCard";
import { Tabs } from "./Tabs";
import { TraceView } from "./TraceView";
import styles from "./DetailPanel.module.css";

interface Props {
  number: number;
  card?: Card;
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
  const search = forecast.search;
  const provider = providerLabel(search?.provider || "tavily");
  const releases =
    forecast.releases ?? forecast.sources.filter((s) => s.includes("/releases/tag/"));
  const pages = forecast.sources.filter((s) => isWebAddress(s) && !releases.includes(s));
  const read = [
    releases.length > 0 ? quantity(releases.length, "release note", "release notes") : "",
    pages.length > 0 ? `${quantity(pages.length, "page", "pages")} ${provider} found` : "",
  ]
    .filter(Boolean)
    .join(" and ");
  const when = search?.recorded
    ? "recorded"
    : search?.at
      ? `${search.at.slice(0, 16).replace("T", " ")} UTC`
      : "";
  const linked = [
    ...pages.map((url) => ({ url, kind: "search" as const })),
    ...releases
      .filter(isWebAddress)
      .slice(0, 4)
      .map((url) => ({ url, kind: "release" as const })),
  ];
  const moreReleases = releases.filter(isWebAddress).length - 4;
  return (
    <>
      <p className={styles.lead}>
        Super read {read || "the release notes and the migration guide"} for {forecast.module}{" "}
        {forecast.from} to {forecast.to} before any fix, and predicted what would break. The
        compiler then said whether it did.
      </p>
      {search ? (
        <p className={styles.searchLine}>
          <Icon name="search" />
          <span>
            {provider} searched <code>“{search.query}”</code>
            {when ? ` · ${when}` : ""}
            {search.latency_ms ? ` · ${seconds(search.latency_ms)}` : ""}
            {search.skipped
              ? ` · not asked this time: ${search.skipped}`
              : search.error
                ? ` · failed this time: ${search.error}`
                : search.reused
                  ? " · no new page, so the forecast was reused"
                  : ""}
          </span>
        </p>
      ) : null}
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
      {linked.length > 0 ? (
        <ul className={styles.sources}>
          {linked.map(({ url, kind }) => (
            <li key={url} className={styles.source}>
              <span className={styles.sourceKind} data-kind={kind}>
                <Icon name={kind === "search" ? "search" : "patch"} />
                {kind === "search" ? "web search" : "release notes"}
              </span>
              <a href={url} target="_blank" rel="noreferrer noopener">
                {url.replace(/^https?:\/\//, "")}
              </a>
            </li>
          ))}
        </ul>
      ) : null}
      {moreReleases > 0 ? (
        <p className={styles.muted}>
          and {quantity(moreReleases, "more release note", "more release notes")}
        </p>
      ) : null}
      {forecast.releases_error ? (
        <p className={styles.muted}>
          The release notes could not be read from GitHub: {forecast.releases_error}
        </p>
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
        </Section>
      ) : null}

      {d.confidence ? (
        <Section title="Merge confidence">
          <p className={styles.confidence} data-level={d.confidence.level}>
            <Icon name="gauge" className={styles.confidenceIcon} />
            {d.confidence.level}
          </p>
          <ul className={styles.reasons}>
            {d.confidence.reasons.map((r) => (
              <li key={r}>{r}</li>
            ))}
          </ul>
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

function byline(pr: PullRequest): string {
  const bump = pr.bump
    ? ` · ${pr.bump.module} ${pr.bump.from} to ${pr.bump.to} (${pr.bump.level})`
    : "";
  return `${pr.author.login} · ${pr.branch}${bump}`;
}

export function DetailPanel({ number, card, load, onClose }: Props) {
  const [loaded, setLoaded] = useState<{ number: number; detail: Detail } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState(
    card?.phase === "working" || card?.phase === "verified" ? "trace" : "summary",
  );
  const [finishedAt, setFinishedAt] = useState(card?.finishedAt);
  if (card?.finishedAt !== undefined && card.finishedAt !== finishedAt) {
    setFinishedAt(card.finishedAt);
  }

  useEffect(() => {
    let cancelled = false;
    setError(null);
    load(number).then(
      (d) => {
        if (!cancelled) {
          setLoaded({ number, detail: d });
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
  }, [load, number, finishedAt]);

  const detail = loaded?.number === number ? loaded.detail : null;
  const pr: PullRequest | undefined = detail ?? card?.pr;
  const summary =
    error && !detail ? (
      <p className={styles.error} role="alert">
        {error}
      </p>
    ) : detail ? (
      <>
        {error ? (
          <p className={styles.error} role="alert">
            {error}
          </p>
        ) : null}
        <Body d={detail} />
      </>
    ) : (
      <p className={styles.muted}>Loading the details.</p>
    );
  return (
    <Overlay
      side
      title={pr ? `#${pr.number} ${pr.title}` : `Pull request #${number}`}
      subtitle={pr ? byline(pr) : undefined}
      onClose={onClose}
    >
      {card ? (
        <Tabs
          label="Pull request details"
          selected={tab}
          onSelect={setTab}
          tabs={[
            {
              id: "trace",
              label: "Trace",
              content: <TraceView card={card} onSummary={() => setTab("summary")} />,
            },
            { id: "summary", label: "Summary", content: summary },
          ]}
        />
      ) : (
        summary
      )}
    </Overlay>
  );
}
