import type { ReactNode } from "react";
import { Icon } from "./Icon";
import styles from "./Landing.module.css";

const REPO = "https://github.com/aalvaropc/upkeep";
const DOCS = `${REPO}/blob/main/docs`;
const SAMPLE = "https://github.com/aalvaropc/upkeep-sample";

type Hue = "ultra" | "super" | "nano" | "search" | "tool";

interface Use {
  label: string;
  hue: Hue;
}

interface Step {
  name: string;
  text: string;
  uses: Use[];
}

const STEPS: Step[] = [
  {
    name: "Plan",
    text: "Finds which open pull requests change the same files and which pairs conflict, then suggests a merge order. After the run it flags what went stale.",
    uses: [{ label: "git merge-tree", hue: "tool" }],
  },
  {
    name: "Forecast",
    text: "For a dependency bump, finds the migration guide and predicts what will break in the identifiers this project uses. The compiler later confirms each prediction or not.",
    uses: [
      { label: "Tavily", hue: "search" },
      { label: "Nemotron Super", hue: "super" },
      { label: "the compiler", hue: "tool" },
    ],
  },
  {
    name: "Fix",
    text: "Merges and formatting are mechanical. Small conflicts go to Ultra in one call. Broken builds and tests go to Super, which works with four tools for up to three fresh attempts.",
    uses: [
      { label: "Nemotron Ultra", hue: "ultra" },
      { label: "Nemotron Super", hue: "super" },
      { label: "gofmt", hue: "tool" },
    ],
  },
  {
    name: "Verify",
    text: "Outside the model. Build, vet and tests pass in a container with no network. At most 5 files and 80 lines. No test touched, no comment added, formatted.",
    uses: [
      { label: "go build, vet, test", hue: "tool" },
      { label: "a sandbox with no network", hue: "tool" },
    ],
  },
  {
    name: "Act",
    text: "Pushes to a bot's branch or one labelled upkeep, otherwise attaches a patch. Merges a bot's patch or minor bump when every check passes. Anything else gets a diagnosis for a person. Every pull request gets a receipt with each model's calls and cost, and a merge confidence.",
    uses: [
      { label: "GitHub", hue: "tool" },
      { label: "Nemotron Ultra", hue: "ultra" },
      { label: "Nemotron Nano", hue: "nano" },
    ],
  },
  {
    name: "Learn",
    text: "A verified fix is remembered. When the same failure returns, the fix is reapplied with zero model calls, and it still goes through every check.",
    uses: [{ label: "zero model calls", hue: "tool" }],
  },
];

interface Finding {
  claim: ReactNode;
  source: string;
  href: string;
}

const FINDINGS: Finding[] = [
  {
    claim: (
      <>
        About <strong>one merge in five</strong> conflicts, and conflicted code is about{" "}
        <strong>twice as likely</strong> to carry a bug.
      </>
    ),
    source: "Brindescu et al., Empirical Software Engineering, 2019",
    href: "https://doi.org/10.1007/s10664-019-09735-4",
  },
  {
    claim: (
      <>
        <strong>11.3%</strong> of projects using Dependabot dropped it, and developers tune it to
        get fewer pull requests.
      </>
    ),
    source: "He et al., IEEE Transactions on Software Engineering",
    href: "https://arxiv.org/abs/2206.07230",
  },
  {
    claim: (
      <>
        Unmerged Dependabot security pull requests are{" "}
        <strong>mostly blocked by concurrent changes</strong>.
      </>
    ),
    source: "Alfadel et al., MSR 2021",
    href: "https://2021.msrconf.org/details/msr-2021-technical-papers/25/On-the-Use-of-Dependabot-Security-Pull-Requests",
  },
  {
    claim: (
      <>
        Flaky tests take <strong>at least 2.5%</strong> of productive developer time.
      </>
    ),
    source: "Leinen et al., ICST 2024",
    href: "https://conf.researchr.org/details/icst-2024/icst-2024-industry/1/Cost-of-Flaky-Tests-in-CI-An-Industrial-Case-Study",
  },
];

