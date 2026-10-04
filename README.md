# Markdown linter

[![Go version](https://img.shields.io/github/go-mod/go-version/portpowered/markdown-linter)](https://github.com/portpowered/markdown-linter/blob/main/go.mod)
[![CI](https://github.com/portpowered/markdown-linter/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/portpowered/markdown-linter/actions/workflows/ci.yml)
[![Coverage](https://portpowered.github.io/markdown-linter/coverage.svg)](https://portpowered.github.io/markdown-linter/coverage.html)
[![Release](https://img.shields.io/github/v/release/portpowered/markdown-linter?display_name=tag)](https://github.com/portpowered/markdown-linter/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/portpowered/markdown-linter.svg)](https://pkg.go.dev/github.com/portpowered/markdown-linter)
[![License](https://img.shields.io/github/license/portpowered/markdown-linter)](https://github.com/portpowered/markdown-linter/blob/main/LICENSE)
[![Documentation](https://img.shields.io/badge/docs-GitHub%20Pages-blue)](https://portpowered.github.io/markdown-linter/)

A Go library and `marklint` command for configurable Markdown checks, cross-file analysis, and explicit safe fixes. It runs on a standalone documentation tree and has no service dependency.

## Run

From a source checkout with Go 1.24 or newer:

```sh
go build -o marklint ./cmd/marklint
./marklint --config ./examples/rules.yaml ./examples/docs
./marklint --version
```

With no paths, the version-2 command scans the current directory. `--root` bounds inputs and local links, including symlink targets. External URLs are ignored by local-link checks.

Without configuration, the default is `markdown:recommended`. At the selected root, `.marklint.yaml` is discovered automatically. Use `--config` to select an explicit version-2 policy. Version-1 configuration is rejected.

## Single-file policy

Version 2 embeds rules, matcher patterns, templates, sets, and path routing in one YAML file.
Shared evaluators check word budgets, sentence patterns, table columns, block order, and Mermaid flowcharts.
See the [single-file guide](docs/single-file-policy.md) and [complete configuration](examples/v2/.marklint.yaml).

```sh
make runtime-deps
./marklint --config examples/v2/.marklint.yaml --root examples/v2 examples/v2/docs
./marklint --config .marklint.yaml --json .
```

MDX and Mermaid use pinned static parsers that require Node 24.
The repository policy checks README and Markdown sources as part of CI.
The documentation policy also checks the generated site.

## Install a release

Build this checkout with `go build ./cmd/marklint` for the version 2 contract. Release archives include the parser runtime and schemas. Verify an archive against its SHA-256 checksum, unpack it, and put `marklint` (`marklint.exe` on Windows) on your PATH. Archives cover Linux, macOS, and Windows on amd64 and arm64. CI executes tests on hosted Linux, macOS, and Windows; cross-built architectures receive build verification.

Go-installed commands report `dev` unless built with version linker flags; release archives embed their tag. Remove the installed executable to uninstall. No service or background process is installed.

## Customer rules

```yaml
version: 2
rules:
  handbook.purpose:
    check: markdown.required-heading
    options: {heading: Purpose, level: 2}
sets:
  handbook:
    rules: [{rule: handbook.purpose, on: document}]
apply:
  - use: ["markdown:recommended"]
  - {files: [handbook/**], use: [handbook]}
```

YAML assigns rule IDs, selects registered checks, supplies options, and controls scope and severity. New logic uses the public Go analyzer and factory API. Stock checks use the same registry without privileged engine access. See [rule packs](docs/rule-packs.md) and the [custom command example](examples/custom/main.go).

## Diagnostics and fixes

Text output includes the path, line, column, severity, rule, and check. `--json` and `--format json` emit a version-2 result envelope. Exit codes are 0 for success, 1 for policy findings, and 2 for operational failure.

Linting is read-only. `--fix-check` previews rule-provided fixes; `--fix` applies accepted safe edits. The planner rejects invalid ranges and overlapping edits. Fix modes use the same selected pack as linting. Select `markdown:maintenance` explicitly for relocation and duplicate `doc-id` repair. After applying edits, the command reruns lint and returns the remaining finding status. File changes between analysis and application are outside the command's concurrency contract; do not edit inputs concurrently.

Relocation uses the moves option on a configured `markdown.link-relocation` rule. Ambiguous destinations and missing anchors remain manual-review findings.

## Go packages

| Package | Responsibility |
| --- | --- |
| `pkg/interfaces` | Parsed documents, analyzers, passes, diagnostics, and edit contracts |
| `pkg/engine` | Parsing, visitor helpers, execution, and fix planning |
| `pkg/rules` | Public implementations and cross-file indexes |
| `pkg/rulepack` | Strict YAML schema, factory registry, scope, and suppression |
| `pkg/cli` | Stock command and customer-registry command entrypoints |

Build a pack with `rulepack.Decode`, register checks with `RegisterStock` and `Registry.Register`, compile it, and call `Program.Run` on documents parsed through `engine.New().ParseFile`. `Pass.Documents` contains the rule's selected documents; `Pass.Root` specifies its filesystem boundary. Composed analyzers can share indexes through `Pass.SetData` and `Pass.Data` using `rulepack.AnalysisGroup`. Direct engine callers own their filesystem policy; use the public root options when reading other files.

## Development

See [development](docs/development.md). Run `GOWORK=off go test ./...` and `GOWORK=off go vet ./...`. The fixtures and examples belong to this repository; no parent checkout or credentials are required.

## Rule and usability roadmap

See the [linter plan](docs/linter-roadmap.md) for system comparisons, new rules, customer activation, composition, tests and rollout. It records implemented rules and UX, Portos requirements with rule IDs, and the remaining customer pilot work.

The opt-in [Strunk and White ruleset](docs/strunk-white.md) adds ten editorial checks, including two paired-construction checks. Compose `text:strunk-white` with `markdown:recommended`; the guide documents the source analysis, finite patterns, legitimate exceptions, and tests.

## Compose Portos policy

Use `portos` in a set to activate general recommendations and internal Portos policy. `rules list`, `sets list`, and `config explain` show capabilities and effective policy. See the rule-pack reference for routing, SARIF, failure thresholds, and configuration changes.

Pin a release containing the version 2 contract when deploying to CI.

## Development checks

Run `make verify` for formatting, build, vet, standard Go linters, race tests and the 95% coverage gate. It also checks quality tools. Install Python 3, Go and golangci-lint v2.14.0 first. Windows users can set `PYTHON=python`; Unix installations may prefer `PYTHON=python3`. `GOLANGCI_LINT` can point to the pinned executable.

Individual targets are `make test`, `make lint`, `make coverage`, `make coverage-check`, `make fmt-check`, and `make vet`. `make fmt` applies formatting. Coverage is statement-weighted across library, command and example packages. `-coverpkg=./...` counts library execution from integration tests. The report includes every file and package. The gate compares the unrounded value to 95%. `coverage.out` is local output and CI uploads each platform/toolchain profile.

CI runs the same coverage/lint policy on Linux, macOS and Windows with the current stable Go release. The explicit `linters.default: standard` configuration enables errcheck, govet, ineffassign, staticcheck and unused, with no preset issue exclusions. See the [official standard linter list](https://golangci-lint.run/docs/welcome/quick-start/) and [pinned release](https://github.com/golangci/golangci-lint/releases/tag/v2.14.0). Lint failures are fixed instead of baselined.

The opt-in [STE100 ruleset](docs/ste100.md) checks prose against a supplied approved-word dictionary and configurable grammar patterns. Dictionary entries group explicit inflections and provide alternatives as editorial suggestions.

Browse the [documentation website](https://portpowered.github.io/markdown-linter/) for rule behavior, parameters, configuration examples and Go library usage. See [website maintenance](docs/website.md) for local builds and CI.
