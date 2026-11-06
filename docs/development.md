# Development

Use Go 1.24 or newer. Run all commands from this repository:

```sh
GOWORK=off go test ./... -timeout 120s
GOWORK=off go test -race ./... -timeout 120s
GOWORK=off go vet ./...
GOWORK=off go build ./cmd/marklint
```

## Adding a check

Implement `interfaces.Analyzer` and report findings through `Pass.Report`. Register a factory that strictly validates options with `rulepack.DecodeOptions`. Use the same registration path for stock checks and customer extensions. A single-document `interfaces.Rule` can use `rulepack.AdaptRule`; diagnostics and suggested fixes remain available through the public engine interfaces.

Keep customer conventions in packs or customer checks. The stock command must not discover a particular service repository or load its files implicitly. Respect `Pass.Root` for filesystem access, and use `interfaces.CheckPathRoot` before reading targets. Customer Go code runs with the command's privileges. The API defines an extension contract for trusted code.

## Verification

Tests should cover findings, locations, malformed options, deterministic output, and meaningful behavior at the check boundary. Fixes need dry-run, conflict, and apply evidence. Cross-file checks need more than one input file. The CLI tests verify custom packs, severity, suppression, root boundaries, and configuration errors. The separate consumer project that publishes a release must fetch the exact tag without workspace replacements.

Application integration belongs in each consumer. Test fixtures may express particular customer conventions, but production packages must receive those conventions through public configuration.

## Parser lifetime

The stock CLI owns one lazy Node MDX parser worker per invocation. Each document is parsed anew; only runtime imports are reused. The worker uses newline-delimited JSON, observes the existing per-request deadline and is terminated and reaped when its owner exits or a transport fails. Mermaid retains a separate process per request because its parser has mutable global state.

Installed runtimes opt into this protocol with `marklintMdxStream: 1` in their package manifest and implement `analyze.mjs --stream-mdx`. Custom runtimes without that declaration retain the original single-request stdin protocol. Direct library calls outside a CLI invocation also retain process isolation. MDX imports, expressions and components remain syntax data and are never evaluated.

Run `node --test runtime/analyze.test.mjs` after installing the frozen runtime dependencies to verify isolated/stream parity, Unicode positions, malformed input and non-execution of MDX code.
