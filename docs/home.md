# Marklint

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

Every rule page explains what the check does, lists its parameters and defaults, and supplies a configuration example checked against the public registry. Search by rule ID, behavior or parameter name.
