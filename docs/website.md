# Documentation website

The GitHub Pages site uses a static documentation layout with a searchable rule reference, sidebar navigation, light and dark themes, and code-copy controls. It follows the customer-reference approach of the [Portpowered library template](https://github.com/portpowered/go-third-party-template) and [go-ring site](https://portpowered.github.io/go-ring/). The template action expects wire schemas, so this rules-and-library site uses Material for MkDocs without generating a fictitious API.

## Build and preview

Create and activate a Python virtual environment, then run:

```sh
make docs-deps
make docs
python -m mkdocs serve --config-file mkdocs.generated.yml
```

Dependencies are pinned in `docs/requirements.txt`. `make docs` verifies that the catalog matches the running CLI, generates customer pages, builds with strict Markdown/link validation and checks local links and anchors throughout the rendered site. Generated `.site-docs`, `mkdocs.generated.yml` and `site` are ignored by Git.

## Update a rule

The registered factories remain the source of option types, defaults, input kind and pack membership. After changing those, run `make docs-update` to refresh `docs/rule-catalog.json`. Add or update `docs/rule-reference.json` with a reviewed behavior summary, parameter descriptions and example options. Every catalog check and every parameter must have an entry. The Go tests compile every reference configuration against the stock registry, so unsupported examples fail verification.

Keep CLI onboarding in `getting-started.md` and embedding instructions in `library.md`. Existing composition, STE100, editorial and API-standard guides are published alongside individual rule pages. Source and example links point to the repository; local schema and catalog downloads ship with the site.

## CI and publication

One Ubuntu quality job runs the same `make verify` used locally: pack integrity, formatting, build, vet, all standard Go linters, race tests, 95% aggregate module coverage, tooling tests, module tidiness, CLI examples and the strict site build. Three compatibility jobs build and run race tests on Linux, macOS and Windows with the minimum supported Go 1.24 series. They do not repeat the entire quality suite.

Pull requests build and validate the site without deploying. Main pushes upload the static site, and the deployment job publishes only after quality and all compatibility jobs pass. The repository Pages source is GitHub Actions. The workflow exposes the live URL through its github-pages environment. No release tag is required for documentation updates.

## Embedded pack hashes

Embedded YAML is canonical LF text. After editing packs, run `python scripts/packs.py --write` to normalize bytes and refresh hashes. `make verify` rejects noncanonical bytes and stale hashes. This prevents a Windows working-copy hash from disagreeing with the Git checkout used in CI.
