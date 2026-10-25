import type { StreamStatus } from "../events";
import { Icon, type IconName } from "./Icon";
import styles from "./StatusBadge.module.css";

interface Props {
  status: StreamStatus;
  version: string | null;
}

const ICONS: Record<StreamStatus, IconName> = {
  connecting: "connecting",
  live: "live",
  lost: "lost",
};

function label(status: StreamStatus, version: string | null): string {
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
