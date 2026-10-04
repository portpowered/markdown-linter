import hashlib
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch
import zipfile

import package


class ArchiveTests(unittest.TestCase):
    def make_root(self, root):
        for name in ["LICENSE", "NOTICE", "docs/rule-packs.md", "docs/rule-catalog.json",
                     "docs/strunk-white.md", "docs/ste100.md", "docs/single-file-policy.md",
                     "pkg/rulepack/packs/default.yaml", "pkg/contract/ruledefs/core.limit.yaml",
                     "examples/ste100/rules.yaml", "runtime/node_modules/mermaid/package.json",
                     "schemas/checks/core.limit.schema.json", "examples/v2/.marklint.yaml"]:
            path = root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("fixture", encoding="utf-8")

    def test_single_build_per_platform_and_complete_archives(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self.make_root(root)
            built = []

            def build(command, cwd, env, check):
                self.assertEqual(cwd, root)
                self.assertTrue(check)
                self.assertEqual(env["CGO_ENABLED"], "0")
                built.append((env["GOOS"], env["GOARCH"]))
                binary = Path(command[command.index("-o") + 1])
                binary.write_bytes(b"compiled")
                binary.chmod(0o755)

            with patch.object(package.subprocess, "run", side_effect=build):
                archives = package.build_archives(root, "v2.0.0")
            self.assertEqual(built, list(package.TARGETS))
            self.assertEqual(len(set(built)), 6)
            self.assertEqual(len(archives), 6)
            for archive in archives:
                if archive.suffix == ".zip":
                    with zipfile.ZipFile(archive) as bundle:
                        names = bundle.namelist()
                        self.assertEqual(bundle.read("marklint.exe"), b"compiled")
                else:
                    with tarfile.open(archive) as bundle:
                        names = bundle.getnames()
                        self.assertEqual(bundle.extractfile("marklint").read(), b"compiled")
                        self.assertTrue(bundle.getmember("marklint").mode & 0o111)
                self.assertEqual(len(names), len(set(names)))
                for entry in ["runtime/node_modules/mermaid/package.json",
                              "pkg/contract/ruledefs/core.limit.yaml",
                              "schemas/checks/core.limit.schema.json", "examples/v2/.marklint.yaml"]:
                    self.assertIn(entry, names)
            checksums = (root / "dist/checksums.txt").read_text().splitlines()
            self.assertEqual(len(checksums), 6)
            for line in checksums:
                digest, name = line.split()
                self.assertEqual(digest, hashlib.sha256((root / "dist" / name).read_bytes()).hexdigest())

    def test_duplicate_or_invalid_target_rejected_before_build(self):
        with patch.object(package.subprocess, "run") as build:
            for targets in [(package.TARGETS[0], package.TARGETS[0]), (("linux", "386"),)]:
                with self.assertRaises(ValueError):
                    package.build_archives(Path("missing"), "v2.0.0", targets)
            build.assert_not_called()

    def test_version_and_runtime_assets_required(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            self.make_root(root)
            with self.assertRaises(ValueError):
                package.build_archives(root, "../../bad")
            (root / "runtime/node_modules/mermaid/package.json").unlink()
            with self.assertRaisesRegex(ValueError, "runtime dependencies"):
                package.build_archives(root, "v2.0.0")


if __name__ == "__main__":
    unittest.main()
