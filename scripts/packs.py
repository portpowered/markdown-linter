"""Regenerate embedded-pack hashes from canonical LF bytes, or verify them."""
import argparse
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
PACKS = ROOT / "pkg/rulepack/packs"


def update(entries, packs, write=False):
    for entry in entries:
        path = packs / entry["file"]
        raw = path.read_bytes()
        canonical = raw.replace(b"\r\n", b"\n")
        digest = hashlib.sha256(canonical).hexdigest()
        if write:
            path.write_bytes(canonical)
            entry["sha256"] = digest
        elif raw != canonical or entry["sha256"] != digest:
            raise ValueError(f"{entry['name']}: normalize LF and regenerate with python scripts/packs.py --write")
    return entries


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write", action="store_true")
    args = parser.parse_args()
    manifest = PACKS / "manifest.json"
    entries = update(json.loads(manifest.read_text(encoding="utf-8")), PACKS, args.write)
    if args.write:
        manifest.write_text(json.dumps(entries, indent=2) + "\n", encoding="utf-8", newline="\n")


if __name__ == "__main__":
    main()
