# Upkeep

[![ci](https://github.com/aalvaropc/upkeep/actions/workflows/ci.yml/badge.svg)](https://github.com/aalvaropc/upkeep/actions/workflows/ci.yml)

Upkeep keeps GitHub pull requests mergeable. It works the whole queue of open pull requests:
- plans the order they can merge in;
- fixes what a bounded policy allows, with NVIDIA Nemotron models on Nebius Token Factory;
- proves every fix with the compiler and the tests;
- remembers the fixes that passed;
- hands the rest to a person with a diagnosis.

Every pull request ends with a receipt that says what each model did and what it cost.

It is one Go binary with its dashboard built in, and it ships as a GitHub Action.

**Live demo:** https://3-146-121-173.sslip.io (the overview). The dashboard is at [`/demo`](https://3-146-121-173.sslip.io/demo).

![The dashboard after a run: the queue on the left, the merge order drawn as a graph above the results in the centre, the models and the web search on the right](docs/images/dashboard-done.png)

## How it works

For each open pull request Upkeep finds the first thing that blocks it:
- the branch is behind;
- a conflict;
- a broken build or test;
- formatting;
- a flaky test.

It then takes the cheapest fix that can work:

| Blocker | Fix |
|---|---|
| Behind or formatting | Mechanical; no model |
| Small conflict | Nemotron Ultra, in one call |
| Broken build or test | Nemotron Super, editing files through four tools for up to three attempts |
| Dependency upgrade | Tavily finds the migration guide and Super predicts what will break before the fix starts |

Nothing is pushed until a checklist outside the model passes: build, vet and tests, at most 5 files and 80 lines, no test touched, no comment added. Before any fix it plans the merge order from the files the pull requests share, and after the run it flags the ready ones that the run's own merges made stale. [`docs/architecture.md`](docs/architecture.md) has the pipeline; [`docs/policy.md`](docs/policy.md) has every rule.

While a run goes, the centre of the dashboard follows the agent at work, one pull request at a time: its five stages, every Nemotron turn and tool call, the Tavily search and the verifier.

![The live stage following the jwt major upgrade: its stages, the other workers in parallel, and Super's attempt turn by turn with its edits](docs/images/dashboard-trace.png)

## The demo scenario

[`upkeep-sample`](https://github.com/aalvaropc/upkeep-sample) is a small Go service. The demo builds nine pull requests on it from recipes, one blocker each, in a private git repository per session:

| # | Pull request | Blocker | Result |
|---|---|---|---|
| 1 | Search notes | behind `main` | the clean merge of `main`, delivered in a patch comment |
| 2 | Longer titles (`upkeep` label) | one-line conflict | resolved by Ultra, pushed |
| 3 | Tag counts | conflicting imports | resolved by Ultra; the resolution is delivered in a patch comment |
| 4 | Notes export (`upkeep` label) | `main` renamed a method it calls | fixed by Super, pushed |
| 5 | jwt v4 to v5 (bot, major) | build fails | fixed by Super and pushed; ready to merge with low confidence, and flagged stale after #7 merged |
| 6 | cors patch bump (bot) | none | merged, no model call |
| 7 | clockwork minor bump (bot) | build fails | fixed by Super, pushed and merged |
| 8 | Note summary | formatting, and a test that depends on order | formatted, patch attached; the order-dependent test reported and left alone |
| 9 | Auth rewrite | four overlapping conflict hunks | handed back with Ultra's diagnosis |

## Evidence

- **Replay**, run by CI on every change: 9 of 9 for $0.0191 and 29 model calls. [`docs/evals/latest.md`](docs/evals/latest.md)
- **Live:** 8 runs against the real models, 72 of 72 pull requests as expected, two to four cents a run. [`docs/evals/live.md`](docs/evals/live.md)
- **Learned fixes:** "Run again" reapplies the three fixes Super made with zero model calls, and the run costs $0.0055.
- **Real pull requests:** on the three pull requests Dependabot opened on `upkeep-sample`, all merged. [`docs/evals/action.md`](docs/evals/action.md)
  - The clockwork 0.5.0 bump broke the build; Super fixed it in one attempt for $0.0017.
  - The Action brought the other two up to date and saw they conflict in `go.mod`; Ultra resolved the second one's conflict for $0.0006.

## Run it

You need Go 1.26, Node.js 22 (the exact version is in `web/.tool-versions`) and Docker. Replay needs no key:

```sh
make build
./bin/upkeep demo
```

Open http://127.0.0.1:8080/demo and press **Work the queue**. The recording built into the binary answers every model call; git, the sandbox, the compiler and the tests run for real.

With keys, the same page runs live:

```sh
export NEBIUS_API_KEY=...        # https://tokenfactory.nebius.com
export TAVILY_API_KEY=...        # optional: web search for migration guides
UPKEEP_MODE=live ./bin/upkeep demo
```

On a real repository (live by default):

```sh
./bin/upkeep run -repo owner/name -dry-run      # detect and fix in scratch copies, change nothing
GITHUB_TOKEN=... ./bin/upkeep run -repo owner/name
```

From GitHub Actions: `uses: aalvaropc/upkeep@v0.1.0` ([`docs/action.md`](docs/action.md)).

| Command | What it does |
|---|---|
| `upkeep demo` | the overview at `/` and the dashboard at `/demo`, with a private session of the scenario per visitor |
| `upkeep run -repo owner/name` | works the open pull requests of a GitHub repository (`-dry-run` changes nothing, `-pr 9,11` picks pull requests) |
| `upkeep eval [-runs N]` | runs the scenario and writes the outcomes and costs under `docs/evals` |
| `upkeep record -fixtures DIR` | runs the scenario with the real models and saves every exchange |
| `upkeep scenario build -out DIR` | builds the scenario's repository from its recipes |
| `upkeep serve` | the bare dashboard and event stream |

## Nemotron, Token Factory and Tavily

| Model | Job |
|---|---|
| Ultra 550B | small conflicts in one call, and the diagnosis for a person |
| Super 120B | the tool loop that fixes builds and tests, and the forecast of an upgrade |
| Nano 30B | the one-sentence note for the reviewer, checked against the diff |

The routing comes from 73 measured runs. [`docs/models.md`](docs/models.md) has the numbers, the prices and what did not work.

Where Token Factory made the work faster:

- **Routing by measurement.** Three model sizes behind one key and one OpenAI-compatible API let us run the 73 experiments (3.06 million tokens, about two dollars) in a day and route each step on evidence instead of guesses.
- **Live prices.** `GET /v1/models?verbose=true` returns each model's price, so every receipt is priced from the catalog and the comparison with the largest model is computed, not assumed.
- **No infrastructure to run.** Inference needs only a key: the demo machine runs the Go binary and the build sandbox, and parallel runs never came near the rate limits.
- **Reasoning switch.** `chat_template_kwargs: {"enable_thinking": false}` turns reasoning off for the tool loop, where it measured slower and less accurate.

Tavily runs on every live forecast of a minor or major upgrade. When neither the release notes nor the search have anything, the forecast compares the module's exported API between the two versions instead, with no model.

## Documentation

- [`docs/architecture.md`](docs/architecture.md): the pipeline, the merge plan, learned fixes, the forecast, the sandbox, events, record and replay.
- [`docs/policy.md`](docs/policy.md): the rules, the verifier and the settings.
- [`docs/models.md`](docs/models.md): which model does what, and the measurements behind it.
- [`docs/limitations.md`](docs/limitations.md): what Upkeep does not do and where it can be wrong.
- [`docs/action.md`](docs/action.md): the GitHub Action.
- [`docs/evals`](docs/evals): the replay, the live runs and the Action on real pull requests.
- [`web/DESIGN.md`](web/DESIGN.md): the dashboard's design system.
- [`deploy/README.md`](deploy/README.md): running the public demo on one machine.

## Configuration

Settings come from the environment; [`.env.example`](.env.example) lists them all.

| Variable | Default | Meaning |
|---|---|---|
| `NEBIUS_API_KEY` | none | Token Factory key; required in `live` and `record` modes |
| `NEBIUS_BASE_URL` | `https://api.tokenfactory.nebius.com/v1` | Token Factory endpoint |
| `UPKEEP_MODEL_ULTRA`, `_SUPER`, `_NANO` | the three Nemotron 3 models | which model plays which role |
| `TAVILY_API_KEY` | none | web search for migration guides |
| `UPKEEP_SEARCH_DAILY_LIMIT` | `100` | live searches per day; `0` turns the search off |
| `UPKEEP_PRICE_TAVILY_SEARCH` | none | USD per search, for the receipt |
| `GITHUB_TOKEN` | none | needed to push to, comment on and merge pull requests |
| `UPKEEP_MODE` | `replay` (`live` for `upkeep run`) | `live` calls the models, `record` also saves every exchange, `replay` answers from recorded ones |
| `UPKEEP_DAILY_TOKENS` | `1000000` | model tokens per day. Once spent, the demo replays its recording and says so; `upkeep run` and the Action stop with an error |
| `UPKEEP_ADDR` | `127.0.0.1:8080` | address of the server |
| `UPKEEP_DATA_DIR` | `data` | working copies, caches, learned fixes and the scenario repository |
| `UPKEEP_SANDBOX` | `docker` | where builds and tests run; `exec` runs them on the host and is for development only |
| `UPKEEP_SANDBOX_RUNTIME` | Docker's default | `runsc` runs the sandbox under gVisor |
| `UPKEEP_PUBLIC` | `false` | marks a public deployment in `/api/info` and the log; it changes nothing else |
| `UPKEEP_PRICE_<MODEL>_INPUT`, `_OUTPUT` | from the Token Factory catalog | USD per million tokens, to override the catalog |

## Development

`make check` runs formatting, `go mod tidy`, the dependency allowlist, the no-comments rule, `go vet`, golangci-lint, the tests with the race detector, and the dashboard's formatting, types and tests, then builds. `make eval` replays the scenario and fails if any pull request ends differently. CI runs both, plus a smoke test and secret scanning.

Tests that call Token Factory or GitHub need `UPKEEP_LIVE_TESTS=1`, and tests that run containers need `UPKEEP_DOCKER_TESTS=1`.

The code has no comments; only `//go:embed` is allowed. Every pull request targets `main` and is merged with a merge commit once every check passes.

## License

MIT. See [`LICENSE`](LICENSE).
