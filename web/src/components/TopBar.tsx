import type { Connection } from "../board";
import type { Info } from "../types";
import { Mark } from "./Mark";
import { StatusBadge } from "./StatusBadge";
import styles from "./TopBar.module.css";

interface Props {
  connection: Connection;
  version: string | null;
  info: Info | null;
  running: boolean;
}

const MODES = {
  replay: { label: "Replay", hint: "A recorded run: real Nemotron answers, no tokens spent now." },
  live: { label: "Live", hint: "Calls Nemotron on Nebius Token Factory right now." },
  record: { label: "Recording", hint: "Calls Nemotron and saves every answer for replay." },
} as const;

export function TopBar({ connection, version, info, running }: Props) {
  const mode = info ? MODES[info.mode] : null;
  return (
    <header className={styles.bar}>
      <div className={styles.inner}>
        <div className={styles.brand}>
          <a className={styles.home} href="/" aria-label="Upkeep, back to the overview">
            <Mark className={styles.mark} running={running} />
            <span className={styles.name}>Upkeep</span>
          </a>
          <span className={styles.scope}>
            <code>upkeep-sample</code>
          </span>
        </div>
        <div className={styles.right}>
          {mode ? (
            <span className={styles.mode} title={mode.hint} data-mode={info?.mode}>
              {mode.label}
            </span>
          ) : null}
          {info?.mode === "live" && info.daily_tokens > 0 ? (
            <span
              className={styles.budget}
              title="Tokens spent today by live runs, out of the daily limit"
            >
              {Math.round(info.tokens_today / 1000)}k / {Math.round(info.daily_tokens / 1000)}k
              tokens today
            </span>
          ) : null}
          <StatusBadge status={connection} version={version} />
        </div>
      </div>
    </header>
  );
}
