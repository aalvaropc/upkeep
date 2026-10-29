# Architecture

One Go binary is the command line, the HTTP server and the dashboard. It uses the standard library for almost everything.

## The path of one pull request

```
forge ──► Prepare (clone; check out the branch, or pull/N/head for a fork)
          │
          ▼
        forecast (minor or major dependency bump) ──► release notes + Tavily ──► Super predicts what breaks
          │                                       └─► nothing found ──► compare the module's exported API
          ▼
        detect ──► no go.mod ──► hand back, no model
          ├─► conflict (small) ──► Ultra resolves it in one call ────────────────────────┐
          ├─► conflict (large) ──► Ultra diagnoses ──► hand back                          │
          ├─► build / test fails ──► learned fix? ──► Super repairs, up to 3 attempts ────┤
          ├─► formatting ──► gofmt ───────────────────────────────────────────────────────┤
          ├─► behind ──► merge the base ──────────────────────────────────────────────────┤
          └─► clean or flaky ─────────────────────────────────────────────────────────────┤
                                                                                           ▼
                                   detect again ──► verify (outside the model) ──► Nano writes the note
                                                                                           │
                                                                                           ▼
             act: push (label or bot) │ patch comment │ merge (bot patch or minor) │ diagnosis comment
```

- `engine.Processor.Process` is that loop. It re-detects after every fix. It hands the pull request back when the same blocker comes back a third time, or after six rounds.
- `queue.Runner` lists the open pull requests and announces the ones it skips, with the reason: drafts, other target branches, deleted forks, and forks without the docker sandbox. It prepares every working tree before the first merge can move the base, plans the merge order, and runs the loop with four workers by default. Then it applies each result through the forge.

### Delivery

