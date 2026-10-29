import { STAGES } from "../progress";
import styles from "./Onboarding.module.css";

const TEXT: Record<string, string> = {
  check: "Behind main, a conflict, a broken build, a failing test or bad formatting.",
  forecast:
    "Tavily finds the migration guide and the changelog. Nemotron Super reads them before it touches a dependency bump.",
  fix: "Nemotron proposes the change inside tight limits. Majors and large conflicts wait for a person.",
  verify: "The compiler and the tests decide, outside the model.",
  act: "Only then does Upkeep push, attach a patch or merge, and it leaves a receipt.",
};

export function Onboarding() {
  return (
    <section className={styles.panel}>
      <header className={styles.head}>
        <h2 className={styles.title}>Every pull request goes through five stages</h2>
        <p className={styles.sub}>The live stage lights them up one by one as the agent works.</p>
      </header>
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