const BLOCKERS = [
  "behind main",
  "a one-line conflict",
  "a rename that broke the build",
  "a bump that no longer compiles",
  "gofmt",
  "a flaky test",
];

const RULES = [
  "Never force-pushes.",
  "Never merges a pull request opened by a person.",
  "Never edits, deletes or skips a test.",
  "Changes at most 5 files and 80 lines.",
  "Stops after 3 attempts.",
];

interface Model {
  name: string;
  size: string;
  hue: Hue;
  text: string;
}

const MODELS: Model[] = [
  {
    name: "Nemotron Ultra",
    size: "550B",
    hue: "ultra",
    text: "Resolves small conflicts in one call: 3 of 3. Inside a tool loop it fixed 0 of 21 builds, so it never gets that job.",
  },
  {
    name: "Nemotron Super",
    size: "120B",
    hue: "super",
    text: "The agent. About 65% per attempt and about 96% over three. Asked three times, it gave the same forecast three times.",
  },
  {
    name: "Nemotron Nano",
    size: "30B",
    hue: "nano",
    text: "Writes the reviewer's note, and the note is checked against the diff.",
  },
];

interface Stat {
  value: string;
  text: string;
}

const STATS: Stat[] = [
  {
    value: "9 of 9",
    text: "The recorded nine-pull-request run, replayed in CI on every change.",
  },
  {
    value: "72 of 72",
    text: "Across eight live runs: 45 of 45 in five runs at $0.020 to $0.037 a run, then 27 of 27 in three more at $0.0230 a run after the latest features.",
  },
  {
    value: "$0.0191",
    text: "The recorded run, against $0.0502 for the same tokens at the largest model's price. A price comparison, not a claim the largest model would have fixed them.",
  },
  {
    value: "$0.0055",
    text: "Run again: the same queue reuses its learned fixes, with 3 model calls.",
  },
];

interface Tier {
  name: string;
  status: string;
  planned: boolean;
  points: string[];
}

const TIERS: Tier[] = [
  {
    name: "Open source core",
    status: "Available now · MIT",
    planned: false,
    points: [
      "The CLI and the GitHub Action.",
      "Bring your own Token Factory key.",
      "A queue of nine pull requests costs about two cents in model calls.",
    ],
  },
  {
    name: "Team",
    status: "Planned",
    planned: true,
    points: [
      "A managed app per organization.",
      "Learned fixes shared across repositories.",
      "The dashboard and the history of receipts.",
      "Priced per active repository.",
    ],
  },
  {
    name: "Enterprise",
    status: "Planned",
    planned: true,
    points: [
      "Policy as code across an organization.",
      "An audit trail of receipts.",
      "Private model endpoints.",
    ],
  },
];

const NEXT = [
  "Webhooks, so Upkeep reacts the moment a bot opens a pull request.",
  "A GitHub App with scoped permissions.",
  "More languages behind the same verifier.",
];

const LINKS = [
  { label: "Repository", href: REPO },
  { label: "Architecture", href: `${DOCS}/architecture.md` },
  { label: "Models", href: `${DOCS}/models.md` },
  { label: "Live runs", href: `${DOCS}/evals/live.md` },
  { label: "Limitations", href: `${DOCS}/limitations.md` },
  { label: "GitHub Action", href: `${DOCS}/action.md` },
];

function Uses({ uses }: { uses: Use[] }) {
  return (
    <ul className={styles.uses} aria-label="Uses">
      {uses.map((u) => (
        <li key={u.label} data-hue={u.hue}>
          {u.label}
        </li>
      ))}
    </ul>
  );
}

