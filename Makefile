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
	@set -e; data="$$(mktemp -d)"; export UPKEEP_ADDR=127.0.0.1:18080 UPKEEP_DATA_DIR="$$data"; \
	./$(BIN) serve 2>"$$data/serve.log" & pid=$$!; trap 'kill -TERM $$pid 2>/dev/null || true; rm -rf "$$data"' EXIT; \
	base=http://$$UPKEEP_ADDR; \
	curl -fsS --retry 20 --retry-delay 1 --retry-all-errors $$base/healthz; echo; \
	curl -fsS $$base/ | grep -q 'id="board"'; \
	timeout 3 curl -sN $$base/api/sessions/local/events > "$$data/sse.txt" || true; \
	grep -q '^event: hello' "$$data/sse.txt"; \
	kill -TERM $$pid; wait $$pid; trap 'rm -rf "$$data"' EXIT; \
	echo "smoke: health check, board page, event stream and clean shutdown ok"
