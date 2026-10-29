# Upkeep dashboard design

The dashboard tells one story in three moments: a queue that cannot merge, an agent working it live, and what the run left behind. The agent at work is the hero; everything else supports it.

## The flow

1. **Before the run.** A large title states the problem ("9 pull requests can't merge.") on one line, one sentence says what Upkeep does with NVIDIA Nemotron on Nebius Token Factory, and the primary action sits right under it. The queue waits on the left, the five stages every pull request goes through sit in the centre as a panel the live stage later lights up, the models on the right.
2. **During the run.** The centre becomes the **live stage**: the pull request the agent touched last, its five stages (check, forecast, fix, verify, act), a "now" line with the latest thing that happened (a Nemotron turn and what it said, a tool and the file it edited, the Tavily search, a check), and the trace scrolling as it grows. The stage stays pinned in view while the page scrolls and follows the agent by itself; a click on a pull request in the queue pins the stage to it, and "Follow the agent" returns.
3. **After the run.** The centre shows the **results**: the run's figures and what Upkeep did grouped into merged, ready to merge and needs a person. "Run again" starts the next run and shows the learned fixes.

The full detail of a pull request (trace and summary tabs) opens in a side sheet from the stage or the results.

## Principles

- **Alive, not busy.** Motion answers an event the agent really produced (see Motion). Nothing moves for decoration, and nothing is typed out to fake streaming.
- **Colour means state.** NVIDIA green (`--accent`, `#76b900`) means done or the primary action; a brighter green (`--live`) means happening now; amber is waiting on a person or stale; red is a failure; each model keeps its own hue (Ultra violet, Super blue, Nano teal, Tavily pink). Every coloured state also has an icon or a word.
- **The product's own things carry the screen:** the merge order, the five stages, the models' turns and tool calls, the receipts. No stock cards, no decorative charts.
- **Honest states.** A replayed run says so; zero is zero; an empty group says "None".

## Tokens

All colours and sizes are custom properties in `src/tokens.css`; components never use a literal colour.

| Role                  | Token                                                                            | Value                                                 |
| --------------------- | -------------------------------------------------------------------------------- | ----------------------------------------------------- |
| Page, surfaces        | `--frame`, `--sheet`, `--card`, `--card-hover`, `--muted`                        | `#08090a`, `#0e1011`, `#131618`, `#191d20`, `#20252a` |
| Ink                   | `--fg`, `--fg-muted`                                                             | `#f2f4f3`, `#9aa3a0`                                  |
| Edges                 | `--rule`, `--border`, `--border-strong`                                          | `#1c2023`, `#23282c`, `#3a4247`                       |
| Done, action          | `--accent` (`--accent-ink` on it)                                                | `#76b900` (`#0a0b0a`)                                 |
| Happening now         | `--live`                                                                         | `#9be22d`                                             |
| Danger, warning, info | `--danger`, `--warn`, `--info`                                                   | `#ff6b6b`, `#f5b041`, `#4fb3ff`                       |
| Models and search     | `--ultra`, `--super`, `--nano`, `--search`                                       | `#b69cff`, `#4fb3ff`, `#2dd4bf`, `#ff7ab6`            |
| Tints for pills       | `--tint-accent`, `--tint-warn`, `--tint-danger`, `--tint-info`, `--tint-neutral` | translucent versions of the above                     |

Text reaches 4.5:1 on every surface; hues reach 3:1.

## Type

- Bricolage Grotesque for what names a moment: the title (up to 56px before the run, 30px during it), the stage title, the figures and the sheet titles. Its optical size axis tightens the forms at display sizes.
- Hanken Grotesk for the interface: section headings at 15-22px, body at 13-15px, nothing under 11px.
- Martian Mono for what is code or machine output: pull request numbers, file paths, model names, the live line, the mode and status labels. It is wide, so it is set 1px smaller than a usual mono.
- Tabular figures on every number that updates.

## Motion

Every effect is tied to a real event or state and is CSS only (no inline styles, no library). Durations come from `--fast` 150ms, `--base` 240ms and `--slow` 480ms; the easing is `--out`, an exponential ease-out. Only `transform`, `opacity`, `filter`, background position and SVG geometry move.

