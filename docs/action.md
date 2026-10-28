# GitHub Action

`aalvaropc/upkeep` runs `upkeep run` inside a GitHub Actions job. It installs the binary with `go install`, works the open pull requests of the repository with the live models, and writes the run's summary table to the job summary. Use it on a schedule, from the Actions tab, or from a `/upkeep` comment on a pull request.

## Install it

1. Create the secrets (Settings, Secrets and variables, Actions):

   | Secret | Required | What it is |
   |---|---|---|
   | `NEBIUS_API_KEY` | yes | a Nebius Token Factory key (https://tokenfactory.nebius.com) |
   | `UPKEEP_GITHUB_TOKEN` | yes | a token that can push, comment and merge; see [Which token](#which-token) |
   | `TAVILY_API_KEY` | no | web search for migration guides; without it the forecast reads the GitHub release notes only |

2. Check the repository settings:

   - **Allow auto-merge** is on (Settings, General, Pull Requests). Upkeep merges a bot's patch or minor bump by arming auto-merge, so GitHub merges it once the required checks pass.
   - The branch protection or ruleset of the default branch has required status checks. Without them auto-merge has nothing to wait for, and Upkeep merges directly when the pull request is clean.
   - The `upkeep` label exists (`gh label create upkeep`). Upkeep pushes a fix to a person's branch only when the pull request carries it. Otherwise it attaches the fix as a patch comment. The label name can be changed in the policy.

3. Add `.github/workflows/upkeep.yml`:

```yaml
name: upkeep

on:
  workflow_dispatch:
    inputs:
      pull-requests:
        description: Pull requests to work, as numbers separated by commas; empty works every open one
        required: false
        default: ""
      dry-run:
        description: Push, comment and merge nothing
        type: boolean
        default: false
  schedule:
    - cron: "37 5 * * *"
  issue_comment:
    types: [created]

permissions:
  contents: write
  pull-requests: write

jobs:
  queue:
    if: github.event_name != 'issue_comment'
    runs-on: ubuntu-24.04
    timeout-minutes: 60
    concurrency:
      group: upkeep
      cancel-in-progress: false
    steps:
      - uses: aalvaropc/upkeep@v0.1.0
        with:
          pull-requests: ${{ inputs.pull-requests }}
          dry-run: ${{ inputs.dry-run || false }}
          nebius-api-key: ${{ secrets.NEBIUS_API_KEY }}
          tavily-api-key: ${{ secrets.TAVILY_API_KEY }}
          github-token: ${{ secrets.UPKEEP_GITHUB_TOKEN }}

  comment:
    if: >-
      github.event_name == 'issue_comment' &&
      github.event.issue.pull_request &&
      startsWith(github.event.comment.body, '/upkeep') &&
      contains(fromJSON('["OWNER", "MEMBER", "COLLABORATOR"]'), github.event.comment.author_association)
    runs-on: ubuntu-24.04
    timeout-minutes: 30
    concurrency:
      group: upkeep
      cancel-in-progress: false
    steps:
      - uses: aalvaropc/upkeep@v0.1.0
        with:
          pull-requests: ${{ github.event.issue.number }}
          nebius-api-key: ${{ secrets.NEBIUS_API_KEY }}
          tavily-api-key: ${{ secrets.TAVILY_API_KEY }}
          github-token: ${{ secrets.UPKEEP_GITHUB_TOKEN }}
```

The schedule works every open pull request once a day. Pick your own off-hour minute so your run does not start at the same moment as everyone else's. **Run workflow** in the Actions tab works all of them, or only the numbers you give, and can be a dry run. A comment that starts with `/upkeep` on a pull request works that pull request, but only when it comes from the repository's owner, a member of its organization or a collaborator. A comment from anyone else starts nothing. Both jobs share one concurrency group, so two runs never push to the same branch at once: a second run waits for the first.

Pin the action to a release tag or a commit. By default the binary is built at the same ref as the action: `go install github.com/aalvaropc/upkeep/cmd/upkeep@<ref>`, or a build of the action's own source when the ref is a branch with a slash in its name, which Go cannot install. Set the `version` input only to try another version.

## Inputs

| Input | Default | Meaning |
|---|---|---|
| `repo` | the repository running the workflow | repository to work, as `owner/name` |
| `pull-requests` | all open | numbers separated by commas, passed as `-pr` |
| `dry-run` | `false` | detect and fix in scratch copies, but push, comment and merge nothing |
| `policy` | the built-in policy | path to a policy file in the workspace; check out the repository first if it lives there |
| `version` | the action's ref, else `latest` | the Upkeep version that `go install` builds |
| `nebius-api-key` | none, required | Token Factory key |
| `github-token` | none, required | token that pushes, comments and merges |
| `tavily-api-key` | none | web search for migration guides |

The job needs a Linux runner with Docker: builds and tests run in a `golang` container without network access, and none of the secrets are passed into it. The `cache` and `learned` folders of Upkeep's data directory (build and forecast caches, the daily token count, and the fixes Upkeep learned, which it reapplies without a model call when the same failure returns) are kept between runs with the Actions cache, keyed by repository.

## Which token

Use a fine-grained personal access token or a GitHub App installation token (for example from `actions/create-github-app-token`), not the workflow's own `GITHUB_TOKEN`. Commits pushed with the workflow's token do not start other workflows. A fixed branch would then never get its checks, and an armed auto-merge would wait forever.

A fine-grained token needs access to this repository only, with these repository permissions:

- **Contents**: read and write, to push fixes and merge.
- **Pull requests**: read and write, to list pull requests, comment and arm auto-merge.
- **Issues**: read and write, because comments on a pull request go through the issues API.

The `permissions` block in the workflow sets what the workflow's own token may do. It matters when you pass that token, which is fine for a dry run (`github-token: ${{ github.token }}`). With a personal access token or an App token, that token's own permissions apply, and the block can be lowered to `contents: read`.

## Do not run it from Dependabot events

Do not trigger the workflow from `pull_request` or `push` events that Dependabot causes. Workflows started by Dependabot get no Actions secrets, so the run would fail without a model key. The daily schedule already covers Dependabot's and Renovate's pull requests. Triggering on every pull request event would also start a run for each push Upkeep itself makes.

## What it does and does not do

Upkeep follows the same [policy](policy.md) as on the command line. It never force-pushes: pushes are fast-forward only. It never merges a pull request opened by a person, and never merges a major bump. It pushes a fix only to a bot's branch or to a branch with the `upkeep` label. Everything else gets a patch comment or a diagnosis. It keeps one receipt, patch and diagnosis comment per pull request and edits them on later runs. It does not post new ones each time.

## Costs

- **Models**: cents per run. Five live runs of the nine-pull-request sample scenario cost between $0.020 and $0.037 each ([`evals/live.md`](evals/live.md)). A pull request that only needs a merge or `gofmt` costs nothing, and each receipt comment shows what its run cost.
- **Search**: at most one Tavily search per minor or major dependency bump, only when `TAVILY_API_KEY` is set. A forecast is cached, so a bump is searched once.
- **Runner minutes**: free on public repositories. On private ones the job takes the time of the builds and tests of each pull request worked, plus a minute or two to build Upkeep.
