.PHONY: build test test-race test-shell coverage coverage-html install clean release

BIN := agnostic-ai
PKG := ./cmd/agnostic-ai

build:
	go build -o $(BIN) $(PKG)

test:
	go test ./...

test-race:
	go test -race ./...

test-shell:
	bashunit scripts/release_test.sh

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