export function Landing() {
  return (
    <div className={styles.page}>
      <a className={styles.skip} href="#content">
        Skip to content
      </a>
      <header className={styles.nav}>
        <div className={styles.navInner}>
          <a className={styles.brand} href="/" aria-label="Upkeep home">
            <span className={styles.mark} aria-hidden="true">
              <Icon name="mark" />
            </span>
            <span>Upkeep</span>
          </a>
          <nav className={styles.navLinks} aria-label="Sections">
            <a href="#how">How it works</a>
            <a href="#evidence">Evidence</a>
            <a href="#business">Business</a>
            <a href={REPO}>GitHub</a>
          </nav>
          <a className={styles.navCta} href="/demo">
            Open the demo
          </a>
        </div>
      </header>

      <main id="content">
        <section className={styles.hero} aria-labelledby="hero-title">
          <div className={styles.wrap}>
            <h1 id="hero-title" className={styles.headline}>
              <span>Clear the pull request queue.</span> <span>Prove every fix.</span>
            </h1>
            <div className={styles.heroRow}>
              <p className={styles.lede}>
                Upkeep is an agent that works your whole pull request queue. It plans the merge
                order, fixes what your policy allows with NVIDIA Nemotron on Nebius Token Factory,
                and proves every fix with your compiler and your tests.
              </p>
              <div className={styles.heroSide}>
                <div className={styles.actions}>
                  <a className={styles.primary} href="/demo">
                    Watch it work the queue
                  </a>
                  <a className={styles.secondary} href={`${DOCS}/action.md`}>
                    Install the GitHub Action
                  </a>
                </div>
                <p className={styles.proof}>
                  <span>9 of 9 in the recorded run</span>
                  <span>72 of 72 across eight live runs</span>
                  <span>about two cents a queue</span>
                </p>
              </div>
            </div>
            <figure className={styles.shot}>
              <div className={`${styles.frame} ${styles.crop}`}>
                <img
                  src="/landing/dashboard-trace.png"
                  width={1440}
                  height={900}
                  alt="The Upkeep dashboard mid-run: the queue of nine pull requests in merge order on the left, the live stage following a jwt major upgrade in the centre with Nemotron Super's turns and the files it edited, and each model's calls and cost on the right."
                />
              </div>
              <figcaption>
                The live stage following a dependency upgrade: the forecast, then Nemotron Super's
                turns and the files it edited.
              </figcaption>
            </figure>
          </div>
        </section>

        <section className={styles.section} id="problem" aria-labelledby="problem-title">
          <div className={`${styles.wrap} ${styles.split}`}>
            <div className={styles.intro}>
              <h2 id="problem-title" className={styles.title}>
                Pull requests stuck one small change from merging.
              </h2>
              <p>Every repository collects them.</p>
              <ul className={styles.blockers} aria-label="What blocks them">
                {BLOCKERS.map((b) => (
                  <li key={b}>{b}</li>
                ))}
              </ul>
              <p>
                Nobody is blocked for a good reason, and nobody has the next twenty minutes. So the
                queue grows.
              </p>
            </div>
            <ul className={styles.findings} aria-label="What research says">
              {FINDINGS.map((f) => (
                <li key={f.href}>
                  <p>{f.claim}</p>
                  <a href={f.href}>{f.source}</a>
                </li>
              ))}
            </ul>
          </div>
        </section>

        <section className={styles.section} id="audience" aria-labelledby="audience-title">
          <div className={styles.wrap}>
            <h2 id="audience-title" className={styles.title}>
              For the people who own the queue.
            </h2>
            <div className={styles.pair}>
              <div>
                <h3>Maintainers and platform teams</h3>
                <p>Go services, with Dependabot or Renovate pull requests piling up.</p>
              </div>
              <div>
                <h3>Teams that need to trust automation</h3>
                <p>
                  A bounded policy decides what Upkeep may touch. Every fix is verified outside the
                  model. Every pull request gets a receipt.
                </p>
              </div>
            </div>
          </div>
        </section>

        <section className={styles.section} id="how" aria-labelledby="how-title">
          <div className={styles.wrap}>
            <div className={styles.intro}>
              <h2 id="how-title" className={styles.title}>
                How it works
              </h2>
              <p>
                Upkeep works the whole queue, not one pull request at a time. Each step says what
                does the work.
              </p>
            </div>
            <div className={styles.howGrid}>
              <ol className={styles.steps}>
                {STEPS.map((s) => (
                  <li key={s.name} className={styles.step}>
                    <h3>{s.name}</h3>
                    <p>{s.text}</p>
                    <Uses uses={s.uses} />
                  </li>
                ))}
              </ol>
              <figure className={`${styles.shot} ${styles.sticky}`}>
                <div className={styles.frame}>
                  <img
                    src="/landing/dashboard-plan.png"
                    width={778}
                    height={970}
                    loading="lazy"
                    alt="The results of a run: 6 ready to merge, 2 merged, 1 needs a person, $0.0191 for the run; the merge order Upkeep planned, with #5 flagged stale after #7 merged; and every pull request grouped by outcome with what Upkeep did."
                  />
                </div>
                <figcaption>
                  After the run: the merge order it planned, #5 flagged stale after #7 merged, and
                  every pull request grouped by outcome.
                </figcaption>
              </figure>
            </div>
          </div>
        </section>

        <section className={styles.section} id="rules" aria-labelledby="rules-title">
          <div className={styles.wrap}>
            <div className={styles.intro}>
              <h2 id="rules-title" className={styles.title}>
                Rules it never breaks.
              </h2>
              <p>They are rules in code, not in a prompt.</p>
            </div>
            <div className={styles.rulesGrid}>
              <ul className={styles.rules}>
                {RULES.map((r) => (
                  <li key={r}>
                    <Icon name="verified" className={styles.ruleIcon} />
                    <span>{r}</span>
                  </li>
                ))}
              </ul>
              <figure className={styles.shot}>
                <div className={styles.frame}>
                  <img
                    src="/landing/dashboard-detail.png"
                    width={1440}
                    height={900}
                    loading="lazy"
                    alt="The summary of a jwt major upgrade: ready to merge with low merge confidence and the reasons behind it, what Upkeep pushed, and the forecast built from release notes and pages Tavily found, each prediction marked confirmed by the build or not seen."
                  />
                </div>
                <figcaption>
                  What a reviewer reads: what changed, the merge confidence and its reasons, and
                  each prediction confirmed by the build or not.
                </figcaption>
              </figure>
            </div>
          </div>
        </section>

        <section className={styles.section} id="models" aria-labelledby="models-title">
          <div className={styles.wrap}>
            <div className={styles.intro}>
              <h2 id="models-title" className={styles.title}>
                Three Nemotron models, measured, not assumed.
              </h2>
              <p>
                Before writing the routing, Upkeep ran 73 experiments: 3.06 million tokens for about
                $2. Each model does the job it measured best at.
              </p>
            </div>
            <ul className={styles.models}>
              {MODELS.map((m) => (
                <li key={m.name} data-hue={m.hue}>
                  <h3>
                    {m.name} <span>{m.size}</span>
                  </h3>
                  <p>{m.text}</p>
                </li>
              ))}
            </ul>
            <p className={styles.note}>Prices are read live from Token Factory's catalog.</p>
          </div>
        </section>

        <section className={styles.section} id="evidence" aria-labelledby="evidence-title">
          <div className={styles.wrap}>
            <div className={styles.intro}>
              <h2 id="evidence-title" className={styles.title}>
                The evidence
              </h2>
              <p>
                Measured on a sample Go service with nine open pull requests, one blocker each: in
                replay, live, and as a GitHub Action.
              </p>
            </div>
            <dl className={styles.stats}>
              {STATS.map((s) => (
                <div key={s.value}>
                  <dt>{s.value}</dt>
                  <dd>{s.text}</dd>
                </div>
              ))}
            </dl>
            <figure className={`${styles.shot} ${styles.wide}`}>
              <div className={styles.frame}>
                <img
                  src="/landing/dashboard-learned.png"
                  width={1440}
                  height={900}
                  loading="lazy"
                  alt="The second run of the same queue: #4, #5 and #7 carry a learned fix, the results show 6 ready to merge, 2 merged and 1 needing a person, and the run cost $0.0055 with 3 model calls."
                />
              </div>
              <figcaption>
                Run again: #4, #5 and #7 reuse a learned fix, and the queue costs $0.0055.
              </figcaption>
            </figure>
            <div className={styles.action}>
              <div className={styles.actionHead}>
                <h3>On pull requests Dependabot really opened</h3>
                <p>
                  As a GitHub Action on the sample repository, Upkeep worked{" "}
                  <a href={`${SAMPLE}/pull/8`}>#8</a> and <a href={`${SAMPLE}/pull/11`}>#11</a> as
                  Dependabot left them.
                </p>
                <a className={styles.more} href={`${DOCS}/evals/action.md`}>
                  Read the runs
                </a>
              </div>
              <ol className={styles.timeline}>
                <li>Brought both up to date with main.</li>
                <li>Saw that both change go.mod and will conflict.</li>
                <li>GitHub merged one. The other then conflicted, as the plan said.</li>
                <li>
                  Nemotron Ultra resolved that conflict in one call for $0.0006. Every check passed,
                  and it merged.
                </li>
              </ol>
            </div>
          </div>
        </section>

        <section className={styles.section} id="business" aria-labelledby="business-title">
          <div className={styles.wrap}>
            <div className={styles.intro}>
              <h2 id="business-title" className={styles.title}>
                The business model
              </h2>
              <p>
                This is the plan, not a track record. The open source core exists today. The Team
                and Enterprise plans are planned.
              </p>
            </div>
            <ul className={styles.tiers}>
              {TIERS.map((t) => (
                <li key={t.name} data-planned={t.planned}>
                  <h3>{t.name}</h3>
                  <p className={styles.status}>{t.status}</p>
                  <ul>
                    {t.points.map((p) => (
                      <li key={p}>{p}</li>
                    ))}
                  </ul>
                </li>
              ))}
            </ul>
            <div className={styles.payback}>
              <h3>How it pays for itself</h3>
              <div className={styles.sum}>
                <p>
                  <strong>15 min</strong>
                  <span>per stuck pull request. An assumption: use your own.</span>
                </p>
                <p>
                  <strong>× 40</strong>
                  <span>bot pull requests a month, as an example.</span>
                </p>
                <p>
                  <strong>= 10 hours</strong>
                  <span>of an engineer's time every month.</span>
                </p>
                <p data-cost="true">
                  <strong>under $1</strong>
                  <span>
                    Upkeep's model cost for those 40, at about $0.002 a pull request ($0.0191 for
                    nine in the recorded run).
                  </span>
                </p>
              </div>
            </div>
          </div>
        </section>

        <section className={styles.section} id="next" aria-labelledby="next-title">
          <div className={`${styles.wrap} ${styles.split}`}>
            <h2 id="next-title" className={styles.title}>
              What's next
            </h2>
            <ul className={styles.next}>
              {NEXT.map((n) => (
                <li key={n}>{n}</li>
              ))}
            </ul>
          </div>
        </section>

        <section className={styles.closing} aria-labelledby="closing-title">
          <div className={styles.wrap}>
            <h2 id="closing-title" className={styles.closingTitle}>
              Nine pull requests, one blocker each. Watch it work.
            </h2>
            <p className={styles.closingText}>
              Upkeep plans the order, fixes what the policy allows, proves every fix and hands the
              rest to a person. Then run it again with what it learned.
            </p>
            <div className={styles.actions}>
              <a className={styles.primary} href="/demo">
                See the queue worked live
              </a>
            </div>
            <ul className={styles.links} aria-label="Documentation">
              {LINKS.map((l) => (
                <li key={l.label}>
                  <a href={l.href}>{l.label}</a>
                </li>
              ))}
            </ul>
          </div>
        </section>
      </main>

      <footer className={styles.footer}>
        <div className={styles.wrap}>
          <p>Upkeep · MIT · NVIDIA Nemotron on Nebius Token Factory · Tavily</p>
        </div>
      </footer>
    </div>
  );
}
