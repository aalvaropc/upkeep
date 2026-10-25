import { Icon, type IconName } from "./Icon";
import styles from "./Onboarding.module.css";

const STEPS: { icon: IconName; title: string; text: string }[] = [
  {
    icon: "blocked",
    title: "Find what blocks it",
    text: "Behind main, a conflict, a broken build, a failing test or bad formatting.",
  },
  {
    icon: "working",
    title: "Fix what the policy allows",
    text: "Nemotron on Nebius proposes the change inside tight limits. Majors and large conflicts wait for a person.",
  },
  {
    icon: "verified",
    title: "Prove it outside the model",
    text: "The compiler and the tests decide. Only then does Upkeep push, attach a patch or merge, and it leaves a receipt.",
  },
];

export function Onboarding() {
  return (
    <ol className={styles.steps} aria-label="How it works">
      {STEPS.map((step, i) => (
        <li key={step.title} className={styles.step}>
          <Icon name={step.icon} className={styles.icon} />
          <div>
            <h3 className={styles.title}>
              {i + 1}. {step.title}
            </h3>
            <p className={styles.text}>{step.text}</p>
          </div>
        </li>
      ))}
    </ol>
  );
}
