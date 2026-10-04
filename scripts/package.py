"""Build one CLI archive per supported platform, with checksums."""
import argparse
import hashlib
import os
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile
import zipfile

TARGETS = tuple((os_name, arch) for os_name in ("linux", "darwin", "windows")
                for arch in ("amd64", "arm64"))


def artifact_files(root):
    required = ["LICENSE", "NOTICE", "docs/rule-packs.md", "docs/rule-catalog.json",
                "docs/strunk-white.md", "docs/ste100.md", "docs/single-file-policy.md"]
    files = [(root / name, name) for name in required]
    for directory in ("pkg/rulepack/packs", "pkg/contract/ruledefs", "examples/ste100",
                      "runtime", "schemas", "examples/v2"):
        source = root / directory
        if not source.is_dir():
            raise ValueError(f"Missing archive directory: {directory}")
        files.extend((path, path.relative_to(root).as_posix()) for path in sorted(source.rglob("*"))
                     if path.is_file())
    for path, _ in files:
        if not path.is_file():
            raise ValueError(f"Missing archive asset: {path}")
    if not (root / "runtime/node_modules/mermaid/package.json").is_file():
        raise ValueError("Install runtime dependencies before packaging")
    return files


def build_archives(root, version, targets=TARGETS):
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?", version):
        raise ValueError("expected a semantic version tag")
    if len(set(targets)) != len(targets) or any(target not in TARGETS for target in targets):
        raise ValueError("archive targets must be supported and unique")
    files = artifact_files(root)
    output = root / "dist"
    output.mkdir(exist_ok=True)
    archives = []
    for target_os, arch in targets:
        filename = "marklint.exe" if target_os == "windows" else "marklint"
        name = f"marklint_{version}_{target_os}_{arch}"
        with tempfile.TemporaryDirectory() as temporary:
            binary = Path(temporary) / filename
            env = dict(os.environ, GOWORK="off", CGO_ENABLED="0", GOOS=target_os, GOARCH=arch)
            subprocess.run(["go", "build", "-trimpath", "-ldflags",
                            f"-s -w -X github.com/portpowered/markdown-linter/pkg/cli.Version={version}",
                            "-o", str(binary), "./cmd/marklint"], cwd=root, env=env, check=True)
            entries = [(binary, filename)] + files
            if target_os == "windows":
                archive = output / (name + ".zip")
                with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as bundle:
                    for path, entry in entries:
                        bundle.write(path, entry)
            else:
                archive = output / (name + ".tar.gz")
                with tarfile.open(archive, "w:gz") as bundle:
                    for path, entry in entries:
                        info = bundle.gettarinfo(str(path), arcname=entry)
                        if entry == filename:
                            info.mode = 0o755
                        with path.open("rb") as source:
                            bundle.addfile(info, source)
            archives.append(archive)
    (output / "checksums.txt").write_text(
        "".join(f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}\n"
                for path in sorted(archives)), encoding="utf-8")
    return archives


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version", help="semantic version tag")
    args = parser.parse_args()
    build_archives(Path(__file__).resolve().parent.parent, args.version)


if __name__ == "__main__":
    main()
