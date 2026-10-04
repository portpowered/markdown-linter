"""Generate the customer site from verified catalog metadata and reviewed rule descriptions."""
import argparse
import json
import yaml
import re
import shutil
import subprocess
from html.parser import HTMLParser
from pathlib import Path
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parent.parent


def write(path, content):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content, encoding="utf-8", newline="\n")


def table(value):
    return str(value).replace("|", "\\|").replace("\n", " ")


def option_type(name, kind):
    types = {"string": "string", "int": "integer", "bool": "boolean", "*bool": "boolean", "[]string": "list of strings", "map[string]string": "string mapping"}
    structured = {"identifiers": "list of identifier objects", "types": "list of document group objects", "moves": "path mapping", "heading": "string", "level": "integer", "allow-directories": "boolean"}
    return types.get(kind, structured.get(name, "structured value; see behavior"))


def rule_page(rule, reference):
    options = reference["example-options"]
    config = {"version": 1, "rules": [{"id": rule["id"], "check": rule["id"], "severity": rule["recommendedSeverity"]}]}
    if options:
        config["rules"][0]["options"] = options
    page = "---\n" + yaml.safe_dump({"title": rule["id"], "check": rule["id"]}, sort_keys=False) + "---\n\n"
    page += f"# {rule['id']}\n\n{reference['summary']}\n\n"
    page += f"Input kind: **{rule['kind']}** · Suggested severity: **{rule['recommendedSeverity']}** · Automatic fix: **{'available' if rule['fixable'] else 'none'}**.\n\n"
    page += "## Configuration\n\nSave this YAML in your linter configuration file. "
    page += "This selects this check independently; compose a [rule pack](../rule-packs.md) to select related checks.\n\n"
    page += "```yaml\n" + yaml.safe_dump(config, sort_keys=False).rstrip() + "\n```\n\n"
    example = reference["example"]
    page += "## Violation example\n\n"
    if rule["kind"] != "markdown":
        page += "These focused YAML fragments show the relevant contract fields. The surrounding document and referenced schemas are omitted.\n\n"
    page += "### Fails\n\n~~~~" + example["language"] + "\n" + example["bad"].rstrip("\n") + "\n~~~~\n\n"
    page += "### Why it fails\n\n" + example["explanation"] + "\n\n"
    page += "### Corrected example\n\n~~~~" + example["language"] + "\n" + example["good"].rstrip("\n") + "\n~~~~\n\n"
    page += "## Parameters\n\n"
    if rule.get("options"):
        page += "| Name | Type | Default | Behavior |\n| --- | --- | --- | --- |\n"
        for name, kind in sorted(rule["options"].items()):
            default = rule.get("defaults", {}).get(name)
            display = "Unset; see behavior" if default is None else "`" + table(yaml.safe_dump(default, default_flow_style=True, width=10000).strip().removesuffix("\n...")) + "`"
            page += f"| `{name}` | {option_type(name, kind)} | {display} | {table(reference['parameters'][name])} |\n"
    else:
        page += "This check has no parameters. Omit `options` or use an empty mapping.\n"
    page += "\n## Rule packs\n\n"
    if rule.get("presets"):
        page += "Included in: " + ", ".join("`" + name + "`" for name in rule["presets"]) + ".\n"
    else:
        page += "Select this check explicitly in a custom pack.\n"
    page += "\nRule-level `enabled`, `severity`, `include`, and `exclude` fields apply to every check. "
    page += "Overrides replace the entire supplied options mapping. See [configuration and composition](../rule-packs.md) for scopes and reasoned exceptions.\n"
    if reference.get("notes"):
        page += "\n## Usage notes\n\n" + reference["notes"] + "\n"
    return page


