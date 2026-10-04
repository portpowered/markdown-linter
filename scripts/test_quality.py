import unittest
from pathlib import Path
import tempfile
from unittest.mock import patch
import quality
from quality import coverage_totals, coverage_badge


class CoverageTests(unittest.TestCase):
    def test_statement_weighted_and_every_path_counted(self):
        self.assertEqual(coverage_totals("mode: atomic\nmodule/main.go:1.1,2.1 1 0\nmodule/lib.go:2.1,3.1 99 12\n"), (99, 100))

    def test_no_rounding_to_pass(self):
        covered, total = coverage_totals("mode: atomic\na.go:1.1,2.1 9499 1\nb.go:1.1,2.1 501 0\n")
        self.assertLess(covered * 100, total * 95)

    def test_bad_profiles_fail(self):
        for profile in ["", "mode: set", "mode: atomic", "mode: atomic\na.go:1.1,2.1 -1 1", "mode: atomic\nmalformed"]:
            with self.subTest(profile=profile), self.assertRaises(ValueError):
                coverage_totals(profile)

    def test_cross_package_profiles_merge_the_same_source_block(self):
        self.assertEqual(coverage_totals("mode: atomic\na.go:1.1,2.1 3 0\na.go:1.1,2.1 3 2\nb.go:3.1,4.1 1 0\n"), (3, 4))

    def test_inconsistent_repeated_block_rejected(self):
        with self.assertRaises(ValueError):
            coverage_totals("mode: atomic\na.go:1.1,2.1 3 0\na.go:1.1,2.1 4 1\n")

    def test_badge_uses_real_statement_coverage(self):
        self.assertIn("94.99%", coverage_badge(9499, 10000))
        self.assertIn("#e05d44", coverage_badge(9499, 10000))
        self.assertIn("#4c1", coverage_badge(95, 100))


class ModuleTests(unittest.TestCase):
    def test_module_check_accepts_existing_uncommitted_contents(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for name in ("go.mod", "go.sum"):
                (root / name).write_text("current edited contents")
            with patch.object(quality, "ROOT", root), patch.object(quality, "run") as run, \
                    patch("sys.argv", ["quality.py", "module-check"]):
                self.assertEqual(quality.main(), 0)
                run.assert_called_once_with(["go", "mod", "tidy"])

    def test_module_check_rejects_tidy_drift(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for name in ("go.mod", "go.sum"):
                (root / name).write_text("before")
            def tidy(_):
                (root / "go.sum").write_text("after")
            with patch.object(quality, "ROOT", root), patch.object(quality, "run", side_effect=tidy), \
                    patch("sys.argv", ["quality.py", "module-check"]):
                with self.assertRaisesRegex(ValueError, "tidy changed"):
                    quality.main()
