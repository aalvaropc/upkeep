# Upkeep

Upkeep keeps GitHub pull requests mergeable. It finds what blocks each open pull request, fixes what a bounded policy allows, proves every fix with the compiler and the tests, and hands the rest back with a diagnosis and a receipt.

It runs on NVIDIA Nemotron models served by Nebius Token Factory.

Work in progress.

## Build and run

Upkeep is a single Go binary with the dashboard built in. You need Go 1.26 or newer and Node.js 22.12 or newer (the version is pinned in `web/.tool-versions`).

```sh
make build
./bin/upkeep serve
```

Then open http://127.0.0.1:8080. `make build` compiles the React dashboard into `internal/web/dist` and embeds it in the binary; a plain `go build` still works and serves a short notice instead of the dashboard.

To work on the dashboard with hot reload, run `./bin/upkeep serve` in one terminal and `make dev` in another, then open the address Vite prints; it forwards `/api` and `/healthz` to the server.

## Configuration

Upkeep reads its settings from the environment; `.env.example` lists every variable with its default.

| Variable | Default | Meaning |
|---|---|---|
| `NEBIUS_API_KEY` | none | Nebius Token Factory key; required in `live` and `record` modes |
| `NEBIUS_BASE_URL` | `https://api.tokenfactory.nebius.com/v1` | Token Factory endpoint |
| `UPKEEP_MODEL_ULTRA` | `nvidia/Nemotron-3-Ultra-550b-a55b` | diagnoses hard failures and large conflicts |
| `UPKEEP_MODEL_SUPER` | `nvidia/nemotron-3-super-120b-a12b` | writes fixes and resolves small conflicts |
| `UPKEEP_MODEL_NANO` | `nvidia/NVIDIA-Nemotron-3-Nano-30B-A3B` | reads release notes and writes summaries |
| `TAVILY_API_KEY` | none | web search for release notes and migration guides |
| `GITHUB_TOKEN` | none | needed to act on your own repositories |
| `UPKEEP_MODE` | `replay` | `live` calls the models, `record` calls them and saves every exchange, `replay` answers from saved exchanges with zero tokens |
| `UPKEEP_ADDR` | `127.0.0.1:8080` | address of the dashboard |
| `UPKEEP_DATA_DIR` | `data` | SQLite database and working copies |
| `UPKEEP_SANDBOX` | `docker` | where builds and tests run; `exec` runs them on the host and is only for local development |
| `UPKEEP_DAILY_TOKENS` | `1000000` | model tokens per day before live runs fall back to replay |
| `UPKEEP_SEARCH_DAILY_LIMIT` | `100` | web searches per day |
| `UPKEEP_PRICE_<MODEL>_INPUT`, `_OUTPUT` | unset | USD per million tokens, for the cost on each receipt |

## Policy

What Upkeep may do on its own is decided by a policy, not by the model. The defaults are in [`examples/upkeep.toml`](examples/upkeep.toml); a repository can override them with an `upkeep.toml` at its root. Some rules are hard and cannot be configured: Upkeep never force-pushes, never merges a pull request opened by a person, and never lets a fix edit, delete or skip tests.

## How it is built

One Go binary is both the command line and the server. The standard library does almost everything; the only dependency is a pure Go SQLite driver.

| Package | Job |
|---|---|
| `cmd/upkeep` | the `serve`, `run`, `demo`, `record`, `eval` and `version` commands |
| `internal/engine` | the queue and the state of each pull request: queued, detecting, forecasting, fixing, verifying, acting, done |
| `internal/detect` | finds what blocks a pull request, in a fixed order |
| `internal/strategy` | one fix strategy per blocker |
| `internal/verify` | the checklist every fix must pass, computed outside the model: build, vet, tests, size limits, tests untouched |
| `internal/policy` | what Upkeep may do on its own |
| `internal/model` | Nemotron on Token Factory, routed by role (Ultra, Super, Nano), with record and replay |
| `internal/forecast` | release notes and search results that predict what a dependency bump breaks |
| `internal/sandbox` | runs builds and tests in a container without network access |
| `internal/gitx` | git operations; there is no way to force-push |
| `internal/github`, `internal/tavily` | API clients |
| `internal/fixture` | stores recorded exchanges for replay |
| `internal/store`, `internal/events`, `internal/web` | SQLite audit trail, live events, the HTTP server and the event stream |
| `web` | the dashboard: React and TypeScript, built with Vite and embedded in the binary |

## Development

`make check` runs the same checks as CI: formatting, `go mod tidy`, the dependency allowlist, the no-comments rule, `go vet`, golangci-lint, the tests with the race detector, the dashboard's formatting, types and tests, and the build.

Tests that call Token Factory are skipped unless you opt in: `UPKEEP_LIVE_TESTS=1 go test ./internal/model ./internal/fixer -run Live -v` with `NEBIUS_API_KEY` set. They cost a fraction of a cent. `UPKEEP_LIVE_TESTS=1 go test ./internal/engine -run LiveMajor -v` runs the whole product on a real major dependency upgrade: it clones the sample repository, upgrades `golang-jwt/jwt` from v4 to v5 and lets Upkeep fix it; it costs about a cent and needs network access.

Tests that run containers are skipped unless you opt in: `UPKEEP_DOCKER_TESTS=1 go test ./internal/sandbox -v` needs Docker, the `golang` image from `internal/sandbox` and, for one test, network access.

The code has no comments. Names and tests carry the meaning; the only exception is the `//go:embed` directive. `go run ./internal/tools/nocomments .` enforces it for Go, and `npm run nocomments` in `web` for TypeScript, CSS and HTML.

Every pull request targets `main` and is merged with a merge commit once the `pr-targets-main`, `gitleaks`, `test`, `web` and `build` checks pass.

## License

MIT. See [LICENSE](LICENSE).
