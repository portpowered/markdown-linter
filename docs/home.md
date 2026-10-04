# Marklint

[![Go version](https://img.shields.io/github/go-mod/go-version/portpowered/markdown-linter)](https://github.com/portpowered/markdown-linter/blob/main/go.mod)
[![CI](https://github.com/portpowered/markdown-linter/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/portpowered/markdown-linter/actions/workflows/ci.yml)
[![Coverage](https://portpowered.github.io/markdown-linter/coverage.svg)](https://portpowered.github.io/markdown-linter/coverage.html)
[![Release](https://img.shields.io/github/v/release/portpowered/markdown-linter?display_name=tag)](https://github.com/portpowered/markdown-linter/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/portpowered/markdown-linter.svg)](https://pkg.go.dev/github.com/portpowered/markdown-linter)
[![License](https://img.shields.io/github/license/portpowered/markdown-linter)](https://github.com/portpowered/markdown-linter/blob/main/LICENSE)
[![Documentation](https://img.shields.io/badge/docs-GitHub%20Pages-blue)](https://portpowered.github.io/markdown-linter/)

Check Markdown structure, links and prose with rules that match your documentation policy.

Start with `markdown:recommended`, compose optional editorial or company packs, and configure individual checks without changing the linter. The same registry and rules power both the CLI and the Go library.

## Choose a starting point

| Task | Guide |
| --- | --- |
| Install the CLI and lint documentation | [Get started](getting-started.md) |
| Embed checks in a Go application | [Go library](library.md) |
| Find a rule and its parameters | [Rule reference](rules/index.md) |
| Compose packs, scopes and exceptions | [Rule packs](rule-packs.md) |
| Enforce approved words and explicit forms | [STE100](ste100.md) |
| Review style with finite editorial signals | [Strunk and White](strunk-white.md) |

## A configuration you control

```yaml
version: 1
extends: [markdown:recommended, text:strunk-white]
overrides:
  - id: markdown.line-length
    options: {max: 100}
```

Each rule page explains the check and its parameters. It supplies a configuration example checked against the public registry. Search by rule ID, behavior or parameter name.
