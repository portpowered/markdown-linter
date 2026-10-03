export GOWORK := off
GO ?= go
PYTHON ?= python
GOLANGCI_LINT ?= golangci-lint
GO_TEST_TIMEOUT ?= 120s
COVERAGE_MIN ?= 95

.DEFAULT_GOAL := verify
.PHONY: default verify build test coverage coverage-check lint fmt fmt-check vet tools-test deps deps-tidy clean

default: verify
verify: fmt-check build vet lint coverage tools-test
build:
	$(GO) build ./...
test:
	$(GO) test -race ./... -timeout $(GO_TEST_TIMEOUT)
coverage:
	$(PYTHON) scripts/quality.py coverage --go "$(GO)" --timeout $(GO_TEST_TIMEOUT) --threshold $(COVERAGE_MIN)
coverage-check:
	$(PYTHON) scripts/quality.py coverage-check --threshold $(COVERAGE_MIN)
lint:
	$(PYTHON) scripts/quality.py lint --lint "$(GOLANGCI_LINT)"
fmt:
	$(GO) fmt ./...
fmt-check:
	$(PYTHON) scripts/quality.py format
vet:
	$(GO) vet ./...
tools-test:
	$(PYTHON) scripts/quality.py tools-test
deps:
	$(GO) mod download
deps-tidy:
	$(GO) mod tidy
clean:
	$(GO) clean ./...
