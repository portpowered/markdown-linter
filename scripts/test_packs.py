import hashlib
import tempfile
import unittest
from pathlib import Path
import packs


class PackHashes(unittest.TestCase):
    def test_canonical_bytes_and_mismatch(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            path = root / "pack.yaml"
            path.write_bytes(b"version: 1\r\n")
            entries = [{"name": "fixture", "file": "pack.yaml", "sha256": "wrong"}]
            with self.assertRaises(ValueError):
                packs.update(entries, root)
            packs.update(entries, root, write=True)
            self.assertEqual(path.read_bytes(), b"version: 1\n")
            self.assertEqual(entries[0]["sha256"], hashlib.sha256(path.read_bytes()).hexdigest())
            packs.update(entries, root)
            path.write_bytes(b"changed\n")
            with self.assertRaises(ValueError):
                packs.update(entries, root)
