# Markdown linter

A Go library and `marklint` command for configurable Markdown checks, cross-file analysis, and explicit safe fixes. It runs on a standalone documentation tree and has no service dependency.

## Run

From a source checkout with Go 1.24 or newer:

```sh
go build -o marklint ./cmd/marklint
./marklint --rules ./examples/rules.yaml ./examples/docs
./marklint --version
```

The command requires files or directories. Directories are searched recursively for Markdown. `--root` defaults to the working directory and bounds inputs, local links, and relocation targets, including symlink targets. External URLs are ignored; no network checks or executable downloads occur when loading a pack.

Without `--rules`, the default is `markdown:recommended`. At the selected root, `.marklint.yaml` is discovered automatically. An explicit pack runs its resolved configured rules, including named/local sets from `extends`. `--only id,other-id` filters that pack after the whole configuration has been validated.

## Install a release

Install the versioned Go command with `go install github.com/portpowered/markdown-linter/cmd/marklint@v0.1.0`, or download your platform archive and `checksums.txt` from the repository releases. Verify the archive against its SHA-256 checksum, unpack it, and put `marklint` (`marklint.exe` on Windows) on your PATH. Archives cover Linux, macOS, and Windows on amd64 and arm64. CI executes tests on hosted Linux, macOS, and Windows; cross-built architectures receive build verification.

Go-installed commands report `dev` unless built with version linker flags; release archives embed their tag. Remove the installed executable to uninstall. No service or background process is installed.

## Customer rules

```yaml
version: 1
rules:
  - id: handbook.purpose
    check: markdown.required-heading
    severity: error
    include: ["handbook/**"]
    options:
      heading: Purpose
      level: 2
```

YAML assigns rule IDs, selects registered checks, supplies options, and controls scope and severity. New executable logic uses the public Go analyzer and factory API: stock checks use this same registry and receive no privileged engine access. See [rule packs](docs/rule-packs.md) and the [custom command example](examples/custom/main.go).

## Diagnostics and fixes

Text output uses `file:line: rule-id: message`. `--format json` emits an array of diagnostics with source locations, configured identity/severity, and optional suggested fixes. Exit codes are 0 for success (including warnings and information), 1 for error findings, and 2 for configuration or execution failure.

Linting is read-only. `--fix-check` previews rule-provided fixes; `--fix` applies accepted safe edits. The planner rejects invalid ranges and overlapping edits. Fix modes use the same selected pack as linting. Activate `markdown:maintenance` explicitly for relocation and duplicate `doc-id` repair. After applying edits, rerun ordinary lint: successful application reports exit 0 even when other findings require manual review. File changes between analysis and application are outside the command's concurrency contract; do not edit inputs concurrently.

Relocation accepts repeated `--move old=new` arguments or a `--move-map` file containing a JSON mapping or `old=new` lines. A custom pack must include `markdown.link-relocation` to accept these arguments. Ambiguous destinations and missing anchors remain manual-review findings.

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

See the [OpenAPI and Markdown linter plan](docs/linter-roadmap.md) for the existing-system comparison, proposed rule additions, customer activation UX, rule-set composition, testing, and rollout. It records implemented rules and UX, Portos requirements with rule IDs, and the remaining customer pilot work.

The opt-in [Strunk and White ruleset](docs/strunk-white.md) adds ten editorial checks, including two paired-construction checks. Compose `text:strunk-white` with `markdown:recommended`; the guide documents the source analysis, finite patterns, legitimate exceptions, and tests.

## Compose Portos policy

Use `init --preset portos-defaults` to activate general recommendations plus internal Portos policy in one version 1 configuration. `rules list`, `presets list`, and `config explain` make rule activation and overrides inspectable. See the rule-pack reference for baselines, SARIF, the warning failure threshold, and breaking default/configuration changes.

The expanded rules and composition UX are available in this source checkout; pin the next release containing these changes when deploying them to CI. Existing v0.1.0 installation examples refer to the earlier released baseline.

## Development checks

Run `make` (or `make verify`) for formatting validation, build, vet, all standard Go linters, race-enabled tests and a 95% statement coverage gate, plus quality-tool tests. Install Python 3, Go and golangci-lint v2.14.0 first. Windows users can set `PYTHON=python`; Unix installations may prefer `PYTHON=python3`. `GOLANGCI_LINT` can point to the pinned executable.

Individual targets are `make test`, `make lint`, `make coverage`, `make coverage-check`, `make fmt-check`, and `make vet`. `make fmt` applies formatting. Coverage is statement-weighted across both library and command/example packages, using `-coverpkg=./...` so integration tests count library execution; no files or packages are removed from the report. The gate compares the unrounded value to 95%. `coverage.out` is local output and CI uploads each platform/toolchain profile.

CI runs the same coverage/lint policy on Linux, macOS and Windows for each supported Go version. The explicit `linters.default: standard` configuration enables errcheck, govet, ineffassign, staticcheck and unused, with no preset issue exclusions. See the [official standard linter list](https://golangci-lint.run/docs/welcome/quick-start/) and [pinned release](https://github.com/golangci/golangci-lint/releases/tag/v2.14.0). Lint failures are fixed rather than baselined.

The opt-in [STE100 ruleset](docs/ste100.md) checks prose against a supplied approved-word dictionary and configurable grammar patterns. Dictionary entries group explicit inflections and provide alternatives as editorial suggestions.

Browse the [documentation website](https://portpowered.github.io/markdown-linter/) for rule behavior, parameters, configuration examples and Go library usage. See [website maintenance](docs/website.md) for local builds and CI.
