import yaml
import tempfile
import unittest
from pathlib import Path
import importlib.util

spec = importlib.util.spec_from_file_location("site_builder", Path(__file__).with_name("site.py"))
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)


class DocumentationSite(unittest.TestCase):
    def fixture(self):
        rule = {"id": "text.fixture", "kind": "markdown", "category": "text", "recommendedSeverity": "error", "fixable": False,
                "options": {"scope": "string"}, "defaults": {"scope": "prose"}, "presets": ["text:fixture"]}
        ref = {"summary": "Checks fixture prose.", "parameters": {"scope": "Choose prose or headings."}, "example-options": {"scope": "prose"}, "example": {"language": "markdown", "bad": "Bad prose.\n", "good": "Good prose.\n", "explanation": "The example violates the configured policy."}}
        return rule, ref

    def test_complete_parameter_reference_and_configuration(self):
        rule, ref = self.fixture()
        builder.validate([rule], {rule["id"]: ref})
        page = builder.rule_page(rule, ref)
        self.assertIn('check: text.fixture', page)
        self.assertIn("Choose prose or headings.", page)
        self.assertIn("`text:fixture`", page)
        self.assertIn("## Violation example", page)
        self.assertIn("### Why it fails", page)
        self.assertNotIn("```json", page)
        import re
        configuration = yaml.safe_load(re.search(r"```yaml\n(.*?)\n```", page, re.S)[1])
        self.assertEqual(configuration["rules"][0]["options"], ref["example-options"])
        for rules, refs in [([rule, rule], {rule["id"]: ref}), ([rule], {}), ([rule], {rule["id"]: dict(ref, summary="")}),
                            ([rule], {rule["id"]: dict(ref, parameters={})}), ([rule], {rule["id"]: dict(ref, **{"example-options": []})})]:
            with self.assertRaises(ValueError):
                builder.validate(rules, refs)
        simple = dict(rule, options={}, presets=[], fixable=True)
        page = builder.rule_page(simple, dict(ref, **{"example-options": {}, "notes": "Editorial review."}))
        self.assertIn("no parameters", page)
        self.assertIn("Editorial review.", page)

    def test_generation_and_project_site_links(self):
        rule, ref = self.fixture()
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / "docs/stylesheets").mkdir(parents=True)
            builder.write(root / "docs/home.md", "# Fixture\n")
            builder.write(root / "docs/library.md", "[Example](../examples/custom/main.go)\n")
            builder.write(root / "docs/stylesheets/extra.css", "body { color: black; }")
            builder.write(root / "docs/schema.json", "{}")
            config = {"repository": "portpowered/fixture", "title": "Fixture", "description": "Fixture docs", "guides": []}
            builder.generate(root, [rule], {rule["id"]: ref}, config)
            self.assertIn("github.com/portpowered/fixture/blob/main/examples", (root / ".site-docs/library.md").read_text())
            self.assertTrue((root / ".site-docs/rules/text.fixture.md").exists())
            self.assertEqual(yaml.safe_load((root / "mkdocs.generated.yml").read_text())["site_url"], "https://portpowered.github.io/fixture/")

    def test_named_rule_families_are_nested_without_duplicate_pages(self):
        config = yaml.safe_load((builder.ROOT / "docs/site.yaml").read_text(encoding="utf-8"))
        catalog = [{"id": name, "category": name.split(".")[0]} for name in [
            "markdown.single-title", "text.matcher", "text.repeated-word",
            "text.ste100.dictionary", "text.ste100.grammar",
            "text.strunk-white.needless-words", "text.strunk-white.fancy-words",
        ]]
        navigation = builder.rule_navigation(catalog, config)
        self.assertEqual(navigation, [
            {"Markdown": [{"markdown.single-title": "rules/markdown.single-title.md"}]},
            {"Text": [
                {"text.matcher": "rules/text.matcher.md"},
                {"text.repeated-word": "rules/text.repeated-word.md"},
                {"STE100": [
                    {"text.ste100.dictionary": "rules/text.ste100.dictionary.md"},
                    {"text.ste100.grammar": "rules/text.ste100.grammar.md"},
                ]},
                {"Strunk and White": [
                    {"text.strunk-white.needless-words": "rules/text.strunk-white.needless-words.md"},
                    {"text.strunk-white.fancy-words": "rules/text.strunk-white.fancy-words.md"},
                ]},
            ]},
        ])
        self.assertEqual(builder.rule_navigation(catalog[:2], config), [
            {"Markdown": [{"markdown.single-title": "rules/markdown.single-title.md"}]},
            {"Text": [{"text.matcher": "rules/text.matcher.md"}]},
        ])
        self.assertEqual(builder.rule_navigation(catalog[:2], {}), builder.rule_navigation(catalog[:2], config))

    def test_all_rendered_links_and_anchors(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            with self.assertRaises(ValueError):
                builder.check_html(root)
            builder.write(root / "index.html", '<a href="child/#target">Child</a><a href="https://example.test">External</a>')
            builder.write(root / "child/index.html", '<h1 id="target">Target</h1><a href="../">Home</a>')
            builder.check_html(root)
            builder.write(root / "index.html", '<a href="/fixture/child/#target">Project path</a>')
            builder.check_html(root, "/fixture/")
            for link in ["missing/", "child/#missing", "/absolute/"]:
                builder.write(root / "index.html", '<a href="' + link + '">Bad</a>')
                with self.assertRaises(ValueError):
                    builder.check_html(root)

    def test_rejects_incomplete_or_ambiguous_rule_examples(self):
        rule, ref = self.fixture()
        for example in [{}, dict(ref["example"], language="json"), dict(ref["example"], explanation=""), dict(ref["example"], good=ref["example"]["bad"]), dict(ref["example"], surprise=True)]:
            with self.subTest(example=example), self.assertRaises(ValueError):
                builder.validate([rule], {rule["id"]: dict(ref, example=example)})
        for patch in [{"surprise": True}, {"parameters": {"scope": ""}}, {"example-options": {"missing": True}}]:
            with self.subTest(patch=patch), self.assertRaises(ValueError):
                builder.validate([rule], {rule["id"]: dict(ref, **patch)})

    def test_api_examples_are_yaml_contract_fragments(self):
        rule, ref = self.fixture()
        rule["kind"] = "openapi"
        ref["example"] = {"language": "yaml", "bad": "type: string", "good": "type: object", "explanation": "The response needs an object."}
        builder.validate([rule], {rule["id"]: ref})
        for bad in ["[", "[]", "null"]:
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                builder.validate([rule], {rule["id"]: dict(ref, example=dict(ref["example"], bad=bad))})
