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

- **One recorded run, five live ones.** The recording behind replay is one run of the nine-pull-request scenario. Live runs vary: across five live runs every pull request ended as expected, but the cost ranged from $0.020 to $0.037 and the jwt major needed one attempt in four runs and two in one (`docs/evals/live.md`). Earlier development runs were less tidy: two conflict resolutions came back malformed and were handed back, and the jwt major once used all three attempts without passing. The code now repairs the malformed answer, asks again once, and gives the fixer more turns and tokens. Five runs is a small sample, not a guarantee; `upkeep eval -runs N` with `UPKEEP_MODE=live` measures it again.
- **Sessions do not survive a restart** of the demo server, and the demo is limited to 50 sessions at a time and two runs at once.
- **The token budget is per server and per day (UTC).** When it is spent, runs replay the recording and say so. Before every live run the server also makes a one-token check call to Token Factory; when it fails (no credit left, a revoked key, the service down), that run replays the recording and says so too. A failure in the middle of a live run is reported, not replayed.
- **The sandbox trusts Docker.** Builds run with no network, no capabilities, a read-only root and a non-root user, but a Docker escape would not be stopped by Upkeep.
