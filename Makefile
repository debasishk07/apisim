# apisim Makefile
#
# Usage:
#   make build       build the apisim binary
#   make test        run the full test suite
#   make lint        run golangci-lint
#   make run         build and run against a scenario (dev convenience)
#   make ci          run lint + test (mirrors CI)

BINARY := apisim
SRC := ./cmd/apisim
PACKAGES := ./...

.PHONY: all build test lint ci run vet clean fmt

all: build

build:
	go build -o $(BINARY) $(SRC)

test:
	go test -race $(PACKAGES)

lint:
	golangci-lint run $(PACKAGES)

vet:
	go vet $(PACKAGES)

fmt:
	gofmt -l . | grep -v '^$$' || true

ci: vet lint test

run: build
	./$(BINARY) start scenario.yaml

clean:
	rm -f $(BINARY)
	go clean -testcache
