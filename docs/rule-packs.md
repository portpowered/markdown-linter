# Rule packs

Use `version: 1` for every pack. This is the only supported configuration schema. Named sets have no version suffix; pin the executable release in CI and review `packs/manifest.json` hashes when upgrading. There is no compatibility branch for previous default behavior.

## Discover and activate

Commands below use COMMAND as the executable name and CONFIG as its conventional root configuration. Replace these placeholders as shown in the tool section below.

```sh
COMMAND rules list --format json
COMMAND rules describe CHECK_ID
COMMAND presets list
COMMAND presets describe portos-defaults
COMMAND presets export portos-defaults
COMMAND init --preset portos-defaults --output CONFIG
COMMAND config schema
COMMAND config validate --rules CONFIG --root .
COMMAND config explain --rules CONFIG --root . --path INPUT_PATH
COMMAND --rules CONFIG --root . INPUT_PATH
```

A check is executable logic; a rule is one configured instance with a customer-visible ID. A rule set is a reusable pack. `rules list` lists available checks; `config explain` lists configured rules and whether they apply to a supplied path. `describe` includes options, defaults, kind, preset membership, severity guidance, and fix support. `docs/rule-catalog.json` is a readable catalog snapshot. `config schema` exports an editor JSON Schema generated from registry options; `docs/rule-pack.schema.json` is the stock snapshot. Runtime validation additionally enforces root boundaries, IDs, regex/options semantics, and kind applicability. Customer commands use the same public registry and can add descriptors with `Registry.SetDescriptor`.

`init` refuses to overwrite a file unless `--overwrite` is supplied. An explicit `--rules` wins. Otherwise the command looks only for CONFIG at `--root`; it does not read home/ancestor configs. Invalid discovered config fails instead of falling back. An empty explicit rule list with no extends intentionally disables policy checks, while parse/execution failures still fail.

## Compose and override

This is supported single-version configuration:

```yaml
version: 1
extends:
  - portos-defaults
  - ./.lint/company.yaml
overrides:
  - id: RULE_ID_FROM_PRESET
    severity: warning
  - id: ANOTHER_RULE_ID_FROM_PRESET
    enabled: false
rules: []
suppressions: []
```

Replace the two example IDs with actual configured IDs from `config explain`; unknown IDs are errors. A local company pack uses the same schema and can extend sets, add its own instances, and override inherited instances. Imports are relative to the containing pack, bounded by the selected root including symlinks. All scope paths remain relative to the lint root. Explicit root configuration must also be within that root.

Dependencies are resolved in declared order, with repeated identical dependencies loaded once. Cycles and duplicate configured IDs are errors; use `overrides` rather than defining the same ID in two packs. Each pack's overrides apply after its dependencies and its added rules. Root overrides run last. Independent instances of the same check use distinct IDs; choose disjoint scopes if overlapping findings would be redundant.

Overrides can change enabled state, severity, include, exclude, and options, but not check identity. Supplied options maps and scope lists replace whole fields, while omitted fields inherit. `options: {}` resets to check defaults. The complete resolved configuration is validated, including disabled rules and rules omitted by `--only`. Each check rejects unsupported option keys. Unknown fields/checks, bad severities/options, duplicate IDs, and unsupported configuration versions fail before linting.

Severity is error, warning, or info, default error. A rule requires id and check; optional fields are description, enabled, severity, include, exclude, and options. Include/exclude patterns are root-relative slash paths: `*`, `?`, and character classes match within a segment, and a trailing `/**` selects a subtree. Exclusions win. No absolute paths/backslashes or other recursive glob forms are supported.

`--only id,other-id` selects configured enabled instance IDs after validation. It does not activate an arbitrary installed check. An unknown/disabled selection fails. Kind mismatches fail instead of silently producing no findings; custom checks without kind metadata remain allowed.

## Exceptions and CI adoption

```yaml
suppressions:
  - rule: RULE_ID
    path: apis/legacy.yaml
    line: 42
    reason: Temporary migration exception tracked in API-123
```

Suppressions require an existing rule ID, root-relative path pattern, and nonblank reason. Omitted/zero line covers the file. Suppressed fixes are removed as well as findings. Source parse, reference boundary, and execution failures cannot be suppressed. Markdown additionally supports `<!-- marklint-disable-next-line RULE_ID reason: explanation -->` immediately before a finding. The ID must match the configured rule instance, the reason must be nonblank, and directives inside code do not suppress findings. OpenAPI pack suppressions can add `pointer: "#/paths/~1widgets/get"` to match exactly that diagnostic pointer; omitted pointers cover all locations selected by the path and line.

```sh
COMMAND baseline create --output baseline.json --rules CONFIG --root . INPUT_PATH
COMMAND --baseline baseline.json --rules CONFIG --root . INPUT_PATH
COMMAND --fail-on warning --rules CONFIG --root . INPUT_PATH
COMMAND --format sarif --rules CONFIG --root . INPUT_PATH
```

Baseline creation is explicit and read-only with respect to inputs. Replacement requires `--overwrite` on baseline create (or `--baseline-overwrite` with `--baseline-write`). Ordinary lint never updates the baseline. Fingerprints contain root-relative file identity, instance/check identity, message, and source evidence; OpenAPI also includes the pointer. Counts matter: a newly introduced duplicate is new debt. Moving unchanged lines remains recognized when message/pointer identity is unchanged; changed evidence produces a new finding. Known counts go to stderr and new findings remain in the selected output format. This is conservative debt matching, not an AST-aware API comparison.

