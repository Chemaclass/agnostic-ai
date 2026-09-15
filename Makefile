.PHONY: build test test-race test-shell bench coverage coverage-html cover lint fmt fmt-check vet preflight tools hooks install clean release site-check site-build site-test site-serve site-clean playground-build playground-serve playground-clean

BIN := agnostic-ai
PKG := ./cmd/agnostic-ai

# Pinned to match the golangci-lint-action version in
# .github/workflows/ci.yml. Bump both together;
# tests/integration/toolchain_pins_test.go holds them level.
GOLANGCI_LINT_VERSION := v2.13.2
LEFTHOOK_VERSION := v1.10.10
ZOLA_VERSION := 0.22.0

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

# The version check runs first because a stale golangci-lint reports the
# mismatch as "export data version N is greater than maximum supported
# version M" against arbitrary untouched files, which costs a contributor
# a diff review before they reach the real cause.
lint:
	@have=$$(golangci-lint version --short 2>/dev/null); \
	want=$(GOLANGCI_LINT_VERSION); want=$${want#v}; \
	if [ -z "$$have" ]; then \
		echo "golangci-lint not found. run 'make tools'."; \
		exit 1; \
	elif [ "$$have" != "$$want" ]; then \
		echo "golangci-lint $$have installed, $$want pinned. run 'make tools'."; \
		exit 1; \
	fi
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

# Static site (docs/site/). Zola owns templates, content, feeds, and aliases.
# The sitemap is written after Zola so its lastmod values can come from Git.
SITE_DIR := docs/site
SITE_OUTPUT_DIR := _site

site-check:
	@have=$$(zola --version 2>/dev/null); \
	want="zola $(ZOLA_VERSION)"; \
	if [ "$$have" != "$$want" ]; then \
		echo "$$want required, found $${have:-nothing}."; \
		exit 1; \
	fi
	zola --root $(SITE_DIR) check --skip-external-links

site-build:
	@have=$$(zola --version 2>/dev/null); \
	want="zola $(ZOLA_VERSION)"; \
	if [ "$$have" != "$$want" ]; then \
		echo "$$want required, found $${have:-nothing}."; \
		exit 1; \
	fi
	zola --root $(SITE_DIR) build --force --minify --output-dir $(abspath $(SITE_OUTPUT_DIR))
	./scripts/build-site-sitemap.sh $(SITE_OUTPUT_DIR)/sitemap.xml

site-test:
	go test -count=1 ./tests/integration -run '^(TestTargetUpdates_|TestZolaPin_)'

site-serve:
	@have=$$(zola --version 2>/dev/null); \
	want="zola $(ZOLA_VERSION)"; \
	if [ "$$have" != "$$want" ]; then \
		echo "$$want required, found $${have:-nothing}."; \
		exit 1; \
	fi
	zola --root $(SITE_DIR) serve

site-clean:
	rm -rf _site

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
