# Upkeep as a GitHub Action on real Dependabot pull requests

On 2026-10-29 the Upkeep Action (`aalvaropc/upkeep@v0.1.0`) was installed in [`upkeep-sample`](https://github.com/aalvaropc/upkeep-sample) ([workflow](https://github.com/aalvaropc/upkeep-sample/blob/main/.github/workflows/upkeep.yml)) and dispatched on the two pull requests Dependabot had opened there: [#8](https://github.com/aalvaropc/upkeep-sample/pull/8) (rs/cors 1.11.0 to 1.11.1) and [#11](https://github.com/aalvaropc/upkeep-sample/pull/11) (oklog/ulid 2.1.0 to 2.1.2). Nothing about them was staged: both were behind `main`.

| Run | What Upkeep did | Model calls | Cost |
|---|---|---|---|
| [Dry run](https://github.com/aalvaropc/upkeep-sample/actions/runs/40801225260) | found both behind `main`, merged it in a scratch copy, passed every check; the merge plan said #8 and #11 change the same lines of `go.mod` and will conflict, so one must follow the other; pushed, commented and merged nothing | 0 | $0 |
| [Run](https://github.com/aalvaropc/upkeep-sample/actions/runs/40801515122) | merged `main` into both branches, pushed, posted one receipt each and armed auto-merge; GitHub merged #11 when its checks passed, and #8 then conflicted with `main`, as the plan said | 0 | $0 |
| [Run on #8](https://github.com/aalvaropc/upkeep-sample/actions/runs/40801919467) | found a small conflict in `go.mod`, Ultra resolved it in one call, every check passed, pushed and armed auto-merge; GitHub merged #8 | 1 (Ultra) | $0.0006 |

Each pull request carries one Upkeep receipt comment, updated in place rather than posted again, ending with its merge confidence (`high`: no code written by a model, or a model resolved a small conflict).
