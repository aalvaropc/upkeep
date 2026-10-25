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

## Development

`make check` runs the same checks as CI: formatting, `go mod tidy`, the dependency allowlist, the no-comments rule, `go vet`, golangci-lint, the tests with the race detector, and the build.

The code has no comments. Names and tests carry the meaning; the only exception is the `//go:embed` directive. `go run ./internal/tools/nocomments .` enforces it.

Every pull request targets `main` and is merged with a merge commit once the `pr-targets-main`, `gitleaks`, `test` and `build` checks pass.

## License

MIT. See [LICENSE](LICENSE).
