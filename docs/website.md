# Documentation website

The GitHub Pages site provides searchable rules, sidebar navigation, light and dark themes, and code-copy controls. It follows the customer-reference approach of the [Portpowered library template](https://github.com/portpowered/go-third-party-template) and [go-ring site](https://portpowered.github.io/go-ring/). The template action expects wire schemas, so this rules-and-library site uses Material for MkDocs without generating a fictitious API.

## Build and preview

Create and activate a Python virtual environment, then run:

```sh
make docs-deps
make docs
python -m mkdocs serve --config-file mkdocs.generated.yml
```

Dependencies are pinned in `docs/requirements.txt`. `make docs` verifies the catalog against canonical rule YAML and generates customer pages. It builds with strict Markdown validation and checks local links and anchors. Generated `.site-docs`, `mkdocs.generated.yml` and `site` are ignored by Git.

## Update a rule

The YAML files in `pkg/contract/ruledefs` define every check schema, default, target, fixture and documentation example. The runtime embeds these files. Edit the matching file when changing a check. Run `make rules-update` to refresh schema downloads, then `make docs-update` to refresh the catalog and pages. Every parameter requires a reviewed description. Tests validate the baseline against the installed checks.

Keep CLI onboarding in `getting-started.md` and embedding instructions in `library.md`. Existing composition, STE100, editorial and API-standard guides are published alongside individual rule pages. Source and example links point to the repository; local schema and catalog downloads ship with the site.

## CI and publication

One matrix runs `make verify` on the latest Linux, macOS and Windows runners with the current stable Go release. It checks pack integrity, formatting, builds, vet, standard Go linters, race tests and the 95% coverage gate. It also checks tooling, module tidiness, CLI examples and documentation.

Pull requests build and validate the site without deploying. Main pushes upload the static site, and the deployment job publishes only after every verification job passes. The repository Pages source is GitHub Actions. The workflow exposes the live URL through its github-pages environment. No release tag is required for documentation updates.

## Embedded pack hashes

Embedded YAML is canonical LF text. After editing packs, run `python scripts/packs.py --write` to normalize bytes and refresh hashes. `make verify` rejects noncanonical bytes and stale hashes. This prevents a Windows working-copy hash from disagreeing with the Git checkout used in CI.

## Rule page format

Each rule reference uses its canonical YAML definition. The [metadata schema](../schemas/rule-definition.schema.json) defines the required shape. CI compares every descriptor and options schema with the runtime. It rejects missing, changed and obsolete generated schema files. Unknown fields, missing explanations and identical failing and corrected examples fail the site build.

Every page has a summary, YAML configuration, violation explanation, corrected example, parameters and pack membership. API examples are focused fragments; declarations outside the illustrated contract need the normal surrounding document. Markdown examples preserve source text, including significant trailing spaces and final newlines in the metadata.

The generator owns the configuration code fence and YAML serialization. Each rendered configuration is parsed in generator tests, and Go tests compile its options against the registry. These format constraints belong to the documentation template instead of a customer prose rule.

## Badges and coverage

The home page and README follow go-ring's badge set. They show Go version, CI, coverage, release, reference, license and documentation. Coverage comes from the statement-weighted Go profile checked by `make verify`. After a passing build, `make coverage-publish` adds `coverage.svg` and the per-file `coverage.html` report to the site. The Linux main build publishes them with Pages. Failed builds cannot replace the last passing report. The CI badge links to the current workflow, while the coverage badge represents the last successful site deployment.

CI runs one verification matrix on Windows, Linux and macOS, all using the current stable Go release. Each runner executes the default Make checks. Linux supplies the Pages artifact; deployment waits for the full matrix.
