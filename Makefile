.PHONY: build test install clean release

BIN := agnostic-ai
PKG := ./cmd/agnostic-ai

build:
	go build -o $(BIN) $(PKG)

test:
	go test ./...

install:
	go install $(PKG)

clean:
	rm -f $(BIN)
	rm -rf dist/

release:
	mkdir -p dist
	GOOS=darwin  GOARCH=arm64 go build -o dist/$(BIN)-darwin-arm64  $(PKG)
	GOOS=darwin  GOARCH=amd64 go build -o dist/$(BIN)-darwin-amd64  $(PKG)
	GOOS=linux   GOARCH=arm64 go build -o dist/$(BIN)-linux-arm64   $(PKG)
	GOOS=linux   GOARCH=amd64 go build -o dist/$(BIN)-linux-amd64   $(PKG)
	GOOS=windows GOARCH=amd64 go build -o dist/$(BIN)-windows-amd64.exe $(PKG)
