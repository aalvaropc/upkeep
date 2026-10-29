# Policy

What Upkeep may do on its own is decided by a policy that code enforces. A model proposes; the policy and the verifier dispose.

## Hard rules

These cannot be configured. A policy file that tries to turn one off is rejected.

- Upkeep never force-pushes. Pushes are fast-forward only, and the branch name of a push may not start with `+` or `-` or contain a colon or a space. Fixing a stale branch means merging the base into it, never rewriting it.
- Upkeep never merges a pull request opened by a person.
- A fix may never edit, delete or skip a test. The editing tool refuses test files and the verifier checks the final diff for any test change.
- Merges use a merge commit. `merge_method` may be set, but only to `merge`.
- Upkeep never pushes to a branch in a fork.

## What it does when

| Situation | Action |
|---|---|
| a fix exists and the pull request is a bot's, or has the `upkeep` label | push the fix to the branch |
| a fix exists and the pull request is a person's without the label, or comes from a fork | attach the fix as a patch comment: merge the base, the conflict regions as resolved, then the diff |
| the pull request is a bot's patch or minor dependency bump and every check passes | merge it (with GitHub auto-merge when the repository has required checks) |
| the pull request is a major bump | repair it, never merge it |
| the conflict is larger than three hunks or any hunk longer than twenty lines | do not touch it; post Ultra's diagnosis |
| the conflict is in a test file, or in a file other than Go source, `go.mod` and `go.sum` | do not touch it; post Ultra's diagnosis |
| a conflicting file has no text to resolve (one side deleted it, or it is binary) | do not touch it; hand it back naming the file, with Ultra's diagnosis of any other conflict in the same merge |
| Super's repair uses up three attempts, 180 seconds or 300,000 tokens | stop, restore the branch and post a diagnosis |
| a test fails in the full run and passes alone | report it as flaky, or as order-dependent when two more full runs fail too, and leave it alone |
| the repository has no `go.mod` at its root | hand it back without asking a model |
| the pull request is a draft, targets another branch, or comes from a fork while the sandbox is `exec` | skip it and say why in the run's notices |
| a Dependabot bump names a module in a subdirectory ("… in /tools") | skip it and say why in the run's notices |
| the pull request changes Go files, `go.mod` or `go.sum` inside a module in a subdirectory | hand it back without asking a model: Upkeep builds and tests the module at the root |

## On a real repository

- **One comment of each kind.** A pull request gets at most one receipt, one patch and one diagnosis comment from Upkeep. Each starts with a hidden marker such as `<!-- upkeep:receipt -->`, and a later run edits that comment instead of posting another.
- **Model text is published as plain text.** Text a model wrote (the note, the diagnosis, a forecast's descriptions) may quote release notes that credit people or contain markup. In comments, outside code spans, an `@` before a name gets an invisible word joiner so GitHub notifies no one, and `<` and `>` are escaped so no HTML renders. Code spans stay exactly as written.
- **Dry runs say what they skipped.** `upkeep run -dry-run` detects and fixes in scratch copies but pushes, comments and merges nothing. Every action it held back is listed as `skipped` with what it would have done, and no pull request is counted as merged.
- **Which pull requests are bumps.** A Dependabot branch under `dependabot/go_modules/` titled "Bump X from A to B" (optionally followed by "in /", "in the <name> group" or "across N directories"), or a Renovate branch under `renovate/` titled "Update X to B" (with or without "module" or "dependency", and with or without a prefix such as `build(deps):`) whose description has the line `` `A` -> `B` ``. One module per pull request: a grouped update of several modules is worked as a bot's change, with no forecast and no merge, unless it changes a module in a subdirectory, which is handed back. When a major bump moves a module whose path ends in `/vN`, the bump names the new path: `github.com/golang-jwt/jwt/v4` from v4.5.2 to v5.3.1 is read as `github.com/golang-jwt/jwt/v5`.

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

A resolved conflict is checked too. The merge commit is the base the final diff is measured from, so the conflict rule above keeps tests and other files out of Ultra's hands, and the comments a resolution adds to a Go file count toward check 8.

When any check fails, nothing is pushed, the branch is restored and the pull request is handed back with the reason.

## Configuring it

A repository can override the defaults with an `upkeep.toml` at its root; `examples/upkeep.toml` shows every setting.

| Setting | Default | Meaning |
|---|---|---|
| `label` | `upkeep` | the label that lets Upkeep push to a person's branch |
| `bots` | `dependabot[bot]`, `renovate[bot]` | accounts treated as bots, besides any GitHub account of type Bot |
| `auto_merge` | `bots` | `bots` or `none` |
| `auto_merge_bumps` | `patch`, `minor` | bump levels that may merge on their own; `major` is rejected |
| `merge_method` | `merge` | the only accepted value |
| `max_files_changed` | 5 | the size limit of a fix |
| `max_lines_changed` | 80 | the size limit of a fix |
| `max_conflict_hunks` | 3 | the largest conflict Ultra will resolve |
| `max_hunk_lines` | 20 | the longest hunk Ultra will resolve |
| `max_attempts` | 3 | fix attempts per pull request |
| `max_seconds` | 180 | wall time of Super's repair of one pull request |
| `max_tokens` | 300,000 | model tokens of Super's repair of one pull request |

The dashboard's "What Upkeep may do" panel shows the rules the running server was started with, grouped as Never, Only when and Within limits.
