import type { Info } from "../types";
import { Icon } from "./Icon";
import { Overlay } from "./Overlay";
import styles from "./PolicyPanel.module.css";

interface Props {
  info: Info | null;
  onClose: () => void;
}

const ALWAYS = [
  "Never force-pushes: main is merged into the branch and history is kept.",
  "Never merges a pull request opened by a person.",
  "A fix may never edit, delete or skip tests.",
];

export function PolicyPanel({ info, onClose }: Props) {
  const rules = info?.rules && info.rules.length > 0 ? info.rules : ALWAYS;
  return (
    <Overlay
      title="What Upkeep may do"
      subtitle="The policy this server runs with. A model proposes; code enforces."
      onClose={onClose}
    >
      <ul className={styles.list}>
        {rules.map((rule) => (
          <li key={rule} className={styles.rule}>
            <Icon name="verified" className={styles.icon} />
            <span>{rule}</span>
          </li>
        ))}
      </ul>
      <p className={styles.note}>
        Every change is checked by the compiler, vet and the tests outside the model, and by a
        verifier that counts files, lines, comments and test edits. When any check fails nothing is
        pushed and the pull request is handed back with a diagnosis.
      </p>
    </Overlay>
  );
}
