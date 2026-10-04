"""Check or refresh JSON schemas generated from the canonical YAML rule baseline."""
import argparse
import json
import os
import subprocess
import tempfile
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parent.parent
SCHEMAS = {"config": "config", "report": "report", "resolved-core-rule": "resolved",
           "check-descriptor": "check-descriptor", "measure-descriptor": "measure-descriptor",
           "rule-definition": "rule-definition"}


def schema_assets(root, command):
    assets = {}
    for filename, kind in SCHEMAS.items():
        assets[root / "schemas" / (filename + ".schema.json")] = json.loads(command(["config", "schema", "--schema", kind]))
    installed = {descriptor["id"]: descriptor for descriptor in json.loads(command(["rules", "list", "--format", "json"]))}
    definitions = {}
    for path in sorted((root / "pkg/contract/ruledefs").glob("*.yaml")):
        definition = yaml.safe_load(path.read_text(encoding="utf-8"))
        descriptor = definition["descriptor"]
        identifier = descriptor["id"]
        if identifier != path.stem or identifier in definitions:
            raise ValueError("Invalid or duplicate rule definition ID: " + str(path))
        definitions[identifier] = descriptor
        schema = dict(descriptor["parametersSchema"])
        schema["$schema"] = "https://json-schema.org/draft/2020-12/schema"
        assets[root / "schemas/checks" / (identifier + ".schema.json")] = schema
    if not definitions or definitions != installed:
        raise ValueError("Canonical rule definitions differ from installed check descriptors")
    return assets


def synchronize(root, assets, update=False):
    expected = set(assets)
    existing = set((root / "schemas").glob("*.schema.json")) | set((root / "schemas/checks").glob("*.schema.json"))
    stale = existing - expected
    if stale and not update:
        raise ValueError("Obsolete generated schemas: " + ", ".join(str(path.relative_to(root)) for path in sorted(stale)))
    for path in stale:
        path.unlink()
    for path, schema in assets.items():
        content = json.dumps(schema, indent=2, sort_keys=True, ensure_ascii=False) + "\n"
        if update:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content, encoding="utf-8", newline="\n")
        elif not path.exists() or path.read_text(encoding="utf-8") != content:
            raise ValueError("Generated schema is stale: " + str(path.relative_to(root)) + "; run make rules-update")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write", action="store_true", help="refresh committed generated schemas")
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="marklint-schemas-") as temporary:
        binary = Path(temporary) / ("marklint.exe" if os.name == "nt" else "marklint")
        subprocess.run(["go", "build", "-o", str(binary), "./cmd/marklint"], cwd=ROOT, check=True)
        def command(arguments):
            return subprocess.check_output([str(binary), *arguments], cwd=ROOT)
        assets = schema_assets(ROOT, command)
        synchronize(ROOT, assets, args.write)
    print(f"Verified {len(assets)} canonical schema assets")


if __name__ == "__main__":
    main()
