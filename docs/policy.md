# Policy

What Upkeep may do on its own is decided by a policy that code enforces. A model proposes; the policy and the verifier dispose.

## Hard rules

These cannot be configured. A policy file that names them is rejected.

- Upkeep never force-pushes. Pushes are fast-forward only, and the branch name of a push may not start with `+` or `-` or contain a colon or a space. Fixing a stale branch means merging the base into it, never rewriting it.
- Upkeep never merges a pull request opened by a person.
- A fix may never edit, delete or skip a test. The editing tool refuses test files and the verifier checks the final diff for any test change.
- Merges use a merge commit.

## What it does when

| Situation | Action |
|---|---|
| a fix exists and the pull request is a bot's, or has the `upkeep` label | push the fix to the branch |
| a fix exists and the pull request is a person's without the label | attach the fix as a patch comment |
| the pull request is a bot's patch or minor dependency bump and every check passes | merge it (with GitHub auto-merge when the repository has required checks) |
| the pull request is a major bump | repair it, never merge it |
| the conflict is larger than three hunks or any hunk longer than twenty lines | do not touch it; post Ultra's diagnosis |
| three attempts, 180 seconds or 300,000 tokens are used up | stop, restore the branch and post a diagnosis |
| a test fails in the full run and passes alone | report it as flaky and leave it alone |

## What a fix must satisfy

The verifier computes these from the final diff and the container's output, outside any model:

1. `go build` passes
2. `go vet` passes
3. the tests pass
4. at most 5 files changed
5. at most 80 lines changed
6. no test file edited, deleted or skipped
7. only Go files changed (`go.mod` and `go.sum` are allowed for dependency bumps)
8. no comment added
9. the code is formatted

When any check fails, nothing is pushed, the branch is restored and the pull request is handed back with the reason.

## Configuring it

A repository can override the defaults with an `upkeep.toml` at its root; `examples/upkeep.toml` shows every setting.

| Setting | Default | Meaning |
|---|---|---|
| `label` | `upkeep` | the label that lets Upkeep push to a person's branch |
| `bots` | `dependabot[bot]`, `renovate[bot]` | accounts treated as bots, besides any GitHub account of type Bot |
| `auto_merge` | `bots` | `bots` or `none` |
| `auto_merge_bumps` | `patch`, `minor` | bump levels that may merge on their own |
| `max_files_changed` | 5 | the size limit of a fix |
| `max_lines_changed` | 80 | the size limit of a fix |
| `max_conflict_hunks` | 3 | the largest conflict Ultra will resolve |
| `max_hunk_lines` | 20 | the longest hunk Ultra will resolve |
| `max_attempts` | 3 | fix attempts per pull request |
| `max_seconds` | 180 | wall time for fixing one pull request |
| `max_tokens` | 300,000 | model tokens for fixing one pull request |

The dashboard's "What Upkeep may do" panel shows the rules the running server was started with, in plain words.
