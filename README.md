# Upkeep

Upkeep keeps GitHub pull requests mergeable. It finds what blocks each open pull request, fixes what a bounded policy allows, proves every fix with the compiler and the tests, and hands the rest back with a diagnosis and a receipt.

It runs on NVIDIA Nemotron models served by Nebius Token Factory.

Work in progress.

## Build and run

Upkeep is a single Go binary. You need Go 1.26 or newer.

```sh
go build -o bin/upkeep ./cmd/upkeep
./bin/upkeep version
./bin/upkeep help
```

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

## Development

`make check` runs the same checks as CI: formatting, `go mod tidy`, the dependency allowlist, the no-comments rule, `go vet`, golangci-lint, the tests with the race detector, and the build.

The code has no comments. Names and tests carry the meaning; the only exception is the `//go:embed` directive. `go run ./internal/tools/nocomments .` enforces it.

Every pull request targets `main` and is merged with a merge commit once the `pr-targets-main`, `gitleaks`, `test` and `build` checks pass.

## License

MIT. See [LICENSE](LICENSE).
