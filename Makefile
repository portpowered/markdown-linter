GO ?= go
GO_TEST_TIMEOUT ?= 120s

.PHONY: build test fmt vet deps deps-tidy clean

default: fmt build test
build:
	$(GO) build ./...

test:
	$(GO) test ./... -timeout $(GO_TEST_TIMEOUT) -v

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

deps:
	$(GO) mod download

deps-tidy:
	$(GO) mod tidy

clean:
	$(GO) clean ./...