| Situation | What happens |
|---|---|
| Upkeep may push (a bot's pull request, or the `upkeep` label) | It pushes the fix: merges and resolutions are merge commits, fixes are commits |
| A person's branch without the label, or a fork | The fix goes in a patch comment. The comment says to merge the base when a merge was needed, shows each conflict region as Ultra resolved it, and then the diff of the fix, if there is one |
| Every pull request | One receipt comment (what each model did and cost), updated in place on later runs |

## Detect

`detect.Detector` merges the base into the working tree without committing, then checks in this order:

1. No `go.mod` at the root: unsupported. The pull request is handed back with a fixed reason, and no model is asked.
2. A merge with conflicts: small (at most three hunks of twenty lines) or large.
3. `go mod download`, `go build` or `go vet` fails: build.
4. Tests fail: the failing tests run alone once.
   - If they pass alone, the whole suite runs twice more. Failing both times means they depend on the order tests run in (`order-dependent`); otherwise they are `flaky`.
   - If they fail alone, it is a test failure.
5. Files are not formatted: format.
6. The branch is behind the base: behind.
7. Flaky tests were seen: flaky. This never hides a real blocker.
8. Otherwise: clean.

## Forecast

For a minor or major dependency bump, the forecaster runs before detection. The exception is a jump that the learned fixes already know: its forecast waits until the stored fix fails. Patch bumps are never forecast.

How the forecast is written:
1. It reads the release notes between the two versions from GitHub (cached for 24 hours).
2. With a Tavily key, it searches for the migration guide. Only pages from the module's own project are kept.
3. Super lists the identifiers the project uses that will break. Every prediction must appear in the sources, otherwise it is dropped and counted.
4. When there are no sources at all, the forecaster downloads both versions of the module in the sandbox. It compares the exported API on the host, for the identifiers the project uses: what was removed, what changed signature or definition. No model is asked.

A forecast is cached per module and version jump.
- In `live` mode, Tavily is still asked on every run, within `UPKEEP_SEARCH_DAILY_LIMIT`. The trace shows what it found, including pages the cached forecast did not read, but the cached forecast is kept, because a forecast written from weaker pages can miss a break.
- A failed or skipped search falls back to the cached forecast and says why.

Afterwards each prediction is marked:
- **confirmed by the build:** the first failure names it;
- **changed by the fix:** a line the fix removed names it;
- **not seen in the build;**
- **the build passed:** when the bump built cleanly.

## Learned fixes

When Super's repair passes the verifier, the engine stores it in a `learn.Book`:
- the edits, the patch, Nano's note and what the pull request cost;
- under a key of the repository, the module, its from and to major versions, and the signature of the failure (the `file: message` lines of the build or test output, sorted, without line and column numbers).

When the key comes back:
- The engine applies the stored edits first. It uses the same rules as `apply_edit`, all or nothing, and calls no model. It then runs the checks and the same verifier as any other fix.
- A stale fix (an edit no longer matches, or the checks still fail) is undone, and the normal repair runs.
- A fix the verifier rejects is never stored.

Where the book lives:
- The demo keeps it per session. "Run again" copies it into the next session.
- `upkeep eval` always starts empty.
- `upkeep run` keeps it as JSON under `<data dir>/learned` (`-learn=false` turns it off).

## Merge plan

`plan.Planner` runs once every working tree is prepared:
1. It lists the files each pull request changes (`git diff --name-only` against the base).
2. For every pair that shares a file, it asks `git merge-tree --write-tree` whether the two heads merge. Nothing is checked out and no model is called.
3. It orders small bot bumps first, then pull requests that share no files, then those that share files cleanly, then the conflicting ones with the smaller change first, each with a reason.

The plan is advice: the workers still take the pull requests in number order. After the workers finish, the base is fetched again for every ready pull request, and any that now conflict are reported as stale, naming the merge that caused it.

In the scenario, #7 (clockwork) merges during the run and #5 (jwt) then conflicts with `main` in `go.mod` and `go.sum`.

## Merge confidence

`confidence.Assess` rates every ready pull request high, medium or low, with the facts behind it:
- who wrote the code, and on which attempt;
- the size of the change;
- a major upgrade;
- the forecast marks;
- the tests the pull request adds, which passed in the full run;
- a flaky or order-dependent test;
- a conflict with a pull request that merges first.

It informs and never gates a merge.

## The sandbox

Builds and tests run in `golang:1.26.10-bookworm`:

| Constraint | Setting |
|---|---|
| Network | `--network none` |
| Capabilities | all dropped, `no-new-privileges` |
| Filesystem | read-only root, with a tmpfs for `/tmp` |
| User | the caller's user ID |
| Resources | one CPU, 768 MB and 512 processes |
| Time | 120 seconds per command |
| Runtime | optional (`UPKEEP_SANDBOX_RUNTIME=runsc` for gVisor) |

- Module downloads happen in a separate step with network.
- A shared cache directory holds the module and build caches.
- A check's report is cached by the tree's content hash.
- Notices the Docker client prints about its own configuration are dropped from the output, so the build output a model reads is the same everywhere.

## Events and the dashboard

Every step emits an event into a per-session ring of 4,000 events (`internal/events`):

`run.queued`, `run.started`, `run.plan`, `pr.blocker`, `pr.step`, `model.call`, `search.call`, `tool.call`, `verify.result`, `action`, `receipt`, `pr.done`, `notice`, `run.finished`, `session.closed`

- The server streams them over Server-Sent Events, with resume, a gap notice and keepalives.
- The dashboard's reducer (`web/src/board.ts`) is a pure function from events to state, so a reload replays the run from the ring.
- Model and tool events carry the step, the attempt and turn, what the model said, the tools it asked for, and the file or symbol each tool touched.

`upkeep demo` serves:
- the overview page at `/`;
- the dashboard at `/demo`: the queue, the merge order as a graph, a live stage that follows one pull request until it finishes (with the other workers beside it), the results, and a detail sheet with the trace and the summary. [`web/DESIGN.md`](../web/DESIGN.md) describes it.

## Record and replay

Everything that leaves the process goes through an interface with a live, a record and a replay implementation:
- model calls (`model.Recorder`, `model.Replayer`);
- release notes and search, inside the forecaster, cached with the forecast.

A recorded exchange is keyed by a hash of the normalized request (durations, temporary paths and cache markers removed). So a replay is exact as long as the code sends the same prompts, and a request with no recording fails instead of inventing an answer. The recording of the nine-pull-request scenario is embedded in the binary (`scenarios/notes/fixtures`), and CI replays it against the real sample repository.

## Packages

| Package | Job |
|---|---|
| `cmd/upkeep` | the commands and their shared setup |
| `internal/engine` | `Processor.Process`, the state of a pull request, the result |
| `internal/queue` | the worker pool, skipped pull requests, the plan check after the run, and the actions: push, patch, merge, comment |
| `internal/detect` | the order of checks that names the blocker |
| `internal/strategy` | one file per kind of fix: merge, format, conflict (Ultra), repair (Super), explain (Nano) |
| `internal/fixer` | Super's tool loop and its tools, with the rules enforced by code |
| `internal/verify` | the checklist after every fix |
| `internal/policy` | the rules and the TOML reader |
| `internal/plan` | the merge plan |
| `internal/confidence` | the merge confidence |
| `internal/learn` | learned fixes: the failure signature, the key, the book |
| `internal/forecast` | release notes, search, Super, the API comparison, the grounding filter, the marks |
| `internal/tavily` | the Tavily client |
| `internal/model` | the Token Factory client, pricing, record and replay |
| `internal/runtime` | builds the engine for live, record or replay; the daily token budget and the search limit |
| `internal/sandbox` | the Docker and host executors, the Go runner, the report cache |
| `internal/gitx` | git operations; there is no force-push |
| `internal/forge` | the local forge (a bare repository per session) and the GitHub forge; bump detection from branch names and titles |
| `internal/github` | the GitHub REST and GraphQL client |
| `internal/pull` | pull requests, blockers and outcomes |
| `internal/receipt` | what each model did and cost |
| `internal/demo` | sessions, the shared queue of runs, snapshots and detail views |
| `internal/scenario`, `scenarios` | the scenario: recipes as patch files, a reproducible builder, the embedded recording |
| `internal/eval` | runs the scenario and checks every outcome |
| `internal/events` | events and the per-session ring |
| `internal/web`, `web` | the HTTP API, the event stream and the React dashboard |
| `internal/config` | settings from the environment |
| `internal/fixture` | the file stores behind recordings and caches |
| `internal/store` | the schema of an audit database; `serve` opens it, nothing writes to it yet |
| `internal/gocomments`, `internal/jsonfix`, `internal/buildinfo` | the comment scanner, repair of near-JSON answers, the version string |
| `internal/tools/nocomments` | the no-comments check run by `make check` |
