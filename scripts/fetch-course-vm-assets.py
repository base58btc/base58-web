#!/usr/bin/env python3
"""Fetch the exact browser kernel used by Try Nix; verify before extracting files."""
import hashlib
import io
from pathlib import Path
import tarfile
import urllib.request

VERSION = "0.2.0-261119-040e142"
SHA256 = "001642f481d94dfd441fc289b005939806e198b06d6ab2b27ff4e4e07d7ce161"
DEST = Path(__file__).resolve().parents[1] / "static" / "vendor" / ("unkernel-" + VERSION)

def main():
    if (DEST / ".archive-sha256").exists() and (DEST / ".archive-sha256").read_text() == SHA256:
        return
    url = f"https://registry.npmjs.org/unkernel/-/unkernel-{VERSION}.tgz"
    with urllib.request.urlopen(url) as response:
        data = response.read()
    if hashlib.sha256(data).hexdigest() != SHA256:
        raise RuntimeError("unkernel download failed its SHA-256 check")
    with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as archive:
        for source, target in {
            "dist/web.mjs": "web.mjs",
            "dist/_chunks/core.mjs": "_chunks/core.mjs",
            "dist/unkernel.wasm": "unkernel.wasm",
            "LICENSE": "LICENSE",
            "LICENSE-MIT": "LICENSE-MIT",
        }.items():
            dest = DEST / target
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_bytes(archive.extractfile("package/" + source).read())
    (DEST / ".archive-sha256").write_text(SHA256)

if __name__ == "__main__":
    main()
