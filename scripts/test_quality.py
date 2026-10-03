import unittest
from quality import coverage_totals


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
