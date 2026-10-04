# Use Marklint as a Go library

Use the public `pkg/rulepack` API when you want the same configuration, scopes and exceptions as the CLI. Use `pkg/engine` to parse Markdown, and `pkg/interfaces` for documents, analyzers and structured diagnostics. The module name is `github.com/portpowered/markdown-linter`; `marklint` is the command name.

```sh
go get github.com/portpowered/markdown-linter@latest
```

Pin a reviewed module version in your application's `go.mod`. The library does not run checks merely because you construct `engine.New()`; register rules or compile a pack explicitly.

## Compile and run a pack

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/portpowered/markdown-linter/pkg/engine"
    "github.com/portpowered/markdown-linter/pkg/interfaces"
    "github.com/portpowered/markdown-linter/pkg/rulepack"
)

func main() {
    ctx := context.Background()
    root := "."
    registry := rulepack.NewRegistry()
    if err := rulepack.RegisterStock(registry); err != nil {
        log.Fatal(err)
    }
    pack, err := rulepack.Load("markdown:recommended", root)
    if err != nil {
        log.Fatal(err)
    }
    program, err := registry.Compile(pack)
    if err != nil {
        log.Fatal(err)
    }
    filename := "docs/guide.md"
    if err := interfaces.CheckPathRoot(root, filename); err != nil {
        log.Fatal(err)
    }
    doc, err := engine.New().ParseFile(filename)
    if err != nil {
        log.Fatal(err)
    }
    findings, err := program.Run(ctx, root, []*interfaces.Document{doc})
    if err != nil {
        log.Fatal(err)
    }
    for _, finding := range findings {
        fmt.Printf("%s:%d [%s] %s: %s\n",
            finding.Path, finding.Line, finding.Severity,
            finding.RuleID, finding.Message)
    }
}
```

Replace the preset name with `.marklint.yaml` to load customer configuration. Supply all selected documents in one `Program.Run` call for cross-file checks such as unique IDs and link relocation. Pass a cancellable context when analysis belongs to a request or background job.

`Program.Run` returns findings separately from operational errors. A finding contains `RuleID`, `CheckID`, `Origin`, `Severity`, `Path`, `Line`, byte offsets and optional `SuggestedFixes`. Your application decides which severities block publication. A nil error does not mean there are no findings.

Filesystem checks use the explicit root. Check input paths before reading them, and keep the root consistent when loading configuration and running the program. STE100 dictionary paths resolve from this root; broken dictionaries return an error instead of passing silently.

## Add your own check

Implement `interfaces.Analyzer` with `ID()` and `Analyze(context.Context, *interfaces.Pass)`. Report findings through `Pass.Report`; report failures that prevent analysis through `Pass.ReportError`. Register a factory with `Registry.Register`, and use `rulepack.DecodeOptions` for strict customer-option validation. Your configured `check` then selects that factory through the same pack system.

The [custom command example](../examples/custom/main.go) demonstrates registration and `cli.RunWithRegistry`. No plugin discovery or runtime Go compilation is required. Customer Go code runs with the host application's privileges.

For small direct integrations, `engine.WithRules`, `engine.WithDiagnosticRules` and `engine.WithAnalyzers` register checks explicitly. Use the public pack and CLI APIs to retain severity, scopes, baselines and reasoned suppressions. Baselines are an adoption layer; the library's raw findings remain available for your own reporting.

The pack integration shown above is exercised in `pkg/rulepack/site_contract_test.go`; every generated rule configuration is compiled in that test as well. See the [package API reference](https://pkg.go.dev/github.com/portpowered/markdown-linter/pkg/rulepack) for exported contracts.
