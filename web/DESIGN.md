# Upkeep dashboard design

Two pages share one look:

- the overview at `/` explains the product;
- the dashboard at `/demo` tells one run in three moments: a queue that cannot merge, an agent working it live, and what the run left behind.

On the dashboard, the agent at work is the hero.

## The dashboard flow

1. **Before the run.**
   - A large title states the problem ("9 pull requests can't merge.").
   - One sentence says what Upkeep does, and the primary action sits under it.
   - The queue waits on the left; the models sit on the right.
   - The centre holds the five stages every pull request goes through.
2. **During the run.**
   - **Header:** one row with the title, the status line, the actions and a thin progress line.
   - **Merge order graph**, across the centre.
   - **Live stage**, below the graph:
     - it follows one pull request until that one finishes, holds its outcome for 1.2 seconds, then moves to the working pull request that started first;
     - an "In parallel" strip shows the other workers, and a click on any of them, on a queue row or on a graph node pins the stage there ("Follow the agent" returns);
     - the stage shows the five stages (check, forecast, fix, verify, act), a prompt line with the latest event (a Nemotron turn and what it said, a tool call and its file, the Tavily search, a check), and the trace as it grows;
     - it stays in view while the page scrolls.
3. **After the run.**
   - The centre shows the figures and what Upkeep did, grouped into merged, ready to merge and needs a person.
   - The graph marks each outcome, and what went stale.
   - "Run again" starts a new run that reuses the learned fixes.

The full detail of a pull request opens in a side sheet with two tabs:

- **Trace**;
- **Summary**:
  - a verdict, what changed and the facts in one line;
  - a three-step confidence meter with its reasons;
  - how a conflict was resolved;
  - the forecast and its marks;
  - the diff;
  - the verifier in two columns;
  - the receipt and its steps.

## The overview

- **Hero:** the headline and the two actions, beside a console that replays the real trace of the jwt upgrade from the recorded run. It is on a loop and labelled "Recorded run".
- **Evidence strip:** under the hero.
- **Sections:** the problem, who it is for, how it works, the rules, the models, the evidence, the cost, what's next, and a closing call to the demo.

## Principles

- **Alive, not busy.** Motion follows what the agent did. Nothing is typed out to fake streaming. The overview's console is a labelled replay of a recorded trace, not a live feed.
- **Colour means state.**
  - NVIDIA green (`--accent`) means done or the primary action; a brighter green (`--live`) means happening now.
  - Amber means waiting on a person or stale; red means a failure.
  - Each model keeps its own hue: Ultra violet, Super blue, Nano teal, Tavily pink.
  - Every coloured state also has an icon or a word.
- **The product's own things carry the screen:** the merge order, the five stages, the models' turns and tool calls, the receipts.
- **Honest states.** A replayed run says so; zero is zero; an empty group says "None".

## Tokens

Colours and timings are custom properties in `src/tokens.css`. The few literal colours left in components are the diff line colours, translucent glows and the overlay backdrop.

| Role                  | Token                                                                            | Value                                                 |
| --------------------- | -------------------------------------------------------------------------------- | ----------------------------------------------------- |
| Page, surfaces        | `--frame`, `--sheet`, `--card`, `--card-hover`, `--muted`                        | `#070809`, `#0c0e0f`, `#111416`, `#161a1c`, `#1d2225` |
| Ink                   | `--fg`, `--fg-muted`                                                             | `#eef1ef`, `#8f9894`                                  |
| Edges                 | `--rule`, `--border`, `--border-strong`                                          | `#171b1d`, `#1e2326`, `#2f363a`                       |
| Done, action          | `--accent` (`--accent-ink` on it)                                                | `#76b900` (`#0a0b0a`)                                 |
| Happening now         | `--live`                                                                         | `#9be22d`                                             |
| Danger, warning, info | `--danger`, `--warn`, `--info`                                                   | `#f07272`, `#e9b053`, `#5fb4f5`                       |
| Models and search     | `--ultra`, `--super`, `--nano`, `--search`                                       | `#b69cff`, `#4fb3ff`, `#2dd4bf`, `#ff7ab6`            |
| Tints                 | `--tint-accent`, `--tint-warn`, `--tint-danger`, `--tint-info`, `--tint-neutral` | translucent fills at 8–14 %                           |
| Timing                | `--fast`, `--base`, `--slow`; `--ease`, `--out`                                  | 150, 240, 480 ms; an ease and an exponential ease-out |

## Type

