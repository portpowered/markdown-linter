# Single-file policy

Version 2 stores rules, patterns, templates, sets, and routing in one YAML file.
The command reads no customer template, matcher, or vocabulary files.
Installed parsers and trusted analysis extensions belong to the tool installation.

## Run a policy

```sh
marklint --config .marklint.yaml docs
marklint config validate --config .marklint.yaml
marklint config explain --config .marklint.yaml --path docs/guide.md
marklint --config .marklint.yaml --stdin-filepath docs/draft.md --json -
```

Without input paths, the command scans the current directory.
It discovers configuration only at the lint root.
An explicit configuration path keeps that root unchanged.
Inputs and local link reads must stay within the root.
Directory scans include Markdown and MDX, and skip Git and dependency directories.

## Define and activate rules

```yaml
version: 2
language: en
profile: unicode-v1
rules:
  sentence-budget:
    check: core.limit
    options: {measure: words, max: 25}
  section-budget:
    extends: sentence-budget
    options: {max: 300}
  no-hype:
    check: core.match
    options: {mode: word, expect: absent, case: fold, values: [utilize, seamless]}
sets:
  house:
    use: [portos]
    rules:
      - {rule: sentence-budget, on: each sentence}
      - {rule: section-budget, on: each section}
      - {rule: no-hype, on: document}
apply:
  - use: [house]
```

Definitions run only through bindings or template references.
Inheritance replaces supplied option fields and preserves other fields.
Multiple matching apply entries contribute independently.
Cycles, unknown fields, duplicate keys, aliases, and contradictory bounds fail validation.
An unmatched input fails routing unless apply is empty.
An empty apply array requests syntax checks only.

## Embed an outline

```yaml
templates:
  guide: |-
    {{ document sections=1 max-heading-level<=2 }}
    # {{ heading words=1..10 }}
    ## Data
    {{ section paragraphs=1..2 words<=100 }}
    {{ paragraph[1].sentence[1] contains="cache" }}
sets:
  guides: {use: [house], templates: [guide]}
apply:
  - use: [house]
  - {files: [docs/**], use: [guides]}
```

This fragment replaces the example's sets and apply routing as appropriate.
Literal headings require exact visible labels, levels, and order.
Indexed targets must exist; each selectors can select nothing.
Section budgets default to direct scope; subtree includes nested prose and child sections.
Inline constraints use the same evaluators as named rules.
Patterns use literal text and nonempty named captures, with bounded matching.

The [complete example](../examples/v2/.marklint.yaml) includes table-column rules and a Mermaid flowchart requirement.
Its [sample guide](../examples/v2/docs/guide.md) is checked in CI.

## Install analysis capabilities

Ordinary Markdown checks need only the Go executable.
MDX and Mermaid checks require Node 24 and the pinned runtime dependencies.

```sh
make runtime-deps
```

Release archives include the parser runtime beside the executable.
For another installation layout, set MARKLINT_RUNTIME to its runtime directory.
MDX parsing never executes expressions, components, or imports.
Trusted component declarations expose static children; dynamic values remain unknown.
Historical whole-document checks currently declare Markdown support only.
Selecting those checks for MDX fails preflight.

The Unicode profile uses shared word, sentence, and extended grapheme boundaries.
Languages requiring dictionary segmentation need another installed profile; unavailable capabilities fail explicitly.
English readability uses a documented vowel-group syllable estimate and reports its counts and sample size.
Non-ASCII unresolved words produce unknown readability results.
Trusted builds can register measures and native score providers through the contract capability API.
Remote providers require explicit operator network permission.
Paragraph-context provider requests require a native sentence attribution adapter.

## Inspect and share results

```sh
marklint config schema
marklint config schema --schema report
marklint rules describe core.match
marklint sets describe portos
marklint config format --config .marklint.yaml
marklint config format --check --config .marklint.yaml
marklint config export --config .marklint.yaml --set house
```

Formatting writes canonical YAML to stdout and preserves comments and embedded strings.
Export includes the selected dependency closure and replaces routing with a catch-all application.
The generated [configuration schema](../schemas/config.schema.json) and [report schema](../schemas/report.schema.json) describe the installed contract.

JSON output contains one version-2 envelope, including caught configuration and execution failures.
Locations use UTF-8 byte offsets and Unicode scalar columns.
Unknown results keep complete false, even when the failure threshold permits exit zero.
Exit codes are zero for success, one for policy findings, and two for operational failures.
Disk fixes use the existing safe-edit planner and rerun lint after application.
Fixes cannot target stdin.

## Migrate previous policies

Version 1 and --rules are removed from the public command.
Move rule IDs into mapping keys and activate rules through sets and apply entries.
Use portos instead of the previous Portos set names.
Flatten every explicitly approved vocabulary form into the inline dictionary.
A migration with unavailable inline semantics needs manual review; do not drop restrictions.
