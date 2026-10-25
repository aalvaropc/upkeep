GO ?= go
BIN ?= bin/upkeep
NPM ?= npm

.PHONY: check fmt-check tidy-check deps-check nocomments vet lint test web-deps web-check web build go-build smoke dev eval

check: fmt-check tidy-check deps-check nocomments vet lint test web-check build

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

web-deps:
	cd web && $(NPM) ci --no-audit --no-fund

web-check: web-deps
	cd web && $(NPM) run check

web: web-deps
	cd web && $(NPM) run build

go-build:
	CGO_ENABLED=0 $(GO) build -trimpath -o $(BIN) ./cmd/upkeep

build: web go-build

EVAL_OUT ?= docs/evals

eval: go-build
	UPKEEP_MODE=replay ./$(BIN) eval -runs 1 -out $(EVAL_OUT)

dev:
	cd web && $(NPM) run dev

smoke: build
	./$(BIN) version
	@set -e; data="$$(mktemp -d)"; export UPKEEP_ADDR=127.0.0.1:18080 UPKEEP_DATA_DIR="$$data"; \
	./$(BIN) serve 2>"$$data/serve.log" & pid=$$!; trap 'kill -TERM $$pid 2>/dev/null || true; rm -rf "$$data"' EXIT; \
	base=http://$$UPKEEP_ADDR; \
	curl -fsS --retry 20 --retry-delay 1 --retry-all-errors $$base/healthz; echo; \
	page="$$(curl -fsS $$base/)"; echo "$$page" | grep -q 'id="root"'; \
	asset="$$(echo "$$page" | grep -o '/assets/[^"]*\.js' | head -1)"; curl -fsS -o /dev/null "$$base$$asset"; \
	timeout 3 curl -sN $$base/api/sessions/local/events > "$$data/sse.txt" || true; \
	grep -q '^event: hello' "$$data/sse.txt"; \
	kill -TERM $$pid; wait $$pid; trap 'rm -rf "$$data"' EXIT; \
	echo "smoke: health check, dashboard and its assets, event stream and clean shutdown ok"
