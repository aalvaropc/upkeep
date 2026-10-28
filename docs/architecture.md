# Architecture

One Go binary is the command line, the HTTP server and the dashboard. The standard library does almost everything; the only dependency is a pure Go SQLite driver (the audit store's schema is in `internal/store`).

## The path of one pull request

```
forge ──► Prepare (clone, check out the branch)
          │
          ▼
        detect ──► conflict (small) ──► Ultra resolves in one call ──┐
          │                       └─► (large) ──► Ultra diagnoses ──► hand back
          ├─► build / test fails ──► forecast (Super) ──► Super repairs, up to 3 attempts ──┐
          ├─► formatting ──► gofmt ─────────────────────────────────────────────────────────┤
          ├─► behind ──► merge the base ────────────────────────────────────────────────────┤
          └─► clean ────────────────────────────────────────────────────────────────────────┤
                                                                                              ▼
                                       detect again ──► verify (outside the model) ──► Nano writes the note
                                                                                              │
                                                                                              ▼
                     act: push (label or bot) │ attach a patch │ merge (bot patch or minor) │ post the diagnosis
                                                                                              │
                                                                                              ▼
                                                                                  receipt comment, events
```

`engine.Processor.Process` is that loop. It re-detects after every fix and hands the pull request back when the same blocker comes back a third time or after six rounds. `queue.Runner` runs it for every open pull request with a fixed number of workers (four by default), prepares every working tree before the first merge can move the base, and applies the result through the forge.

## Packages

| Package | Job |
|---|---|
| `cmd/upkeep` | the commands, the shared setup |
| `internal/engine` | `Processor.Process`, the state of a pull request, the result with its receipt |
| `internal/queue` | the worker pool and the actions: push, patch, merge, comment |
| `internal/plan` | the merge plan: the files each pull request changes, the pairs that conflict, a suggested order, the check after the merges |
| `internal/detect` | the order of checks that names the blocker |
| `internal/strategy` | one file per kind of fix: merge, format, conflict (Ultra), repair (Super), explain (Nano) |
| `internal/fixer` | Super's tool loop and the tools, with the rules enforced by code |
| `internal/verify` | the nine-check list |
| `internal/policy` | the rules and the TOML reader |
| `internal/forecast` | release notes, search, Super, the grounding filter, confirmation against the build |
| `internal/model` | the Token Factory client, pricing, record and replay of exchanges |
| `internal/runtime` | builds the engine for live, record or replay; the daily token budget |
| `internal/sandbox` | Docker with no network, no capabilities and a read-only root; a build-cache keyed by tree contents |
| `internal/gitx` | git operations; there is no force-push |
| `internal/forge` | the local forge (a bare repository per session) and the GitHub forge |
| `internal/demo` | sessions, the shared queue, snapshots and detail views |
| `internal/scenario`, `scenarios` | the nine-pull-request scenario: recipes as patch files, a reproducible builder, the embedded recording |
| `internal/eval` | runs the scenario and checks every outcome |
| `internal/web`, `web` | the HTTP API, the event stream and the React dashboard |
| `internal/receipt` | what each model did and cost |

## Detect

`detect.Detector` merges the base into the working tree without committing, then:

1. a merge with conflicts: small (at most three hunks of twenty lines) or large
2. `go mod download` fails, or `go build` or `go vet` fails: build
3. tests fail: the failing tests are rerun alone once; if they pass they are noted as flaky and detection goes on, otherwise it is a test failure
4. files are not formatted: format
5. the branch is behind the base: behind
6. flaky tests were seen: flaky (never hides a real blocker)
7. otherwise clean

## Merge plan

Fixing pull requests one at a time misses how they collide. Once every working tree is prepared, `plan.Planner` lists the files each pull request changes (`git diff --name-only` against the base), and for every pair that shares a file it asks `git merge-tree --write-tree` whether the two heads merge. Nothing is checked out and no model is called, so a replayed run stays the same. The plan orders small bot bumps first, then the pull requests that share no files, then the ones that share files cleanly, then the conflicting ones with the smaller change first, and gives each a reason in plain words. It is advice: the workers still take the pull requests in number order.

After the workers finish, the base is fetched again into the working tree of every pull request that is ready to merge. If the base moved and `git merge-tree` now finds a conflict, the plan names the pull request as stale and which merge caused it. In the scenario, the jwt bump (#5) and the clockwork bump (#7) change adjacent lines of `go.mod` and `go.sum`; #7 merges during the run, so #5 is reported as no longer ready to merge. Both plans go out as `run.plan` events, and `upkeep run` prints them after the summary.

## Events and the dashboard

Every step emits an event into a per-session ring buffer (`internal/events`): `run.queued`, `run.started`, `run.plan`, `pr.blocker`, `pr.step`, `model.call`, `search.call`, `tool.call`, `verify.result`, `action`, `receipt`, `pr.done`, `notice`, `run.finished`, `session.closed`. The server streams them over Server-Sent Events with resume, a gap notice and keepalives. The dashboard's reducer (`web/src/board.ts`) is a pure function from events to board state, so reloading the page replays the run from the ring.

## Record and replay

Everything that leaves the process goes through an interface with a live, a record and a replay implementation: model calls (`model.Recorder`, `model.Replayer`), release notes and search (inside the forecaster, cached with the forecast and its cost). A recorded exchange is keyed by a hash of the normalized request (durations, temporary paths and cache markers are removed), so a replay is exact as long as the code sends the same prompts. A request with no recording fails honestly; it never invents an answer. The recording of the nine-pull-request scenario is embedded in the binary (`scenarios/notes/fixtures`), and CI replays it against the real sample repository.

In `live` mode a cached forecast does not skip the web search. Tavily is asked on every run (within `UPKEEP_SEARCH_DAILY_LIMIT`), and Super writes the forecast again only when a page appears that the cached one did not read; a failed or skipped search falls back to the cached forecast and says why. GitHub release lookups are cached on disk for 24 hours so an unauthenticated server stays under GitHub's hourly limit. Each search is a `search.call` event and a line of its own on the receipt, apart from the model lines, so the model cost comparison stays like for like.

## The sandbox

Builds and tests run in `golang:1.26.10-bookworm` with `--network none`, all capabilities dropped, a read-only root with a tmpfs for `/tmp`, the caller's user ID and one CPU and 768 MB. A shared cache directory holds the module and build caches. Module downloads happen in a separate step with network access, before the offline build. The report of a check is cached by the tree's content hash, so an unchanged tree costs nothing the second time.
