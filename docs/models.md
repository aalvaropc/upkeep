# Which Nemotron model does what

Upkeep uses the three Nemotron 3 models on Nebius Token Factory. The routing is not a guess: before writing it, 73 runs (3.06 million tokens, about two dollars) measured what each model is good at on the real jobs. The samples are small (three to six runs per cell), so read the numbers as direction, not precision.

## Platform facts that shaped the code

- Prices per million tokens, read live from `GET /v1/models?verbose=true`: Ultra 1.00 in and 3.00 out, Super 0.30 and 0.90, Nano 0.06 and 0.24. Ultra costs 3.3 times Super; against Nano it is about 17 times on input and 12.5 times on output.
- Reasoning is switched off with `chat_template_kwargs: {"enable_thinking": false}`. `/no_think` in the prompt does nothing, and `reasoning_effort: "none"` returned an empty answer on Nano.
- A small call takes one to two seconds on all three models. Rate limits are 300 requests and 200,000 tokens a minute on Ultra and Super, and 800,000 tokens on Nano.

## Fixing a broken build (tools: `read_file`, `apply_edit`, `go_doc`, `run_checks`)

| Scenario | Configuration | Fixed |
|---|---|---|
| a rename on `main` broke the pull request | Super or Ultra, reasoning on or off | 12 of 12 |
| jwt v4 to v5, the hardest | Super, reasoning off | about 65 percent per attempt |
| | without migration notes | 2 of 6 |
| | with the migration guide | 5 of 6 |
| | Super with reasoning on | 1 of 3, and two to three times slower |
| | Ultra inside the tool loop, any variant | 0 of 21 |
| | Ultra writes a plan and Super applies it | 0 of 6 |
| a small merge conflict | Super | 2 of 3 (one run dropped a constant) |
| | Ultra, one call, no tools | 3 of 3 |

Why Ultra loses inside the loop: it explores instead of editing (ten to twelve of twelve turns reading documentation, no edit), or rewrites more code than needed, or invents packages. Its written plans contained APIs that do not exist and Super followed them.

What follows from it:

- **Super fixes builds and tests**, with reasoning off, in up to three fresh attempts. Each attempt starts from a clean tree and from the previous failure's output; three attempts turn a 65 percent attempt into roughly 96 percent. The receipt says which attempt worked.
- **Ultra stays out of the loop.** It resolves small conflicts in one call and writes the diagnosis when a pull request is handed to a person, where it measured best.
- **Upkeep runs `gofmt` itself** after every edit. Left to the model, formatting caused several failures.
- **Rules are checked by code.** Models added comments in 2 of 73 runs and never tried to edit tests, but the verifier checks both on the final diff regardless.

## Forecasting what an upgrade will break

For a minor or major bump Upkeep reads the release notes and the migration guide and asks a model which identifiers the project uses will break. The first version used Nano. On the recorded prompts, three samples each:

| Model | jwt v4 to v5 | clockwork 0.4 to 0.5 |
|---|---|---|
| Nano | named the real break in two of three runs; one run answered with only the module path and one with eight names, most of them untouched | named the real break each time, with different extras |
| Super | the same answer three times, always naming the real break | the same answer three times |
| Ultra | the same answer three times | two of three; once nothing |

So the forecast is read by Super. Nano's answers were not stable enough to feed a fixer. Two more changes came from reading the recordings: search results are kept only when they are about the module's own repository (one search had returned another project's release page and the model predicted removals that did not exist), and every prediction must appear in the sources, otherwise it is dropped and counted.

The forecast is checked afterwards: the compiler's output either mentions a predicted identifier (confirmed) or does not (not seen), and the dashboard shows both.

## The note for the reviewer

Nano writes one sentence about what a model-made change does. A first version described comment edits in a formatting-only diff, so the sentence is checked against the change: names in backticks, qualified or camel-case identifiers and capitalized words in mid-sentence must all appear in the diff, the failure or the title. Otherwise the sentence is dropped and the note falls back to the list of edited files. Merges and formatting get a fixed sentence and no model call. On the recorded run Nano's note was accepted for some pull requests and dropped for others; the trace of each pull request shows which.

## What the receipt compares

The receipt prices every call at the model's own rate and, next to it, the same tokens at the largest model's rate. That second figure is a price comparison. It is not a claim that the largest model would have fixed the pull request, and the measurements above say it would not always.
