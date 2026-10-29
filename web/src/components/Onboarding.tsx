import { STAGES } from "../progress";
import styles from "./Onboarding.module.css";

const TEXT: Record<string, string> = {
  check: "Behind main, a conflict, a broken build, a failing test.",
  forecast: "Tavily finds the migration guide. Nemotron Super reads it first.",
  fix: "Nemotron edits inside tight limits. Majors wait for a person.",
  verify: "The compiler and the tests decide, outside the model.",
  act: "Push, patch or merge, and a receipt.",
};

export function Onboarding() {
  return (
    <section className={styles.panel}>
      <h2 className={styles.title}>Five stages per pull request</h2>
      <ol className={styles.steps} aria-label="How it works">
        {STAGES.map((stage) => (
          <li key={stage.id} className={styles.step}>
            <span className={styles.bar} aria-hidden="true" />
            <h3 className={styles.name}>{stage.label}</h3>
            <p className={styles.text}>{TEXT[stage.id]}</p>
          </li>
        ))}
      </ol>
    </section>
  );
}
