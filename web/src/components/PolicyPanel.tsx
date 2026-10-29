import type { ReactNode } from "react";
import type { Info } from "../types";
import { Icon, type IconName } from "./Icon";
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

type Kind = "never" | "conditions" | "limits";

const GROUPS: { kind: Kind; title: string; hint: string; icon: IconName }[] = [
  { kind: "never", title: "Never", hint: "No setting turns these on.", icon: "blocked" },
  { kind: "conditions", title: "Only when", hint: "Allowed with a condition.", icon: "verified" },
  { kind: "limits", title: "Within limits", hint: "Past these it hands back.", icon: "wait" },
];

function kindOf(rule: string): Kind {
  if (/at most|stops after/i.test(rule)) {
    return "limits";
  }
  if (/^never|may never/i.test(rule)) {
    return "never";
  }
  return "conditions";
}

function emphasis(rule: string): ReactNode {
  const parts = rule.split(/(\d+|"[^"]+")/g);
  if (parts.length === 1) {
    return rule;
  }
  return parts.map((part, i) =>
    i % 2 === 1 ? (
      <strong key={i} className={styles.figure}>
        {part}
      </strong>
    ) : (
      part
    ),
  );
}

export function PolicyPanel({ info, onClose }: Props) {
  const rules = info?.rules && info.rules.length > 0 ? info.rules : ALWAYS;
  return (
    <Overlay
      title="What Upkeep may do"
      subtitle="The policy this server runs with. A model proposes; code enforces."
      onClose={onClose}
    >
      <div className={styles.groups}>
        {GROUPS.map((group) => {
          const items = rules.filter((rule) => kindOf(rule) === group.kind);
          if (items.length === 0) {
            return null;
          }
          return (
            <section key={group.kind} className={styles.group} data-kind={group.kind}>
              <header className={styles.head}>
                <Icon name={group.icon} className={styles.icon} />
                <h3 className={styles.title}>{group.title}</h3>
                <span className={styles.hint}>{group.hint}</span>
              </header>
              <ul className={styles.list}>
                {items.map((rule) => (
                  <li key={rule} className={styles.rule}>
                    {emphasis(rule)}
                  </li>
                ))}
              </ul>
            </section>
          );
        })}
      </div>
      <p className={styles.note}>
        Every change is checked by the compiler, vet and the tests outside the model, and by a
        verifier that counts files, lines, comments and test edits. When any check fails nothing is
        pushed and the pull request is handed back with a diagnosis.
      </p>
    </Overlay>
  );
}
