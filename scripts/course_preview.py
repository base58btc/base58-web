"""Read the exact prepared course sources shared by the local preview servers."""
import hashlib
import json
from pathlib import Path

MANIFESTS = {"course-runtime.json", "course-runtime-x86_64.json"}


def content_hash(files):
    return hashlib.sha256("".join(
        f"{name}:{hashlib.sha256(content.encode()).hexdigest()}\n"
        for name, content in sorted(files.items()) if name not in MANIFESTS).encode()).hexdigest()


def runtime_hash(files):
    # Keep this boundary identical to courseenv.RuntimeFiles, including Markdown
    # deliberately placed under nix/ for a derivation to consume.
    return content_hash({name: content for name, content in files.items()
                         if Path(name).suffix.lower() != ".md" or name.startswith("nix/")})


def load_bundle(course, port, workspace_prefix="preview-"):
    course = Path(course).resolve()
    manifest = json.loads((course / "course-runtime.json").read_text(encoding="utf-8"))
    if manifest.get("format") not in (1, 2) or manifest.get("system") != "aarch64-linux":
        raise ValueError("Unsupported course runtime manifest")
    extensions = {".md", ".nix", ".py", ".sh", ".json", ".toml", ".yaml", ".yml", ".txt", ".conf"}
    files = {}
    for file in course.rglob("*"):
        relative = file.relative_to(course)
        if not file.is_file() or file.is_symlink() or any(part.startswith(".") for part in relative.parts):
            continue
        if str(relative) in MANIFESTS or relative.parts[0] == "runtime":
            continue
        if str(relative) == "flake.lock" or file.suffix.lower() in extensions:
            files[relative.as_posix()] = file.read_bytes().decode("utf-8")
    if "flake.nix" not in files or "flake.lock" not in files:
        raise ValueError("Course bundle requires flake.nix and flake.lock")
    digest = content_hash(files)
    actual, expected = ((digest, manifest.get("sourceHash")) if manifest["format"] == 1
                        else (runtime_hash(files), manifest.get("runtimeHash")))
    if actual != expected:
        raise ValueError("Course bundle is stale. Run course-sync bundle again.")
    fallback_file = course / "course-runtime-x86_64.json"
    fallback = json.loads(fallback_file.read_text()) if fallback_file.exists() else None
    if fallback and (fallback.get("format") != 2 or fallback.get("system") != "x86_64-linux" or fallback.get("runtimeHash") != runtime_hash(files)):
        raise ValueError("Unsupported or stale fallback runtime")
    archives = {}
    for item in manifest["archives"] + (fallback["archives"] if fallback else []):
        file = (course / item["path"]).resolve()
        if course not in file.parents or not file.is_file() or file.stat().st_size != item["size"]:
            raise ValueError("Missing or invalid course archive: " + item["path"])
        archives["/" + item["path"]] = file
    environment = {"manifest": manifest, "files": files, "contentHash": digest, "workspace": workspace_prefix + digest,
                   "fallbackManifest": fallback,
                   "assetBaseURL": f"http://127.0.0.1:{port}/"}
    return environment, archives
