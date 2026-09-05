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

Without `--rules`, the default pack checks formatting, heading order, and local links. An explicit pack runs only its configured rules. `--only id,other-id` filters that pack after the whole configuration has been validated.

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

Linting is read-only. `--fix-check` previews rule-provided fixes; `--fix` applies accepted safe edits. The planner rejects invalid ranges and overlapping edits. With no explicit pack, fix modes use relocation and duplicate `doc-id` checks. With an explicit pack, they use that pack's checks. After applying edits, rerun ordinary lint: successful application reports exit 0 even when other findings require manual review. File changes between analysis and application are outside the command's concurrency contract; do not edit inputs concurrently.

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
