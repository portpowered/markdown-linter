# OpenAPI and Markdown linter implementation checklist

The approved design uses one configuration version, `version: 2`, with breaking changes allowed. Rule set names have no version suffix. Pin the executable release and review the shipped SHA-256 manifests to reproduce policy. Both commands now default to their recommended set; Portos policy is opt-in through `portos`.

Implementation updated October 3, 2026. Checked items are implemented; rule behavior, options, and boundaries are documented in `docs/rule-packs.md`. This joint document is mirrored in both repositories. Unperformed customer pilot work is listed separately and is not claimed as completed.

## Existing systems and lessons

These are capability comparisons from primary documentation, not benchmark results or claims of compatibility. The recommendations below are our design choices.

| System | Relevant capabilities | Lesson for our linters |
| --- | --- | --- |
| Spectral | An OpenAPI ruleset with per-rule recommendation/severity metadata, operation-ID uniqueness, parameter consistency, and version-aware rules. [Rule reference](https://github.com/stoplightio/spectral/blob/develop/docs/reference/openapi-rules.md) | Provide named sets and metadata; make applicability explicit |
| Redocly CLI | Structural and reference checks alongside API documentation/style checks; built-in and inherited configurations. [Rule catalog](https://redocly.com/docs/cli/rules/built-in-rules), [extends](https://redocly.com/docs/cli/configuration/extends) | Separate correctness from customer conventions and document merge order |
| markdownlint | Rule IDs, categories, options, fixes, image alt text, headings, fences, and spacing. Ordered lists support multiple numbering conventions. [Rules](https://github.com/DavidAnson/markdownlint/blob/main/doc/Rules.md) | Give granular controls and avoid rejecting valid Markdown conventions |
| remark lint | Separate consistency and recommended presets; individual Markdown plugins. [Project documentation](https://github.com/remarkjs/remark-lint) | Offer a small recommended set and a separate style set |
| Vale | Declarative prose checks with scopes, messages, guidance links, and styles distributed as packages. [Styles](https://docs.vale.sh/topics/styles), [packages](https://docs.vale.sh/topics/packages) | Start with customer terminology and scoped prose patterns instead of broad grammar judgments |
| textlint | Explicit rule/preset configuration and plugin-based text processing. [Configuration](https://raw.githubusercontent.com/textlint/textlint/master/docs/configuring.md) | Keep prose checks opt-in and distinguish activation from installing executable logic |

Combine repository document checks and the Go extension API. Provide consistent rule discovery and onboarding across both commands. Do not implement Spectral selectors or JavaScript plugins merely to match another product's configuration language.

## Portos project requirements

The internal requirements are independent rules. In OpenAPI, `portos` composes `openapi:recommended` and `portos:internal`. In Markdown it composes `markdown:recommended` and `portos:internal`. Each requirement can be configured, scoped, overridden, or suppressed independently.

- [x] `portos.operation-vocabulary`: request Path operation IDs use List, Send, Delete, Query, Get, or Modify; Batch and Async are modifiers. Vocabulary and modifiers are configurable. Create, Update, Search, and Enumerate receive findings.
- [x] `portos.query-request`: Query uses POST and an object body.
- [x] `portos.async-response`: Async successes reference the configurable `AsyncIdentifier` object and expose only the identifier field, default `id`.
- [x] `portos.collection-response`: Query/List success objects contain a `results` array of objects and `paginationContext`. Async responses use their identifier contract.
- [x] `portos.pagination-response`: the response pagination context declares string `nextToken` and integer `maxResults`.
- [x] `portos.path-description`: each request Path Item has a nonblank description, independently of operation descriptions.
- [x] `portos.pagination-request`: List declares nextToken/maxResults query parameters; Query includes these in its object body, directly or under paginationContext.
- [x] `portos.open-enums`: open values use `x-extensible-enum` and corresponding unique `x-enum-varnames`. A normal `enum` is closed and is rejected, except exact sort-direction and error-family protocol enums with varnames.
- [x] `portos.date-fields`: date/time properties use string `format: date-time`, representing RFC 3339. Additional field names are configurable. The requested rfc3999 is interpreted as RFC 3339.
- [x] `portos.batch-contract`: Batch request items and synchronous results are objects with IDs. Async Batch uses the async success contract. Field names are configurable. Runtime ID uniqueness/correlation requires service tests.
- [x] `portos.query-graph`: The Query request references the canonical recursive Query model. Checks validate comparator and boolean graph shapes.
- [x] `portos.name-schema`: data properties named name reference the configurable NameValue object.
- [x] `portos.description-schema`: data properties named description reference the configurable DescriptionValue object; OpenAPI metadata descriptions are unaffected.

- [x] `portos.success-status`: exact 200 or 202 success responses only, with at least one declared success.
- [x] `portos.etag-conflict`: conditional ETag request headers require 409 and reject 412.
- [x] `portos.error-contract`: Declared client, server and default errors reference ErrorResponse. It requires string code, message and type. Integer family permits 400/500.
- [x] `portos.path-kebab-case`: literal request path segments use lowercase kebab-case; parameter names are independent.
- [x] `portos.delete-idempotent`: explicit x-portos-idempotent declaration and no absent-resource 404/410; runtime repeats require service tests.
- [x] `portos.version-prefix`: positive major version prefixes such as /v2/groups and /v3/groups.
- [x] `portos.query-sorts`: optional sorting is a list of closed objects containing required direction ASCENDING/DESCENDING and key:string.
- [x] `portos.batch-outcomes`: Synchronous results and errors lists require request-compatible IDs. Each failure references ErrorResponse. ID correlation requires service tests.

- [x] `portos.prose-dashes` (`text.matcher`): no hyphens or Unicode dashes in prose, excluding code and URL targets. Markdown bullet/fence syntax is not prose.
- [x] `portos.banned-phrases` (`text.matcher`): prohibit that phrase in prose, including its spelling with a hyphen; exclude code and URL targets.

Backend evidence: `portos-backend/api/restful_interfaces/components/schemas/Query.yaml`, `QueryComparator.yaml`, `PaginationContext.yaml`, `EndpointQueryRequest.yaml`, and `EndpointQueryResponse.yaml`. The graph contains match, lessThan, greaterThan, and, or, not, patternMatch, and freeformMatch. Comparators have string key/value fields. Response collection and pagination shapes are checked independently because OpenAPI has no Go generics.

The backend spells the field maxResults and currently uses TypeValue for some names. The confirmed NameValue/DescriptionValue names deliberately establish the requested new policy and remain configurable. Backend contracts were inspected but not edited. Schema rules do not prove that running handlers implement those contracts.

## General OpenAPI checklist

- [x] `openapi.auth-required`: auth required.
- [x] `openapi.enum-values`: enum values.
- [x] `openapi.error-response`: error response.
- [x] `openapi.examples-valid`: examples valid.
- [x] `openapi.info-contact`: info contact.
- [x] `openapi.info-description`: info description.
- [x] `openapi.info-license`: info license.
- [x] `openapi.operation-description`: operation description.
- [x] `openapi.operation-id`: operation id.
- [x] `openapi.operation-id-unique`: operation id unique.
- [x] `openapi.operation-success-response`: operation success response.
- [x] `openapi.operation-summary`: operation summary.
- [x] `openapi.pagination`: pagination.
- [x] `openapi.parameter-description`: parameter description.
- [x] `openapi.parameters-unique`: parameters unique.
- [x] `openapi.path-naming`: path naming.
- [x] `openapi.path-parameters`: path parameters.
- [x] `openapi.path-syntax`: path syntax.
- [x] `openapi.property-camel-case`: property camel case.
- [x] `openapi.ref-siblings`: ref siblings.
- [x] `openapi.request-response-example`: request response example.
- [x] `openapi.schema-description`: schema description.
- [x] `openapi.schema-naming`: schema naming.
- [x] `openapi.security-references`: security references.
- [x] `openapi.server-policy`: server policy.
- [x] `openapi.server-variables`: server variables.
- [x] `openapi.spec-structure`: spec structure.
- [x] `openapi.tags-defined`: tags defined.
- [x] `openapi.unused-components`: unused components.

- [x] `schema.no-anonymous-objects` and `schema.property-camel-case`: explicit standalone schema conventions.

Each check has independent options and activation; see the rule-pack reference for exact behavior and exclusions.

## Markdown and prose checklist

- [x] `markdown.blank-lines`: blank lines.
- [x] `markdown.doc-id-unique`: doc id unique.
- [x] `markdown.document-identifier`: document identifier.
- [x] `markdown.document-structure`: document structure.
- [x] `markdown.fence-closed`: fence closed.
- [x] `markdown.fence-language`: fence language.
- [x] `markdown.final-newline`: final newline.
- [x] `markdown.formatting`: formatting.
- [x] `markdown.frontmatter-valid`: frontmatter valid.
- [x] `markdown.heading-duplicates`: heading duplicates.
- [x] `markdown.heading-order`: heading order.
- [x] `markdown.html-policy`: html policy.
- [x] `markdown.image-alt`: image alt.
- [x] `markdown.line-length`: line length.
- [x] `markdown.link-relocation`: link relocation.
- [x] `markdown.link-text`: link text.
- [x] `markdown.list-style`: list style.
- [x] `markdown.local-links`: local links.
- [x] `markdown.ordered-list`: ordered list.
- [x] `markdown.reference-definitions`: reference definitions.
- [x] `markdown.required-heading`: required heading.
- [x] `markdown.single-title`: single title.
- [x] `markdown.trailing-whitespace`: trailing whitespace.
- [x] `text.repeated-word`: repeated word.
- [x] `text.spelling`: spelling.
- [x] `text.terminology`: terminology.

Each check has independent options and activation; see the rule-pack reference for exact behavior and exclusions.
## Version 2 contract

Version 2 replaces the earlier customer configuration format.
The single configuration contains named rules, sets, templates, routing, vocabulary, and reasoned suppressions.
Rules inherit through shallow option patches.
No customer template or dictionary file is read.

The five core checks provide numeric limits, text matching, block sequences, table schemas, and Mermaid flowchart checks.
Unicode segmentation supplies word, sentence, and grapheme counts.
MDX uses a syntax parser and keeps dynamic content opaque.
Installed measure adapters declare their supported inputs and native scales.
Remote adapters require explicit operator permission.

The [single-file guide](single-file-policy.md) documents the active contract and operational limits.
CI checks source Markdown, generated reference pages, and the supplied guide example.
Race tests and the existing 95% coverage gate apply to the module.

## Further work

Run an onboarding pilot with real customers before claiming usability results.
Keep editorial judgment separate from lexical checks.
Review every vocabulary update and provider capability declaration.
