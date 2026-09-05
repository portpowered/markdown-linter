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

Keep customer conventions in packs or customer checks. The stock command must not discover a particular service repository or load its files implicitly. Respect `Pass.Root` for filesystem access, and use `interfaces.CheckPathRoot` before reading targets. Customer Go code runs with the command's privileges; the API is an extension contract, not a sandbox for untrusted code.

## Verification

Tests should cover findings, locations, malformed options, deterministic output, and meaningful behavior at the check boundary. Fixes need dry-run, conflict, and apply evidence. Cross-file checks need more than one input file. The CLI tests verify custom packs, severity, suppression, root boundaries, and configuration errors. The separate consumer project that publishes a release must fetch the exact tag without workspace replacements.

Application integration belongs in each consumer. Test fixtures may express particular customer conventions, but production packages must receive those conventions through public configuration.
