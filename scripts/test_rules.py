import tempfile
import unittest
from pathlib import Path

import rules
import importlib.util
import yaml

spec = importlib.util.spec_from_file_location('site_builder', Path(__file__).with_name('site.py'))
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)


class CanonicalRules(unittest.TestCase):
    def test_scan_covers_every_baseline_and_core_check(self):
        catalog, refs = builder.load_rule_definitions(builder.ROOT)
        ids = {item['id'] for item in catalog}
        self.assertEqual(len(ids), len(list((builder.ROOT / 'pkg/contract/ruledefs').glob('*.yaml'))))
        self.assertTrue({'core.limit', 'core.match', 'core.sequence', 'markdown.table-schema', 'mermaid.flowchart'} <= ids)
        self.assertEqual(set(refs), ids)
        for item in catalog:
            page = builder.rule_page(item, refs[item['id']])
            self.assertIn('example.rule:', page)
            self.assertIn(item['id'] + '.schema.json', page)

    def test_missing_unknown_and_mismatched_definition_files(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            with self.assertRaises(ValueError):
                builder.load_rule_definitions(root)
            directory = root / 'pkg/contract/ruledefs'
            directory.mkdir(parents=True)
            sample = yaml.safe_load(next((builder.ROOT / 'pkg/contract/ruledefs').glob('*.yaml')).read_text(encoding='utf-8'))
            for value, name in [({}, 'invalid'), (dict(sample, surprise=True), sample['descriptor']['id']), (sample, 'wrong')]:
                path = directory / (name + '.yaml')
                path.write_text(yaml.safe_dump(value), encoding='utf-8')
                with self.assertRaises(ValueError):
                    builder.load_rule_definitions(root)
                path.unlink()

    def test_schema_updates_detect_drift_and_obsolete_files(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            path = root / 'schemas/config.schema.json'
            assets = {path: {'type': 'object'}}
            with self.assertRaises(ValueError):
                rules.synchronize(root, assets)
            rules.synchronize(root, assets, True)
            rules.synchronize(root, assets)
            path.write_text('{}\n', encoding='utf-8')
            with self.assertRaises(ValueError):
                rules.synchronize(root, assets)
            stale = root / 'schemas/checks/deleted.schema.json'
            stale.parent.mkdir()
            stale.write_text('{}', encoding='utf-8')
            with self.assertRaises(ValueError):
                rules.synchronize(root, assets)
            rules.synchronize(root, assets, True)
            self.assertFalse(stale.exists())

    def test_schema_scan_matches_entire_runtime_descriptor(self):
        descriptor = {'id': 'core.fixture', 'parametersSchema': {'type': 'object', 'properties': {}}}
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            directory = root / 'pkg/contract/ruledefs'
            directory.mkdir(parents=True)
            path = directory / 'core.fixture.yaml'
            path.write_text(yaml.safe_dump({'descriptor': descriptor}), encoding='utf-8')
            import json
            def command(args):
                return json.dumps([descriptor] if args[0] == 'rules' else {'type': 'object'})
            assets = rules.schema_assets(root, command)
            self.assertEqual(len(assets), len(rules.SCHEMAS) + 1)
            self.assertEqual(assets[root / 'schemas/checks/core.fixture.schema.json']['type'], 'object')
            with self.assertRaises(ValueError):
                rules.schema_assets(root, lambda args: '[]' if args[0] == 'rules' else '{}')