Exit codes: 0 for success, 1 for findings at the selected failure threshold, 2 for configuration/execution failure. Default threshold is error; `--fail-on warning` includes warnings without changing their labels. JSON is an ordered diagnostics array including configured RuleID, CheckID, Origin, Severity, file/location, and message. SARIF 2.1 output uses those same findings. For editors, use the machine-readable catalog and diagnostics rather than a separate policy engine.

## Distribution and extensions

First-party packs are readable YAML embedded in the binary; SHA-256 manifests and YAML sources are included in release archives. `presets describe` includes the source content hash and resolved membership. `presets export` prints the original YAML, including composition references. Customer packs are checked-in local YAML. No config downloads code, packs, or external URLs during a lint run.

New executable checks require a customer-built Go command using `Registry.Register` and `RunWithRegistry`. Stock and customer checks have the same options/pass/diagnostic capabilities. Use `rulepack.Load(filename, root)` before `Compile` for composition; `Decode` alone parses raw YAML. Root policy is enforced by the stock loaders; custom Go code is trusted and must follow its intended filesystem/network policy itself.

## Markdown command and sets

COMMAND is `marklint`, CONFIG is `.marklint.yaml`, and INPUT_PATH is a Markdown file or documentation directory. The default is `markdown:recommended`. Other sets: markdown:core, markdown:documentation, markdown:style, markdown:maintenance, text:prose, portos:internal, and portos-defaults. A prose terminology/spelling instance requires customer options and is not enabled just by selecting text:prose.

```sh
marklint --rules examples/portos/rules.yaml examples/portos/docs
```

Rules in markdown:core are errors. Recommended adds whitespace, alt text, link text, and fence-language warnings. Portos adds text.no-dashes and text.no-load-bearing as errors. Markdown bullet/fence markers and link destinations are not prose; code spans/blocks and URL text are excluded from the two Portos checks. Neither rule automatically rewrites prose.

## Markdown options and boundaries

| Check | Options and behavior |
| --- | --- |
| markdown.formatting | Combined trailing whitespace and fence closure; allow-hard-breaks defaults true |
| markdown.trailing-whitespace | allow-hard-breaks defaults true; preserve code; safe range edits |
| markdown.fence-closed | None; fence closure separately configurable from whitespace |
| markdown.heading-order | None; skipped heading levels |
| markdown.local-links | allow-directories boolean, default false; includes image targets and heading anchors; external URLs ignored |
| markdown.ordered-list | style: one-or-ordered (default), one, or ordered |
| markdown.image-alt | allow: list of explicitly decorative image destinations |
| markdown.link-text | None; empty labels and click here are discouraged |
| markdown.reference-definitions | None; duplicate normalized labels and undefined full/collapsed references; bracketed prose is not presumed a shortcut link |
| markdown.fence-language | None; any nonblank language info string, including text, is accepted |
| markdown.single-title | frontmatter-title boolean permits a frontmatter title when H1 policy is not satisfied; scope fragments out |
| markdown.heading-duplicates | None; repeated headings under the same parent section |
| markdown.blank-lines | None; top-level block separation |
| markdown.final-newline | None; one newline, safe fix preserves LF/CRLF |
| markdown.list-style | allow: first marker string selects marker (default dash); indent: nesting spaces, default 2, range 1 to 8 |
| markdown.frontmatter-valid | None; invalid/duplicate YAML and missing terminator |
| markdown.line-length | max: positive character count, default 120; skip code, tables, URL lines |
| markdown.html-policy | allow: HTML tag names; a style policy, not a sanitizer |
| text.terminology | required terms: preferred-term mapping; optional allow and scope (prose or heading) |
| text.repeated-word | allow: legitimate repetitions; scope: prose or heading; adjacent repeated words |
| text.spelling | required language and dictionary word list; optional allow/scope; offline vocabulary only |
| text.no-dashes and text.no-load-bearing | scope: prose (default) or heading |
| markdown.required-heading | required heading and level 1 through 6 |
| markdown.document-identifier | identifiers sequence; see the entry contract below |
| markdown.document-structure | types sequence; see the entry contract below |
| markdown.doc-id-unique | None; duplicate doc-id values and conservative fixes |
| markdown.link-relocation | Optional moves mapping; no actual file moves |

Identifier entries accept name, required field, paths prefixes, required, pattern (Go regexp), format-label, unique, and match-filename. Structure entries accept paths prefixes, required-fields, required-headings (level 2), any-headings, and alternative-message; first match applies. No backend-specific templates are installed automatically.

```yaml
version: 1
extends: [markdown:recommended]
rules:
  - id: handbook.purpose
    check: markdown.required-heading
    include: [handbook/**]
    options: {heading: Purpose, level: 2}
  - id: handbook.terms
    check: text.terminology
    severity: warning
    options:
      terms: {utilize: use}
```

Fixes use the same selected pack as linting. `--fix-check` previews; `--fix` applies accepted safe nonoverlapping edits. Baselines and SARIF cannot be combined with fix modes. Applying edits can return success while manual findings remain; rerun ordinary lint afterward. File changes during application remain outside the concurrency contract.

For relocation, select markdown:maintenance in a pack and pass repeated `--move old=new` or `--move-map FILE`. Targets use the working-directory spelling and remain root bounded. Ambiguous targets/anchors need manual review. The parser supports CommonMark plus tables, strikethrough, and task lists; scope MDX/template files out unless their syntax is supported by your customer analyzer.
