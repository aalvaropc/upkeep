import { Icon } from "./Icon";
import motion from "../motion.module.css";
import styles from "./Models.module.css";

const ROSTER = [
  {
    tier: "Nano",
    size: "30B",
    role: "Reads release notes and writes the forecast",
  },
  {
    tier: "Super",
    size: "120B",
    role: "Fixes the code with tools, up to three attempts",
  },
  {
    tier: "Ultra",
    size: "550B",
    role: "Resolves small conflicts and writes the diagnosis when a person is needed",
  },
] as const;

export function Models() {
  return (
    <section className={`${styles.panel} ${motion.enter}`} aria-label="Models">
      <h2 className={styles.title}>Models</h2>
      <ul className={styles.list}>
        {ROSTER.map((m) => (
          <li key={m.tier} className={styles.row}>
            <Icon name="chip" className={styles.icon} />
            <div className={styles.body}>
              <p className={styles.name}>
                <span>Nemotron {m.tier}</span>
                <span className={styles.size}>{m.size}</span>
              </p>
              <p className={styles.role}>{m.role}</p>
              <p className={styles.meta}>no calls yet</p>
            </div>
          </li>
        ))}
      </ul>
    </section>
  );
}
