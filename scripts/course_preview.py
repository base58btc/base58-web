"""Read the exact prepared course sources shared by the local preview servers."""
import hashlib
import json
from pathlib import Path


def load_bundle(course, port, workspace_prefix="preview-"):
    course = Path(course).resolve()
    manifest = json.loads((course / "course-runtime.json").read_text(encoding="utf-8"))
    if manifest.get("format") != 1 or manifest.get("system") != "aarch64-linux":
        raise ValueError("Unsupported course runtime manifest")
    extensions = {".md", ".nix", ".py", ".sh", ".json", ".toml", ".yaml", ".yml", ".txt", ".conf"}
    files = {}
    for file in course.rglob("*"):
        relative = file.relative_to(course)
        if not file.is_file() or file.is_symlink() or any(part.startswith(".") for part in relative.parts):
            continue
        if str(relative) == "course-runtime.json":
            continue
        if str(relative) == "flake.lock" or file.suffix.lower() in extensions:
            files[relative.as_posix()] = file.read_bytes().decode("utf-8")
    digest = hashlib.sha256("".join(
        f"{name}:{hashlib.sha256(content.encode()).hexdigest()}\n"
        for name, content in sorted(files.items())).encode()).hexdigest()
    if digest != manifest["sourceHash"]:
        raise ValueError("Course bundle is stale. Run course-sync bundle again.")
    archives = {}
    for item in manifest["archives"]:
        file = (course / item["path"]).resolve()
        if course not in file.parents or not file.is_file() or file.stat().st_size != item["size"]:
            raise ValueError("Missing or invalid course archive: " + item["path"])
        archives["/" + item["path"]] = file
    environment = {"manifest": manifest, "files": files, "workspace": workspace_prefix + digest,
                   "assetBaseURL": f"http://127.0.0.1:{port}/"}
    return environment, archives
