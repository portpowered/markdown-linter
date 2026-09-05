# Rule packs

A pack is one YAML document with `version: 1`, a `rules` sequence, and optional `suppressions`. Unknown fields, duplicate IDs, unknown checks, invalid options, and unsupported versions fail configuration. An empty rule sequence intentionally disables checks.

Each rule has required `id` and `check`; optional `description`, `severity` (`error`, `warning`, or `info`, default `error`), `include`, `exclude`, and `options`. Include/exclude patterns are root-relative slash paths: shell-style `*`, `?`, and character classes apply within a path segment; a trailing `/**` includes a directory recursively. Other recursive glob forms are not supported. Absolute paths, backslashes, and unnormalized paths are rejected. Includes select documents before cross-file analysis; exclusions take precedence. Omitted includes select all supplied documents.

A suppression has `rule`, root-relative `path` pattern, required explanatory `reason`, and optional one-based `line`; omitted or zero line suppresses the entire matching file for that rule. Suppressions apply equally to stock and custom checks and also remove their suggested fixes.

## Stock checks

| Check | Options |
| --- | --- |
| `markdown.formatting` | None; trailing whitespace and unclosed fences |
| `markdown.heading-order` | None; skipped heading levels |
| `markdown.ordered-list` | None; explicit sequential numbering starting at 1 |
| `markdown.local-links` | `allow-directories` boolean, default false |
| `markdown.required-heading` | Required `heading` text and `level` from 1 through 6 |
| `markdown.document-identifier` | `identifiers` sequence described below |
| `markdown.document-structure` | `types` sequence described below |
| `markdown.doc-id-unique` | None; duplicate frontmatter `doc-id` values, with conservative typed-ID fixes |
| `markdown.link-relocation` | Optional `moves` mapping from old paths to new paths |

Identifier entries accept `name`, required `field`, `paths` prefixes, `required`, `pattern` (Go regular expression), `format-label`, `unique`, and `match-filename`. Structure entries accept `paths` prefixes, `required-fields`, `required-headings` (level 2), `any-headings` (at least one), and `alternative-message`. The first matching structure entry applies. No service-specific types or identifier patterns are installed automatically.

Move paths are interpreted relative to the working directory; use the same path spelling as the input paths. Relocation indexes supplied Markdown documents and discovers non-Markdown assets beneath the document root. It does not move files; it proposes link edits in the supplied documents.

## Executable extensions

YAML composes installed checks and does not contain executable code. To add a new check, build a Go command that calls `Registry.Register` and `cli.RunWithRegistry`. The [example](../examples/custom/main.go) registers a customer check beside the stock checks. Its factory receives the same options node, and its analyzer receives the same pass and diagnostic capabilities. There is no runtime plugin loading in version 1.