def validate(catalog, references):
    ids = [rule["id"] for rule in catalog]
    if len(set(ids)) != len(ids) or set(ids) != set(references):
        raise ValueError("Rule descriptions must cover every catalog ID exactly once")
    for rule in catalog:
        reference = references[rule["id"]]
        if not isinstance(reference, dict) or set(reference) - {"summary", "parameters", "example-options", "example", "notes"}:
            raise ValueError("Unknown rule reference fields: " + rule["id"])
        if not reference.get("summary", "").strip():
            raise ValueError("Missing rule summary: " + rule["id"])
        if set(reference.get("parameters", {})) != set(rule.get("options", {})):
            raise ValueError("Parameter descriptions differ from registered options: " + rule["id"])
        if not isinstance(reference.get("example-options"), dict):
            raise ValueError("Configuration example must be an options mapping: " + rule["id"])
        if set(reference["example-options"]) - set(rule.get("options", {})):
            raise ValueError("Configuration example uses unregistered options: " + rule["id"])
        if any(not isinstance(value, str) or not value.strip() for value in reference["parameters"].values()):
            raise ValueError("Parameter descriptions must be nonempty text: " + rule["id"])
        example = reference.get("example", {})
        if not isinstance(example, dict) or set(example) != {"language", "bad", "good", "explanation"}:
            raise ValueError("Rule example requires language, bad, good and explanation: " + rule["id"])
        expected_language = "markdown" if rule["kind"] == "markdown" else "yaml"
        if example["language"] != expected_language or any(not isinstance(example[key], str) or not example[key].strip() for key in ["bad", "good", "explanation"]):
            raise ValueError("Invalid violation example format: " + rule["id"])
        if example["bad"] == example["good"]:
            raise ValueError("Violation and corrected examples must differ: " + rule["id"])
        if expected_language == "yaml":
            for key in ["bad", "good"]:
                try:
                    fragment = yaml.safe_load(example[key])
                except yaml.YAMLError as error:
                    raise ValueError("API example must be valid YAML: " + rule["id"]) from error
                if not isinstance(fragment, dict) or not fragment:
                    raise ValueError("API example must contain contract fields: " + rule["id"])


def rule_navigation(catalog, config):
    groups = {}
    sections = config.get("rule-groups", {})
    for rule in catalog:
        category = rule["category"]
        groups.setdefault(category, [])
    for category in sorted(groups):
        category_rules = [rule for rule in catalog if rule["category"] == category]
        nested = [(section, []) for section in sections.get(category, [])]
        for rule in category_rules:
            page = {rule["id"]: "rules/" + rule["id"] + ".md"}
            for section, pages in nested:
                if rule["id"].startswith(section["prefix"]):
                    pages.append(page)
                    break
            else:
                groups[category].append(page)
        groups[category] += [{section["title"]: pages} for section, pages in nested if pages]
    return [{name.title(): pages} for name, pages in sorted(groups.items())]


def generate(root, catalog, references, config):
    validate(catalog, references)
    output = root / ".site-docs"
    if output.is_symlink():
        raise ValueError("The generated documentation directory cannot be a symlink")
    if output.exists():
        shutil.rmtree(output)
    output.mkdir(exist_ok=True)
    repo = config["repository"]
    for source in (root / "docs").glob("*.md"):
        content = source.read_text(encoding="utf-8")
        # Repository examples and source references remain browsable on GitHub.
        content = re.sub(r"\]\(\.\./((?:examples|pkg|rulepack|scripts)/[^)]+)\)", lambda m: "](https://github.com/" + repo + "/blob/main/" + m[1] + ")", content)
        write(output / source.name, content)
    for source in list((root / "docs").glob("*.json")) + list((root / "docs").glob("*.yaml")):
        shutil.copyfile(source, output / source.name)
    for source in (root / "docs" / "stylesheets").glob("*.css"):
        write(output / "stylesheets" / source.name, source.read_text(encoding="utf-8"))
    write(output / "index.md", (root / "docs" / "home.md").read_text(encoding="utf-8"))
    index = "# Rule reference\n\nSearch by rule ID, behavior, or parameter name. Every registered rule has its own configuration reference.\n\n| Rule | Behavior |\n| --- | --- |\n"
    for rule in catalog:
        identifier = rule["id"]
        write(output / "rules" / (identifier + ".md"), rule_page(rule, references[identifier]))
        index += f"| [{identifier}]({identifier}.md) | {table(references[identifier]['summary'])} |\n"
    write(output / "rules" / "index.md", index)
    nav = [{"Overview": "index.md"}, {"Get started": "getting-started.md"}, {"Go library": "library.md"}, {"Rule packs": "rule-packs.md"}]
    nav += [{"Rules": [{"All rules": "rules/index.md"}] + rule_navigation(catalog, config)}]
    nav += [{"Guides": config["guides"]}]
    mkdocs = {
        "site_name": config["title"], "site_description": config["description"],
        "site_url": "https://portpowered.github.io/" + repo.split("/")[1] + "/",
        "repo_url": "https://github.com/" + repo, "repo_name": repo,
        "docs_dir": ".site-docs", "site_dir": "site", "use_directory_urls": True,
        "theme": {"name": "material", "font": False,
                  "features": ["navigation.sections", "navigation.top", "content.code.copy", "search.highlight", "search.suggest", "toc.follow"],
                  "palette": [{"scheme": "default", "primary": "indigo", "toggle": {"icon": "material/weather-night", "name": "Switch to dark theme"}},
                              {"scheme": "slate", "primary": "indigo", "toggle": {"icon": "material/weather-sunny", "name": "Switch to light theme"}}]},
        "plugins": ["search"], "nav": nav,
        "markdown_extensions": ["tables", "admonition", "pymdownx.highlight", "pymdownx.superfences", "pymdownx.details", {"toc": {"permalink": True}}],
        "extra_css": ["stylesheets/extra.css"],
        "validation": {"links": {"not_found": "warn", "anchors": "warn", "unrecognized_links": "warn"}},
    }
    write(root / "mkdocs.generated.yml", yaml.safe_dump(mkdocs, sort_keys=False))