| Moment                             | What moves                                                                                                                                                                                                                                                                                                                          |
| ---------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| The agent works a pull request     | A beam travels the top edge of the live stage; the five stage bars fill left to right as each stage completes, the running one shimmers; the "now" line is a prompt with a caret and a highlight sweep on every new event; the last three trace rows rise and glow in their tone; an attempt header flashes when it passes or fails |
| Several workers run                | The "In parallel" lanes enter as workers start, their step text re-fades on change, the followed lane's ring transitions; the stage slides in when it moves to another pull request                                                                                                                                                 |
| A pull request finishes            | Its queue row flashes in the outcome colour and the outcome icon pops in; the finished count and the cost flash when they change; the run step fills with the share finished                                                                                                                                                        |
| A model answers or Tavily searches | The model's totals flash, its bar grows, the web search block flashes pink                                                                                                                                                                                                                                                          |
| The plan arrives                   | The graph's nodes reorder into the merge order and the arcs for shared files appear                                                                                                                                                                                                                                                 |
| The run ends                       | The figures rise in with a short blur-to-sharp; the groups stagger in; the merge graph's nodes pop to their outcome and its connectors fill                                                                                                                                                                                         |
| First paint                        | The title, lead and actions rise in sequence; the five stages stagger in, then light one by one in a slow loop that stops when the run starts                                                                                                                                                                                       |
| Overlays                           | The policy modal scales in and its groups stagger; tab panels rise in                                                                                                                                                                                                                                                               |
| The landing                        | The hero rises in sequence and its screenshot sharpens in under a faint beam; sections fade up and the step lines draw as they scroll into view (`animation-timeline: view()`, visible by default where unsupported)                                                                                                                |

Rules that keep it honest and light:

- At most one infinite animation per visible region: the stage beam, the running mark, the idle stage loop, the connection pulse.
- Restarts use a React `key` or a `data-*` attribute that starts matching; there is no animation state in JavaScript.
- Keyframes live in each module that uses them (the build does not share them), with the same `up-` names and shapes.
- `prefers-reduced-motion: reduce` removes every animation and shows the final state: `tokens.css` shortens all durations and each module hides what would otherwise rest in a mid state (the stage beam, the idle loop, the landing beam).

## Components

- `ActionBar`: one row with the title, the status line and the two actions, a thin progress line under it while a run exists, and a larger variant before the run.
- `MergeGraph`: the merge order drawn across the centre. One node per pull request in merge order, joined by a line that fills as they finish; an arc joins two pull requests that change the same files (amber when they conflict); a node shows merged, ready, person, stale or failed; a click follows that pull request, and a line under it gives the reason, the shared files and anything that went stale.
- `QueueList`: the queue in the planned merge order; each row has the number and title, five stage marks and the current step or outcome, with small indicators on the right for overlaps, learned fixes and merge confidence (their text is kept for assistive technology); `aria-pressed` marks the row the stage follows.
- `Stage`: the live stage described above, built on `TraceView`.
- `Results`: the figures, the merge line and the three groups.
- `Agents` in the rail: the models and the web search, plus the cost comparison while the run is going (the results carry the cost after it).

## Accessibility

- A skip link first, one `h1`, every pull request a real button named "Pull request N: title".
- Visible focus (`--ring`, the accent green). The stage's live line is not an `aria-live` region; only the connection status is.
- Motion has a `prefers-reduced-motion` fallback everywhere.

## Constraints that shape the code

- The content security policy forbids inline styles: variants are `data-*` attributes and custom properties, dynamic widths are SVG `rect` attributes, fonts are self-hosted.
- Styles are CSS modules next to their component.
- No comments in any source file; this file is where the reasoning lives.

## Checking a change

Run the dashboard at 1440, 768 and 390 pixels before, during and after a run, with the detail open and after Run again, and measure it with an app-screen inspector (contrast, text size, rules, blank bands). This version measures no failures; its one warning is the 64px title of the first screen, kept on purpose.
