# Get started with Marklint

Marklint checks documentation with one version-2 policy file.
The same stock analyzers also work in Go applications.

## Install and run

```sh
go install github.com/portpowered/markdown-linter/cmd/marklint@latest
marklint --version
marklint --root . docs
```

Pin the chosen module version in production tooling.
Without configuration, Marklint uses the recommended Markdown set.
It discovers .marklint.yaml only at the lint root.
Use --config to select another policy file.
MDX and Mermaid require the installed Node parser runtime.
See the [single-file guide](single-file-policy.md) for installation and capability details.

## Select rules

```yaml
version: 2
rules:
  sentence-budget:
    check: core.limit
    options: {measure: words, max: 25}
sets:
  house:
    use: ["markdown:recommended"]
    rules: [{rule: sentence-budget, on: each sentence}]
apply:
  - use: [house]
```

The mapping key identifies a rule; check names its implementation.
Definitions activate through sets or template references.
Unknown checks and invalid options fail configuration validation.
Use [rule sets](rule-packs.md) for composition, routing, and exceptions.

## CI and results

```sh
marklint config validate --config .marklint.yaml
marklint --config .marklint.yaml --fail-on warning docs
marklint --config .marklint.yaml --json docs
```

JSON output contains one result envelope with diagnostics and completeness metadata.
Exit zero means no finding meets the selected threshold.
Policy findings return one; operational failures return two.
Default lint never modifies content.
Safe disk fixes use --fix; stdin fixes are rejected.
