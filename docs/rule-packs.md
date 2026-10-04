# Rule sets and configuration

Use version 2 for every customer policy.
Rules configure checks, templates embed outlines, sets group applications, and apply entries select files.
The [single-file guide](single-file-policy.md) explains the complete contract and examples.

## Inspect the installed tool

```sh
marklint rules list
marklint rules describe core.match
marklint sets list
marklint sets describe portos
marklint config schema
marklint config validate --config .marklint.yaml
marklint config explain --config .marklint.yaml --path docs/guide.md
```

Schemas come from the installed registry.
Validation also checks inheritance, targets, references, and capabilities.
No configuration downloads code or imports another customer policy file.

## Compose rules and sets

```yaml
version: 2
rules:
  concise:
    check: core.limit
    options: {measure: words, max: 25}
  section-budget:
    extends: concise
    options: {max: 300}
sets:
  house:
    use: [portos]
    rules:
      - {rule: concise, on: each sentence}
      - {rule: section-budget, on: each section}
apply:
  - use: [house]
```

A derived rule has one parent and cannot specify another check.
Supplied option fields replace inherited fields; nested objects replace whole values.
Set inclusion forms a graph; cycles and unknown references are errors.
Multiple active limits apply independently.
Definitions stay inactive until a binding or template names them.

## Route policy by path

Patterns use slash paths relative to the lint root.
Wildcards match inside segments; a trailing recursive wildcard selects a subtree.
Apply entries contribute together and never override earlier entries.
Root exclusions remove inputs; entry exclusions affect only that entry.
A nonempty apply array must match every selected input.
Conflicting languages, profiles, or active outlines fail routing.

## Handle exceptions and findings

A suppression names a configured rule, a path, and a nonblank reason.
Its optional line limits the exception to one source line.
Suppression cannot hide syntax, configuration, or execution errors.
Error, warning, and info severities share one failure threshold.
Unknown findings keep complete false.

```yaml
suppressions:
  - rule: concise
    path: docs/old-guide.md
    line: 12
    reason: Rewrite tracked in DOC-42.
```

## Keep historical checks

Version 2 can bind the existing Markdown and prose analyzers to document targets.
Corpus checks receive their selected inputs together.
Their historical token heuristics and fixes remain unchanged.
New node constraints use shared Unicode measures.
Editorial checks remain opt-in; they do not certify grammar or full STE100 compliance.

The [Portos example](../examples/portos/rules.yaml) uses the single bundled portos set.
Version-1 files and previous CLI aliases are rejected.
