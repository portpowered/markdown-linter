# Marklint

Check Markdown structure, links, and prose with one policy file.
Shared evaluators support outlines, tables, patterns, and code requirements.

## Choose a starting point

| Task | Guide |
| --- | --- |
| Install the CLI | [Get started](getting-started.md) |
| Define a complete policy | [Single-file guide](single-file-policy.md) |
| Embed checks in Go | [Go library](library.md) |
| Compose sets and routing | [Rule sets](rule-packs.md) |
| Enforce approved vocabulary | [STE100](ste100.md) |
| Review finite editorial signals | [Strunk and White](strunk-white.md) |

## Start with recommended checks

```yaml
version: 2
apply:
  - use: ["markdown:recommended"]
```

Add rules and embedded templates when project policy needs them.
Select portos explicitly for internal prose conventions.
Generated reference pages document each stock analyzer.
