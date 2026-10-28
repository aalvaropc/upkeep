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

## Learned fixes

When Super's repair passes the verifier, the engine remembers it in a `learn.Book`: the edits that worked, the patch, Nano's note and what the pull request cost. The key is the repository, the module and its from and to major versions, and a signature of the failure: the `file: message` lines of the build or test output, sorted, with line and column numbers dropped, so the same break on other lines has the same key. The next time that key comes back, the engine applies the stored edits first (`fixer.ApplyEdits`, with the same rules as `apply_edit`, all or nothing), runs the checks and, if they pass, commits and goes on to the same verifier as any other fix; the note is reused when the patch is identical. That path calls no model, does not count as an attempt, and a dependency jump the book already knows is not forecast unless the stored fix fails. A stale fix (an edit no longer matches, or the checks still fail) is undone and the normal repair runs with the same failure. A fix the verifier rejects is never remembered.

The demo keeps the book per session; Run again creates the new session with `{"inherit": "<old id>"}`, which copies it, and only then deletes the old one. `upkeep eval` never inherits, so every evaluation starts empty. `upkeep run` keeps the book as JSON under `<data dir>/learned` (`-learn=false` turns it off).

## Packages

| Package | Job |
|---|---|
| `cmd/upkeep` | the commands, the shared setup |
| `internal/engine` | `Processor.Process`, the state of a pull request, the result with its receipt |
| `internal/queue` | the worker pool and the actions: push, patch, merge, comment |
| `internal/detect` | the order of checks that names the blocker |
| `internal/strategy` | one file per kind of fix: merge, format, conflict (Ultra), repair (Super), explain (Nano) |
| `internal/fixer` | Super's tool loop and the tools, with the rules enforced by code |
| `internal/learn` | learned fixes: the failure signature, the key, the book in memory or on disk |
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

## Events and the dashboard

Every step emits an event into a per-session ring buffer (`internal/events`): `run.queued`, `run.started`, `pr.blocker`, `pr.step`, `model.call`, `tool.call`, `verify.result`, `action`, `receipt`, `pr.done`, `notice`, `run.finished`, `session.closed`. The server streams them over Server-Sent Events with resume, a gap notice and keepalives. The dashboard's reducer (`web/src/board.ts`) is a pure function from events to board state, so reloading the page replays the run from the ring.

## Record and replay

Everything that leaves the process goes through an interface with a live, a record and a replay implementation: model calls (`model.Recorder`, `model.Replayer`), release notes and search (inside the forecaster, cached with the forecast and its cost). A recorded exchange is keyed by a hash of the normalized request (durations, temporary paths and cache markers are removed), so a replay is exact as long as the code sends the same prompts. A request with no recording fails honestly; it never invents an answer. The recording of the nine-pull-request scenario is embedded in the binary (`scenarios/notes/fixtures`), and CI replays it against the real sample repository.

## The sandbox

Builds and tests run in `golang:1.26.10-bookworm` with `--network none`, all capabilities dropped, a read-only root with a tmpfs for `/tmp`, the caller's user ID and one CPU and 768 MB. A shared cache directory holds the module and build caches. Module downloads happen in a separate step with network access, before the offline build. The report of a check is cached by the tree's content hash, so an unchanged tree costs nothing the second time.