| Face                | Used for                                                                                                      | Sizes                                                                                          |
| ------------------- | ------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| Bricolage Grotesque | the titles, the stage and verdict titles, the figures, the sheet titles, and the overview's headings          | up to 56 px before the run, 28 px during it, 72 px on the overview                             |
| Hanken Grotesk      | the interface                                                                                                 | body at 13–16 px, headings at 15–22 px                                                         |
| Martian Mono        | code and machine output: pull request numbers, paths, model names, the prompt line, trace rows, status labels | a pixel smaller than usual, because it is wide; 10 px only for small uppercase labels and tags |

Numbers that update use tabular figures.

## Motion

CSS only: no inline styles and no library. Only `transform`, `opacity`, `filter`, background position and SVG geometry move.

| Moment                             | What moves                                                                                                                                                                                                                                                                                                                           |
| ---------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| The agent works                    | <ul><li>A beam runs along the top edge of the stage.</li><li>The stage bars fill as each stage completes, and the running one shimmers.</li><li>The prompt line has a caret and a highlight sweep on every new event.</li><li>The last three trace rows rise and glow.</li><li>An attempt flashes when it passes or fails.</li></ul> |
| Workers start or switch            | The lanes enter. The stage slides when it moves to another pull request.                                                                                                                                                                                                                                                             |
| A pull request finishes            | <ul><li>Its row flashes in the outcome colour.</li><li>Its node in the graph pops to the outcome, and the connector fills.</li><li>The count and the cost flash.</li><li>The progress line grows.</li></ul>                                                                                                                          |
| A model answers or Tavily searches | The model's totals flash and its bar grows. The search block flashes pink.                                                                                                                                                                                                                                                           |
| The run ends                       | The figures sharpen in and the groups stagger in.                                                                                                                                                                                                                                                                                    |
| First paint                        | The title, lead and actions rise in sequence. The five stages light one by one in a slow loop until the run starts.                                                                                                                                                                                                                  |
| Overlays                           | The modal scales in and the tab panels rise in.                                                                                                                                                                                                                                                                                      |
| The overview                       | <ul><li>The hero rises in sequence and the console replays on a 16-second loop.</li><li>Sections fade up and the step lines draw as they scroll in. This uses `animation-timeline: view()`; content is visible by default where unsupported.</li></ul>                                                                               |

Infinite animations are limited to things that are running:

- the stage's beam, its live dot, the running bar's shimmer and the caret;
- the working pulses in the queue and the graph;
- the header's live dot and spinner;
- the mark while a run is on;
- the idle stage loop;
- the connection pulse;
- the overview console.

How it is built:

- Restarts use a React `key`, or a `data-*` attribute that starts matching. No animation state lives in JavaScript.
- Keyframes live in the module that uses them, because the build does not share them across modules. The shared ones keep the `up-` names.
- With `prefers-reduced-motion: reduce`, every animation is removed and the final state shows.

## Components

| Group    | Components                                                                                                                                                                                                                                                         |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Frame    | `TopBar` (the mark, the mode, the budget, the connection), `Mark` (the five bars, lit in turn during a run), `ActionBar`                                                                                                                                           |
| Centre   | `MergeGraph`, `Stage` (with `Lanes` and `TraceView`), `Results` (with `Figures`), `Onboarding` (the five stages before the run)                                                                                                                                    |
| Sides    | `QueueList`: two lines per pull request (its stage marks and status, small indicators for overlaps, learned fixes and confidence, whose text stays for assistive technology). `Agents`: the models, the web search, and the cost comparison while the run is going |
| Sheets   | `DetailPanel` (Trace and Summary), `PolicyPanel` (the rules grouped as Never, Only when and Within limits), `Overlay`, `Tabs`                                                                                                                                      |
| Overview | `Landing`, `HeroConsole`                                                                                                                                                                                                                                           |

## Accessibility

- A skip link first, one `h1`, and every pull request a real button named "Pull request N: title".
- Visible focus in the accent green.
- Only the connection status is an `aria-live` region; the stage's prompt line is not.
- The overview's console is one image with a text description.

## Constraints that shape the code

- The content security policy forbids inline styles:
  - variants are `data-*` attributes;
  - dynamic widths are SVG `rect` attributes;
  - fonts are self-hosted.
- Styles are CSS modules next to their component.
- No comments in any source file; this file is where the reasoning lives.

## Checking a change

Look at both pages at 390, 1440 and 1920 pixels:

- before, during and after a run;
- with the detail sheet and the policy panel open;
- after "Run again";
- with reduced motion.
