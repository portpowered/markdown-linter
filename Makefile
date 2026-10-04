export GOWORK := off
GO ?= go
PYTHON ?= python
GOLANGCI_LINT ?= golangci-lint
GO_TEST_TIMEOUT ?= 120s
COVERAGE_MIN ?= 95

.DEFAULT_GOAL := verify
.PHONY: default verify build test coverage coverage-check lint fmt fmt-check vet tools-test deps deps-tidy clean

default: verify
verify: pack-check fmt-check build vet lint coverage tools-test module-check smoke docs docs-lint coverage-publish
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

.PHONY: pack-check module-check smoke
pack-check:
	$(PYTHON) scripts/packs.py
module-check:
	$(GO) mod tidy
	git diff --exit-code -- go.mod go.sum
smoke:
	$(GO) run ./cmd/marklint --version
	$(GO) run ./cmd/marklint --rules examples/rules.yaml examples/docs
	$(GO) run ./examples/custom --rules examples/custom/rules.yaml examples/docs

.PHONY: docs docs-update docs-deps
docs-deps:
	$(PYTHON) -m pip install -r docs/requirements.txt
docs-update:
	$(PYTHON) scripts/site.py --update
docs:
	$(PYTHON) scripts/site.py
	$(PYTHON) -m mkdocs build --strict --config-file mkdocs.generated.yml
	$(PYTHON) scripts/site.py --check-html

.PHONY: coverage-publish
coverage-publish: coverage docs
	$(PYTHON) scripts/quality.py coverage-publish --go "$(GO)"

.PHONY: docs-lint
docs-lint: docs
	$(GO) run ./cmd/marklint --rules docs/lint.yaml --root . --fail-on warning .site-docs README.md
