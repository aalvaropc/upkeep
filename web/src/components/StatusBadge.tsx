import type { StreamStatus } from "../events";
import styles from "./StatusBadge.module.css";

interface Props {
  status: StreamStatus;
  version: string | null;
}

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
      {label(status, version)}
    </span>
  );
}
