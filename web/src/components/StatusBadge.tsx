import type { Connection } from "../board";
import { Icon, type IconName } from "./Icon";
import styles from "./StatusBadge.module.css";

interface Props {
  status: Connection;
  version: string | null;
}

const ICONS: Record<Connection, IconName> = {
  connecting: "connecting",
  live: "live",
  lost: "lost",
};

export function shortVersion(version: string): string {
  const m = /^(v\d+\.\d+\.\d+)-(?:0\.)?\d{14}-([0-9a-f]{7})[0-9a-f]*(\+dirty)?$/.exec(version);
  return m ? `${m[1]} · ${m[2]}${m[3] ? " dirty" : ""}` : version;
}

function label(status: Connection, version: string | null): string {
  switch (status) {
    case "connecting":
      return "connecting";
    case "live":
      return version ? `live · ${shortVersion(version)}` : "live";
    case "lost":
      return "reconnecting";
  }
}

export function StatusBadge({ status, version }: Props) {
  return (
    <span className={styles.badge} data-state={status} role="status" aria-live="polite">
      <Icon name={ICONS[status]} className={styles.icon} />
      <span className={styles.text}>{label(status, version)}</span>
    </span>
  );
}
