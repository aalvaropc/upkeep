# Upkeep dashboard design

The dashboard is an operations console: dense, dark and quiet until something happens. A person watches pull requests move from blocked to done and must be able to tell, at a glance, what each model did and what it cost. The starting point was the design-system search of [UI/UX Pro Max](https://uupm.cc/) (dark OLED style, mono headings, sans body), tightened to what this product needs.

## Principles

- **Action first.** The primary button sits above the board. Everything else is read-only evidence.
- **Status is never color alone.** Every status pairs a color with an icon and a word.
- **One live region.** The connection badge is the only `role="status"` element. The event list and the board update silently so a screen reader is not flooded.
- **Evidence over decoration.** Numbers, diffs and receipts are set in the mono face; prose is set in the sans face.
- **Motion explains, it does not decorate.** Elements rise into place once, the connection icon breathes, nothing else moves.

## Tokens

All tokens live in `src/tokens.css`.

| Token             | Value     | Use                             | Contrast on card |
| ----------------- | --------- | ------------------------------- | ---------------- |
| `--bg`            | `#0f172a` | page                            |                  |
| `--bg-sunken`     | `#0b1222` | header, event log               |                  |
| `--card`          | `#1b2336` | panels and columns              |                  |
| `--muted`         | `#272f42` | rows inside a panel             |                  |
| `--fg`            | `#f8fafc` | primary text                    | 14.98            |
| `--fg-muted`      | `#94a3b8` | secondary text                  | 6.11             |
| `--accent`        | `#22c55e` | primary action, done, live      | 6.88             |
| `--danger`        | `#f87171` | blocked                         | 5.66             |
| `--warn`          | `#fbbf24` | working, reconnecting           | 9.39             |
| `--info`          | `#38bdf8` | verified                        | 7.31             |
| `--search`        | `#f9a8d4` | web search (Tavily)             | 8.64             |
| `--border`        | `#334155` | panel outlines                  | decorative       |
| `--border-strong` | `#475569` | dashed empty states, idle badge | decorative       |
| `--ring`          | `#ffffff` | focus outline, 2px, 2px offset  |                  |

Every text pair meets 4.5:1. Borders only separate panels; no state is communicated by a border alone.

## Status language

| Column   | Color    | Icon           | Meaning                                    |
| -------- | -------- | -------------- | ------------------------------------------ |
| Blocked  | `danger` | slashed circle | Needs a change before it can merge         |
| Working  | `warn`   | circular arrow | Upkeep is fixing it now                    |
| Verified | `info`   | shield check   | Passed every check, waiting for its action |
| Done     | `accent` | circle check   | Merged, pushed or handed to a person       |

The connection badge uses the same pattern: `connecting` (dots), `live` (signal), `reconnecting` (triangle).

## Typography

- **JetBrains Mono** (variable, 100-800) for the brand, headings, labels, counts and anything that is data.
- **IBM Plex Sans** (variable, 100-700) for sentences.
- Both are self-hosted from `@fontsource-variable` and only the Latin subset is shipped, because the content security policy is `default-src 'self'`. `font-display: swap` avoids invisible text.
- Body text is 15px with a 1.55 line height; the smallest text is 12px and is always mono.

## Screens

- **Action bar**: the one primary button (Work the queue, then Waiting for a slot, Working, Run again), a plain-words status line, and the policy panel.
- **Board**: four columns, one card per pull request. A card shows who opened it (person or bot), what blocks it, the current step with a running clock, then the outcome, a one-sentence note, what Upkeep did (pushed, patch attached, merged) and the cost.
- **Models rail**: Ultra, Super and Nano each with their job, calls, tokens, time and cost; then the web search (Tavily) with its searches, pages kept and last query, kept out of the cost comparison; and the cost of the run against the same tokens at the largest model's price, always labelled a price comparison.
- **Merge plan rail**: the suggested merge order, each pull request with its reason and its live outcome (icon and word), the pairs that change the same files marked `conflict` (warn, triangle) or `merge cleanly` (info, merge arrow), and after the run a warning for each ready pull request the run's merges made stale. A card carries one matching chip: `conflicts with #7`, `shares files with #6` or `conflicts with main`.
- **Activity**: the last events in plain words.
- **Detail panel** (side sheet, opens from a card) with two tabs. **Trace** is open by default while the pull request is being worked and follows it live: the plan, the forecast with its web search (query, pages kept, time or recorded), each fix attempt with its failure in the header, every model turn (role pill, what it said, which tools it asked for, time, tokens, cost) and every tool call with the file or symbol it touched, the verifier and the action. **Summary** is open by default once it is done: what changed, what Upkeep did, the forecast with each prediction marked confirmed or not seen by the build, the failure output, the diff, the verifier checklist, flaky tests and the receipt with its steps. The summary is fetched again when the pull request finishes, without blanking what was shown. The tabs follow the WAI-ARIA tabs pattern (arrow keys, Home, End) and the trace is not a live region.
- **Policy panel**: the rules the server runs with, as the server states them.

Role colors: Ultra violet, Super sky, Nano teal; they appear only where a model is named. The web search is pink, with the globe icon and the words "web search" wherever it appears.

## Layout

- Header is sticky and carries the brand and the connection badge.
- At 1100px and wider the board takes the left column and a 340px rail (models, events) sits on the right.
- Below 1100px the board is two columns and the rail goes below it in two columns; below 800px everything stacks, below 600px the board is a single column.
- Verified at 375, 768, 1024 and 1440 pixels with no horizontal scroll.

## Motion

- Panels rise 6px and fade in over 240ms with a 50ms stagger. Only `opacity` and `transform` animate.
- The connection icon pulses at a 2.4s period while connecting and live.
- Under `prefers-reduced-motion: reduce`, every animation and transition collapses to an instant change.

## Accessibility

- Visible focus on every interactive element (`:focus-visible`, white 2px ring).
- A skip link is the first focusable element and moves focus to `<main>`.
- Interactive targets are at least 44px tall.
- Decorative icons are inline SVG with `aria-hidden="true"`.
- The disabled primary button keeps its label and drops its fill so the state is readable, not just dimmed.

## Constraints that shape the code

- The content security policy forbids inline styles, so components never use a `style` attribute. Variants are expressed with `data-*` attributes and CSS custom properties.
- Styles are CSS modules next to their component. Dynamic widths (cost bars) are SVG `rect` attributes, which the policy allows, not style attributes. `composes ... from` is avoided because it makes the PostCSS pipeline warn.
- No comments in any source file; this file is where the reasoning lives.

## Adding a screen

1. Reuse the tokens; add a token before adding a literal color.
2. Pair every new status with an icon from `components/Icon.tsx` and a word.
3. Put numbers in the mono face and give them a unit.
4. Keep new live updates out of `aria-live` regions unless a person must react to them.
5. Check the new screen at the four widths and with reduced motion on.
