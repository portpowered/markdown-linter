"""Portable local/CI quality checks. Coverage includes every Go profile entry."""
import argparse
from decimal import Decimal
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent
LINT_VERSION = "2.14.0"


def coverage_totals(profile):
    lines = profile.splitlines()
    if not lines or lines[0] != "mode: atomic":
        raise ValueError("expected an atomic Go coverage profile")
    blocks = {}
    for line in lines[1:]:
        if not line.strip():
            continue
        location, statements, count = line.rsplit(maxsplit=2)
        if ":" not in location or int(statements) < 0 or int(count) < 0:
            raise ValueError("invalid Go coverage entry")
        weight = int(statements)
        previous = blocks.get(location)
        if previous is not None and previous[0] != weight:
            raise ValueError("inconsistent statement count for the same source block")
        blocks[location] = (weight, int(count) > 0 or (previous is not None and previous[1]))
    # -coverpkg profiles repeat source blocks across test binaries. Like go tool
    # cover, merge their execution counts before counting each source block once.
    total = sum(weight for weight, _ in blocks.values())
    covered = sum(weight for weight, executed in blocks.values() if executed)
    if total == 0:
        raise ValueError("coverage profile contains no statements")
    return covered, total


def run(args):
    env = dict(os.environ, GOWORK="off")
    subprocess.run(args, cwd=ROOT, env=env, check=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("check", choices=["coverage", "coverage-check", "format", "lint", "tools-test"])
    parser.add_argument("--threshold", type=Decimal, default=Decimal("95"))
    parser.add_argument("--profile", default="coverage.out")
    parser.add_argument("--go", default=os.environ.get("GO", "go"))
    parser.add_argument("--lint", default=os.environ.get("GOLANGCI_LINT", "golangci-lint"))
    parser.add_argument("--timeout", default="120s")
    args = parser.parse_args()
    if not Decimal("0") <= args.threshold <= Decimal("100"):
        parser.error("threshold must be between 0 and 100")
    if args.check == "coverage":
        run([args.go, "test", "-race", "-covermode=atomic", "-coverpkg=./...",
             "-coverprofile=" + args.profile, "-timeout", args.timeout, "./..."])
    if args.check in ["coverage", "coverage-check"]:
        covered, total = coverage_totals((ROOT / args.profile).read_text(encoding="utf-8"))
        percent = Decimal(covered) * 100 / total
        print(f"Coverage: {percent:.2f}% ({covered}/{total} statements); required {args.threshold}%")
        return 0 if percent >= args.threshold else 1
    if args.check == "format":
        files = [str(p.relative_to(ROOT)) for p in ROOT.rglob("*.go") if ".git" not in p.parts]
        unformatted = subprocess.check_output(["gofmt", "-l"] + files, cwd=ROOT, text=True)
        if unformatted:
            print(unformatted, end="")
            return 1
    elif args.check == "lint":
        version = subprocess.check_output([args.lint, "version"], cwd=ROOT, text=True)
        if "version " + LINT_VERSION + " " not in version:
            raise ValueError(f"golangci-lint v{LINT_VERSION} required; found {version.strip()}")
        run([args.lint, "run", "--allow-parallel-runners", "--config", ".golangci.yml", "./..."])
    elif args.check == "tools-test":
        run([sys.executable, "-m", "unittest", "discover", "-s", "scripts", "-p", "test_*.py"])
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        print(error, file=sys.stderr)
        sys.exit(2)
