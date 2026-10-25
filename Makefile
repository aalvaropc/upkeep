GO ?= go
BIN ?= bin/upkeep

.PHONY: check fmt-check tidy-check deps-check nocomments vet lint test build smoke

check: fmt-check tidy-check deps-check nocomments vet lint test build

fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "$$out"; exit 1; fi

tidy-check:
	$(GO) mod tidy -diff

deps-check:
	@extra="$$($(GO) mod edit -json | jq -r '(.Require // [])[] | select(.Indirect | not) | .Path' | grep -vxF modernc.org/sqlite || true)"; \
	if [ -n "$$extra" ]; then echo "unexpected direct dependency: $$extra"; exit 1; fi

nocomments:
	$(GO) run ./internal/tools/nocomments .

vet:
	$(GO) vet ./...

lint:
	golangci-lint run

test:
	$(GO) test -race -count=1 -shuffle=on ./...

build:
	CGO_ENABLED=0 $(GO) build -trimpath -o $(BIN) ./cmd/upkeep

smoke: build
	./$(BIN) version
