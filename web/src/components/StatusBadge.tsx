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

function label(status: Connection, version: string | null): string {
  switch (status) {
    case "connecting":
      return "connecting";
    case "live":
      return version ? `live · ${version}` : "live";
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
