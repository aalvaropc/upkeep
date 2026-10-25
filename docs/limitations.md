# Limitations

What Upkeep does not do, and where it can be wrong. Nothing here is hidden in the product: each item has a place in the dashboard or the receipt where it shows.

## Scope

- **Go only.** The sandbox runs `go build`, `go vet` and `go test`. Pull requests in other languages are listed and not worked.
- **Same-repository pull requests.** Pull requests from forks are skipped because Upkeep cannot push to a fork.
- **GitHub only.** The demo runs on a local forge over git; the real mode talks to GitHub.
- **Dependency forecasts need the sources.** A forecast reads GitHub releases and, with a Tavily key, the migration guide. A module without release notes gets no forecast, and the fix starts from the build output alone.

## What can be wrong

- **A fix that compiles can still be wrong.** The checks are the compiler, vet and the repository's own tests. The jwt v5 fix in the recorded run passes them; whether it handles a token without an expiry the way the old code did is not something those tests decide, which is why a major bump is never merged on its own.
- **Conflict resolutions are judged by tests, not by intent.** In the scenario, two branches disagree about a title limit; Ultra picks one and the pull request's own test decides whether the pick survives. Where no test covers the disagreement, the resolution is a guess a person should read.
- **The flaky-test check is a heuristic.** A test that fails in the full run and passes alone is reported as flaky. It cannot tell a flaky test from an order dependence (the scenario's flaky test is exactly that) or from a real bug that only shows in the full run. It is reported, never hidden and never fixed.
- **The reviewer's note is checked, not guaranteed.** The check drops notes that name things not in the change, but a note can still be vague.
- **Forecasts are predictions.** Each is marked confirmed or not seen by the build afterwards; in the recorded run some were confirmed and some were not.
- **The price comparison is not a quality claim.** The "largest model's price" line prices the same tokens at the largest model's rate. It does not say that model would have fixed the pull request.

## Operations

- **One recorded run.** The recording behind replay is one run of the nine-pull-request scenario. Live runs vary between attempts: in earlier runs the large conflicts, the title-limit conflict and the jwt major each needed a different number of attempts. `upkeep eval -runs N` against live models measures that spread.
- **Sessions do not survive a restart** of the demo server, and the demo is limited to 50 sessions at a time and two runs at once.
- **The token budget is per server and per day (UTC).** When it is spent, runs replay the recording and say so.
- **The sandbox trusts Docker.** Builds run with no network, no capabilities, a read-only root and a non-root user, but a Docker escape would not be stopped by Upkeep.
