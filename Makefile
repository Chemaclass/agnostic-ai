.PHONY: build test test-race test-shell bench coverage coverage-html cover lint fmt fmt-check vet preflight tools hooks install clean release playground-build playground-serve playground-clean

BIN := agnostic-ai
PKG := ./cmd/agnostic-ai

# Pinned to match .github/workflows/ci.yml (golangci-lint-action@v9 with version: v2.6).
GOLANGCI_LINT_VERSION := v2.6.2
LEFTHOOK_VERSION := v1.10.10

build:
	go build -trimpath -ldflags="-s -w" -o $(BIN) $(PKG)

test:
	go test ./...

test-race:
	go test -race ./...

# e2e_test.sh drives the built binary, so build first.
test-shell: build
	bashunit scripts/release_test.sh scripts/target-facts_test.sh scripts/install_test.sh scripts/e2e_test.sh

# bench runs the permanent sync-hot-path benchmark suite. It is not part
# of preflight or CI: benchmarks are for local comparison, not pass/fail.
# `-run '^$$'` skips the unit tests so only Benchmark* functions run. See
# docs/internal/benchmarks.md for how to read and extend the suite.
bench:
	go test -run '^$$' -bench . -benchmem ./...

lint:
	golangci-lint run

fmt:
	gofmt -w .

fmt-check:
	@out=$$(gofmt -s -l .); if [ -n "$$out" ]; then \
		echo "gofmt issues. run 'make fmt'."; \
		echo "$$out"; \
		exit 1; \
	fi

vet:
	go vet ./...

# preflight runs every gate the CI Lint and Test jobs run, in the same
# order. Use this before `git push` (or wire it into a pre-push hook via
# `make hooks`) so PR checks never surface a `make preflight`-fixable
# error. Mirrors .github/workflows/ci.yml.
preflight: fmt-check vet lint test
	@echo "preflight: ok"

# tools installs the developer toolchain pinned to the versions CI uses.
# Idempotent. Run once after cloning, and again whenever the pins above
# bump.
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	go install github.com/evilmartians/lefthook@$(LEFTHOOK_VERSION)

# hooks installs the lefthook git hooks defined in lefthook.yml. Run
# once after cloning. Requires `make tools` first.
hooks:
	lefthook install

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

coverage:
	go test -coverpkg=./internal/...,./cmd/... -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1

coverage-html: coverage
	go tool cover -html=coverage.out -o coverage.html
	@echo "Open coverage.html in your browser."

install:
	go install $(PKG)

clean:
	rm -f $(BIN) coverage.out coverage.html
	rm -rf dist/

release:
	mkdir -p dist
	GOOS=darwin  GOARCH=arm64 go build -o dist/$(BIN)-darwin-arm64  $(PKG)
	GOOS=darwin  GOARCH=amd64 go build -o dist/$(BIN)-darwin-amd64  $(PKG)
	GOOS=linux   GOARCH=arm64 go build -o dist/$(BIN)-linux-arm64   $(PKG)
	GOOS=linux   GOARCH=amd64 go build -o dist/$(BIN)-linux-amd64   $(PKG)
	GOOS=windows GOARCH=amd64 go build -o dist/$(BIN)-windows-amd64.exe $(PKG)

# WASM playground (docs/playground/). Bundles the WebAssembly entry
# point plus the Go-toolchain wasm_exec.js shim into the static page so
# it can be served straight from GitHub Pages.
PLAYGROUND_DIR := docs/playground

playground-build:
	GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" \
		-o $(PLAYGROUND_DIR)/agnostic-ai.wasm ./cmd/agnostic-ai-wasm
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" $(PLAYGROUND_DIR)/wasm_exec.js
	@printf "playground built. wasm size: "
	@ls -lh $(PLAYGROUND_DIR)/agnostic-ai.wasm | awk '{print $$5}'

playground-serve: playground-build
	@echo "serving $(PLAYGROUND_DIR) at http://127.0.0.1:8080"
	@cd $(PLAYGROUND_DIR) && python3 -m http.server 8080

playground-clean:
	rm -f $(PLAYGROUND_DIR)/agnostic-ai.wasm $(PLAYGROUND_DIR)/wasm_exec.js
