# Limitations

What Upkeep does not do, and where it can be wrong. Each item shows somewhere: in the dashboard, the receipt, the run's notices or the comments.

## Scope

- **Go modules only, with `go.mod` at the root.** A repository without one is handed back at once with that reason, and no model is asked.
- **GitHub only.** The demo runs on a local forge over git; the real mode talks to GitHub.
- **Not every pull request is worked.**
  - Drafts and pull requests against another branch are skipped, and the run's notices say so.
  - Forks are worked only with the docker sandbox, because their code is untrusted. Upkeep never pushes to them: the fix goes in a patch comment.
- **Bumps are recognized from their branch and title.** These are a Dependabot `go_modules` branch or a Renovate branch, one module per pull request. Patch bumps are not forecast.
  - A grouped update of several modules is worked as a bot's change: no forecast, and never merged.
- **One module, at the root.** The checks run on the module at the repository root, so a pull request that changes Go or module files inside a module in a subdirectory is handed back without a model; a Dependabot bump that names one in its title is skipped before it is cloned. A repository with a `go.work` is treated the same way, even though its workspace would let the checks reach those modules. Paths under `testdata`, `vendor`, or a directory whose name starts with `.` or `_` are not treated as modules, as the Go tool ignores them.
- **A forecast needs something to read.** It uses the release notes, a Tavily search, or, failing both, a comparison of the module's exported API that the sandbox must be able to download. A private module that the proxy cannot serve gets no forecast, and the fix starts from the build output.

## What can be wrong

- **A fix that compiles can still be wrong.** The checks are the compiler, vet and the repository's own tests. A major bump is never merged on its own, and the merge confidence says what backs each fix.
  - It names the tests the pull request adds, which passed in the full run.
  - It does not measure whether the lines Upkeep wrote are covered by any test.
- **Conflict resolutions are judged by tests, not by intent.** Where no test covers the disagreement, the resolution is a guess. The patch comment and the detail view show each resolved region so a person can read it. Conflicts in tests or in files other than Go source, `go.mod` and `go.sum` are never resolved, and neither is a file deleted on one side or a binary file.
- **Flaky and order-dependent are heuristics.**
  - A test that fails in the full run and passes alone is run twice more in the full suite: it is "order-dependent" when it fails both times, "flaky" otherwise.
  - A test that fails rarely can look order-dependent, and a real bug that only shows in the full run looks the same.
  - Either way it is reported and never fixed.
- **The reviewer's note is checked, not guaranteed.** A note that names things missing from the change is dropped, but a note can still be vague.
- **Forecasts are predictions.** Each one is marked as confirmed by the build, changed by the fix, not seen, or "the build passed".
- **A forecast is not refreshed on its own.** Once written, it is kept even when a later search finds new pages, because pages vary from day to day and a forecast written from weaker ones once missed a break. The new pages are shown.
- **The price comparison is not a quality claim** (see [`models.md`](models.md)).

## Operations

- **The live evidence is eight runs** (72 of 72 pull requests as expected; [`evals/live.md`](evals/live.md)). Live runs vary:
  - the jwt major needed a second attempt in one run;
  - outside those runs, in one hosted demo run on 29 October, Super did not fix the jwt major within its 300,000-token repair budget: after three attempts it went to a person with Ultra's diagnosis, and that run cost five cents;
  - #4 averaged two attempts in the last three;
  - the cost per run ranged from two to four cents.
  - Run `upkeep eval -runs N` with `UPKEEP_MODE=live` to measure again.
- **The sandbox environment is fixed:**
  - Go 1.26.10 with `GOTOOLCHAIN=local` and cgo off;
  - one CPU, 768 MB and 120 seconds per command.
  A repository that needs cgo or a newer toolchain fails to build, and a test that runs out of time counts as a failing test.
- **The sandbox trusts its container runtime.**
  - Builds run with no network, no capabilities, a read-only root and a non-root user, but a container escape is not stopped by Upkeep. `UPKEEP_SANDBOX_RUNTIME=runsc` adds gVisor.
  - Within a run, the module and build caches are shared and writable, so one pull request's tests could change what a later one builds from.
- **The attempt, time and token limits apply to Super's repair.** The conflict resolution, the diagnosis, the forecast and the note are single calls outside those limits.
- **The token budget is per data directory and per day (UTC).**
  - Each live run starts with a one-token check call, and once the budget is spent the next model call is refused.
  - In the demo, a run that cannot start live replays the recording and says so.
  - `upkeep run` and the Action stop with an error instead.
- **Demo sessions live in memory:**
  - they do not survive a restart, and expire after two idle hours;
  - when the server holds its maximum (`-max-sessions`, 50), the oldest idle one is dropped;
  - two runs go at once by default (`-runs`), four pull requests each.
