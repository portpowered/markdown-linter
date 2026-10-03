"""Build supported CLI archives with checksums; requires Go and Python 3."""
import hashlib
import os
from pathlib import Path
import re
import subprocess
import sys
import tarfile
import tempfile
import zipfile

version = sys.argv[1]
if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?", version):
    raise SystemExit("expected a semantic version tag")
root = Path(__file__).resolve().parent.parent
output = root / "dist"
output.mkdir(exist_ok=True)
archives = []
for target_os in ("linux", "darwin", "windows"):
    for arch in ("amd64", "arm64"):
        filename = "marklint.exe" if target_os == "windows" else "marklint"
        name = f"marklint_{version}_{target_os}_{arch}"
        with tempfile.TemporaryDirectory() as temporary:
            binary = Path(temporary) / filename
            env = dict(os.environ, GOWORK="off", CGO_ENABLED="0", GOOS=target_os, GOARCH=arch)
            subprocess.run(["go", "build", "-trimpath", "-ldflags", f"-s -w -X github.com/portpowered/markdown-linter/pkg/cli.Version={version}", "-o", str(binary), "./cmd/marklint"], cwd=root, env=env, check=True)
            files = [(binary, filename), (root / "LICENSE", "LICENSE"), (root / "NOTICE", "NOTICE")]
            files += [(path, path.relative_to(root).as_posix()) for path in sorted((root / "pkg/rulepack" / "packs").iterdir()) if path.is_file()]
            files += [(root / "docs" / "rule-packs.md", "docs/rule-packs.md"), (root / "docs" / "rule-catalog.json", "docs/rule-catalog.json"), (root / "docs" / "rule-pack.schema.json", "docs/rule-pack.schema.json"), (root / "docs" / "strunk-white.md", "docs/strunk-white.md"), (root / "docs" / "ste100.md", "docs/ste100.md"), (root / "docs" / "ste100-dictionary.schema.json", "docs/ste100-dictionary.schema.json")]
            files += [(path, path.relative_to(root).as_posix()) for path in sorted((root / "examples" / "ste100").iterdir()) if path.is_file()]
            if target_os == "windows":
                archive = output / (name + ".zip")
                with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as bundle:
                    for path, entry in files:
                        bundle.write(path, entry)
            else:
                archive = output / (name + ".tar.gz")
                with tarfile.open(archive, "w:gz") as bundle:
                    for path, entry in files:
                        bundle.add(path, arcname=entry)
            archives.append(archive)
(output / "checksums.txt").write_text("".join(f"{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.name}\n" for path in sorted(archives)))
