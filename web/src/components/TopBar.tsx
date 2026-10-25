import type { Connection } from "../board";
import type { Info } from "../types";
import { Icon } from "./Icon";
import { StatusBadge } from "./StatusBadge";
import styles from "./TopBar.module.css";

interface Props {
  connection: Connection;
  version: string | null;
  info: Info | null;
}

const MODES = {
  replay: { label: "Replay", hint: "A recorded run: real Nemotron answers, no tokens spent now." },
  live: { label: "Live", hint: "Calls Nemotron on Nebius Token Factory right now." },
  record: { label: "Recording", hint: "Calls Nemotron and saves every answer for replay." },
} as const;

export function TopBar({ connection, version, info }: Props) {
  const mode = info ? MODES[info.mode] : null;
  return (
    <header className={styles.bar}>
      <div className={styles.brand}>
        <Icon name="mark" className={styles.mark} />
        <span className={styles.name}>Upkeep</span>
      </div>
      <p className={styles.tagline}>Keeps your pull requests mergeable.</p>
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
            {Math.round(info.tokens_today / 1000)}k / {Math.round(info.daily_tokens / 1000)}k tokens
            today
          </span>
        ) : null}
        <StatusBadge status={connection} version={version} />
      </div>
    </header>
  );
}
