import { Icon } from "./Icon";
import styles from "./Sidebar.module.css";

interface Props {
  onRules: () => void;
}

const DOCS = [
  {
    href: "https://github.com/aalvaropc/upkeep/blob/main/docs/architecture.md",
    label: "How it works",
  },
  {
    href: "https://github.com/aalvaropc/upkeep/blob/main/docs/models.md",
    label: "Which model does what",
  },
  {
    href: "https://github.com/aalvaropc/upkeep/blob/main/docs/evals/live.md",
    label: "Live evaluation",
  },
  {
    href: "https://github.com/aalvaropc/upkeep/blob/main/docs/limitations.md",
    label: "Limitations",
  },
  { href: "https://github.com/aalvaropc/upkeep", label: "Source" },
];

export function Sidebar({ onRules }: Props) {
  return (
    <aside className={styles.side}>
      <div className={styles.brand}>
        <span className={styles.mark} aria-hidden="true">
          <Icon name="mark" />
        </span>
        <span className={styles.names}>
          <span className={styles.name}>Upkeep</span>
          <span className={styles.scope}>
            <code>upkeep-sample</code> · demo repository
          </span>
        </span>
      </div>
      <nav className={styles.nav} aria-label="Upkeep">
        <a className={styles.item} href="#board" aria-current="page">
          <Icon name="merge" className={styles.icon} />
          Pull request queue
        </a>
        <button type="button" className={styles.item} onClick={onRules}>
          <Icon name="rules" className={styles.icon} />
          Policy and rules
        </button>
      </nav>
      <nav className={styles.docs} aria-label="Project links">
        <p className={styles.heading}>Read more</p>
        {DOCS.map((d) => (
          <a
            key={d.href}
            className={styles.link}
            href={d.href}
            target="_blank"
            rel="noreferrer noopener"
          >
            {d.label}
          </a>
        ))}
      </nav>
      <p className={styles.foot}>
        NVIDIA Nemotron on Nebius Token Factory, with Tavily search. MIT licence.
      </p>
    </aside>
  );
}
