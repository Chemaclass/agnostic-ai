.PHONY: build test test-race test-shell coverage coverage-html cover lint fmt vet install clean release playground-build playground-serve playground-clean

BIN := agnostic-ai
PKG := ./cmd/agnostic-ai

build:
	go build -trimpath -ldflags="-s -w" -o $(BIN) $(PKG)

test:
	go test ./...

test-race:
	go test -race ./...

test-shell:
	bashunit scripts/release_test.sh

lint:
	golangci-lint run

fmt:
	gofmt -w .

vet:
	go vet ./...

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
