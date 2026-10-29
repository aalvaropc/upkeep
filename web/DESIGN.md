# Upkeep dashboard design

The dashboard is a screen people work in, not a page they read: a queue of pull requests that an agent works while they watch. Its one question is **which pull requests can merge, and in what order**. Everything on the first screen answers that or the next question it raises (what did Upkeep do to this one, what did it cost, which models did the work).

## Principles

- **Colour goes on data, not on chrome.** The frame, the navigation and the headings are ink and grey. Hue appears on the progress of each pull request, its outcome, its merge confidence, the models' usage and the one primary action. Every coloured state also has an icon and a word.
- **Figures are type, not tiles.** The run's numbers sit in a row of labels and 28px figures, with no boxes, icons or sparklines.
- **Surfaces are flat.** One tinted frame, one white sheet, 1px whisper rules. No shadows except on what truly floats (the side sheet), no gradients, no glass.
- **The product's own things carry the screen.** The queue is shown in the merge order Upkeep planned; each row has a progress strip of Upkeep's own stages; the finding the plan made (a pull request that went stale after another merged) is the line under the title.
- **Honest states.** Zeros print as zeros, missing values as a dash, an empty filter says so, and a replayed run says it is replayed.

## Shell

- A tinted L-frame (`--frame`, a green-grey: the colour of a passing check, low in chroma) holds the 228px sidebar and the 48px top bar with no rule between them.
- The work sits on a white sheet inset 8px from the window's right and bottom edges, 16px corners, a 1px edge and no shadow.
- The sidebar names the product (its mark on the accent) and the repository it works, then the navigation (the queue, the policy) and the documentation links.
- The top bar carries the breadcrumb of the session, the mode (replay or live), the live token budget and the connection status.
- On a phone the frame disappears: the sheet is the screen and the sidebar folds to the product and the navigation.

## Tokens

All colours and sizes are custom properties in `src/tokens.css`; components never use a literal colour.

| Role                                  | Token                                                                            | Value                                      |
| ------------------------------------- | -------------------------------------------------------------------------------- | ------------------------------------------ |
| Frame                                 | `--frame`                                                                        | `#f2f5f2`                                  |
| Sheet, cards                          | `--sheet`, `--card`                                                              | `#ffffff`                                  |
| Hover, quiet fill                     | `--card-hover`, `--muted`                                                        | `#f4f6f5`, `#ebedec`                       |
| Ink, secondary ink                    | `--fg`, `--fg-muted`                                                             | `#131416`, `#5d5f60`                       |
| Edges, rules                          | `--border`, `--rule`, `--border-strong`                                          | `#e6e9e7`, `#f0f2f1`, `#cdd1ce`            |
| Accent (done, primary action, merged) | `--accent`                                                                       | `#17734a`                                  |
| Danger, warning, info                 | `--danger`, `--warn`, `--info`                                                   | `#c4242a`, `#92400e`, `#2f70bb`            |
| Models and search                     | `--ultra`, `--super`, `--nano`, `--search`                                       | `#7a4fb0`, `#2f70bb`, `#1b6c80`, `#a8366f` |
| Pale tints for pills                  | `--tint-accent`, `--tint-warn`, `--tint-danger`, `--tint-info`, `--tint-neutral` |                                            |

Every hue reaches 3:1 or more on white; text colours reach 4.5:1.

## Type

- IBM Plex Sans for everything, at 400, 500 and 600. Sizes: 22px for the title, 15–16px for section headings, 28px for the run's figures, 14px for body and rows, 12–13px for secondary text. Nothing under 11px.
- JetBrains Mono only for identifiers that are code: pull request numbers, file paths, model names, the session id, tool names in the trace.
- Tabular figures everywhere numbers update. Sentence case; no tracked capitals.

## The queue

- One table, no card around it, in the plan's merge order (number order until the plan exists), with underline tabs for its states and the count of each.
- Columns: the pull request (number, title, author, bump, labels, and once done the sentence of what changed), the blocker and any overlap with another pull request, the progress, the result, the cost.
- **The progress strip** is the screen's own element: five segments for check, forecast, fix, verify and act. Accent when done, blue and pulsing while running, red when failed, amber when handed to a person, a dashed outline when the stage was not needed. Its accessible name reads every stage in words.
- The result is the outcome pill, the merge confidence (gauge icon, accent, warn or danger) and a learned-fix mark when a fix was reused.
- Costs are right-aligned: the dollars, then calls and time in grey.
- On a phone each row becomes a two-line item: the pull request across, then the progress, with the result and the cost side by side.

## The rail and the detail

- The rail holds the models (each with its job and its share of the run's cost), the Tavily search, the cost comparison with the largest model's price, and the merge plan (the constrained steps with their reasons, the pairs that share files, what went stale after the run). Sections are separated by rules, never nested in boxes.
- The activity feed sits under the queue.
- The detail is a side sheet with two tabs: Trace (the plan, the forecast and its search, every model turn and tool call grouped by attempt, the verifier, the action) and Summary (what changed, merge confidence, the forecast with its sources, the diff, the verifier, the receipt).

## Motion

Motion is feedback only: a press, a hover, the running stage's pulse, the sheet opening. Nothing performs an entrance. Everything moving has a `prefers-reduced-motion` fallback.

## Accessibility

- Visible focus on every interactive element (`:focus-visible`, a 2px ring in `--ring`).
- A skip link is the first focusable element and moves focus to `<main>`; the screen has one `h1`.
- Each row opens its detail from a real button named "Pull request N: title".
- Decorative icons are inline SVG with `aria-hidden="true"`; the progress strip is an image with a full text name.
- Live updates stay out of `aria-live` regions except the connection status.

## Constraints that shape the code

- The content security policy forbids inline styles, so components never use a `style` attribute. Variants are `data-*` attributes and custom properties; dynamic widths are SVG `rect` attributes.
- Styles are CSS modules next to their component.
- No comments in any source file; this file is where the reasoning lives.

## Checking a change

1. Reuse the tokens; add a token before adding a colour.
2. Pair every new status with an icon from `components/Icon.tsx` and a word.
3. Run the dashboard at 1440, 768 and 390 pixels and look at it idle, running, finished and with the detail open.
4. Measure it with an app-screen inspector (contrast, text size, colour share, rules, ragged rows, blank bands); the redesign of 2026-10-28 went from 3 failures and 3 warnings to none.