class Links(HTMLParser):
    def __init__(self):
        super().__init__()
        self.links = []
        self.ids = set()

    def handle_starttag(self, tag, attrs):
        values = dict(attrs)
        if values.get("id"):
            self.ids.add(values["id"])
        for name in ["href", "src"]:
            if values.get(name):
                self.links.append(values[name])


def check_html(site, base_path=""):
    pages = {}
    for page in site.rglob("*.html"):
        parser = Links()
        parser.feed(page.read_text(encoding="utf-8"))
        pages[page.resolve()] = parser
    if not pages or not (site / "index.html").exists():
        raise ValueError("Site build has no home page")
    for page, parsed in pages.items():
        for link in parsed.links:
            target = urlsplit(link)
            if target.scheme or target.netloc or not target.path:
                destination = page if not target.scheme and not target.netloc else None
            else:
                if target.path.startswith("/"):
                    if not base_path or not target.path.startswith(base_path):
                        raise ValueError("URL escapes the project site: " + link)
                    destination = (site / unquote(target.path[len(base_path):])).resolve()
                else:
                    destination = (page.parent / unquote(target.path)).resolve()
                if not destination.is_relative_to(site.resolve()):
                    raise ValueError("URL escapes the generated site: " + link)
                if destination.is_dir():
                    destination /= "index.html"
                if not destination.exists():
                    raise ValueError(f"Broken site link in {page}: {link}")
            if destination in pages and target.fragment and unquote(target.fragment) not in pages[destination].ids:
                raise ValueError(f"Broken site anchor in {page}: {link}")
    print(f"Verified local links and anchors across {len(pages)} HTML pages")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--update", action="store_true")
    parser.add_argument("--check-html", action="store_true")
    args = parser.parse_args()
    if args.check_html:
        config = yaml.safe_load((ROOT / "docs" / "site.yaml").read_text(encoding="utf-8"))
        check_html(ROOT / "site", "/" + config["repository"].split("/")[1] + "/")
        return
    config = yaml.safe_load((ROOT / "docs" / "site.yaml").read_text(encoding="utf-8"))
    catalog = []
    for kind in config["kinds"]:
        command = ["go", "run", "./cmd/" + config["command"], "rules", "list", "--kind", kind, "--format", "json"]
        catalog += json.loads(subprocess.check_output(command, cwd=ROOT))
    catalog.sort(key=lambda rule: rule["id"])
    path = ROOT / "docs" / "rule-catalog.json"
    if args.update:
        write(path, json.dumps(catalog, indent=2) + "\n")
    elif catalog != json.loads(path.read_text(encoding="utf-8")):
        raise ValueError("Rule catalog is stale; run make docs-update")
    references = yaml.safe_load((ROOT / "docs" / "rule-reference.yaml").read_text(encoding="utf-8"))
    generate(ROOT, catalog, references, config)


if __name__ == "__main__":
    main()
