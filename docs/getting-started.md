# Get started with Marklint

Marklint checks Markdown files using a configuration you control. It can run as a CLI in CI or as a Go library inside your application.

## Install and run

```sh
go install github.com/portpowered/markdown-linter/cmd/marklint@latest
marklint --version
marklint init --preset markdown:recommended --output .marklint.yaml
marklint --root . docs
```

Pin the chosen module version in production tooling. The `latest` command is a convenient starting point. `--root` defines the filesystem boundary for inputs, local configuration and referenced files.

The CLI discovers `.marklint.yaml` from the document root. Use `--rules path/to/config.yaml` to select a different configuration. Input paths can be files or directories. Start with the recommended pack, then add project policy deliberately.

## Select rules

```yaml
version: 1
extends: [markdown:recommended, text:strunk-white]
overrides:
  - id: markdown.line-length
    severity: warning
    options: {max: 100}
rules:
  - id: customer.preferred-words
    check: text.terminology
    severity: error
    options:
      terms: {utilize: use}
      scope: prose
```

`id` identifies your configured rule instance; `check` names the registered implementation. An override targets an existing rule ID. Options mappings are replaced as a whole. Unknown checks or options fail configuration validation. Use [rule packs](rule-packs.md) for full composition and scope semantics, and the [rule reference](rules/index.md) for every check's parameters.

```sh
marklint rules list
marklint rules describe text.ste100.dictionary
marklint presets list
marklint config explain --root . --path docs/guide.md
marklint --root . --format json docs
marklint --root . --format sarif docs
```

Use `text:ste100` with a supplied approved dictionary for controlled vocabulary; see [STE100](ste100.md). `portos-defaults` adds internal Portos prose conventions. Strunk and White findings invite editorial review rather than automatic rewriting.

## CI and results

Run `marklint --root . --fail-on warning docs` to fail on warnings as well as errors. Exit 0 means the selected failure threshold passed, exit 1 means findings reached the threshold, and exit 2 means an operational or configuration failure. Keep dictionary-loading errors visible; treating them as prose exceptions would defeat vocabulary checking.

For gradual adoption, the [rule-pack guide](rule-packs.md) covers baselines, reasoned suppressions and selecting a subset with `--only`. Review suggested fixes with the CLI's documented fix workflow before applying edits. The [Go library guide](library.md) shows how to receive the same structured findings without invoking a subprocess.
