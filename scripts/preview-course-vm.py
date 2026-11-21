#!/usr/bin/env python3
"""Preview a prepared course bundle locally without a database or asset uploads."""
import argparse
import hashlib
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
from urllib.parse import unquote, urlsplit

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--dir", required=True, type=Path)
    parser.add_argument("--port", type=int, default=8097)
    args = parser.parse_args()
    course = args.dir.resolve()
    static = Path(__file__).resolve().parents[1] / "static"
    manifest = json.loads((course / "course-runtime.json").read_text())
    extensions = {".md", ".nix", ".py", ".sh", ".json", ".toml", ".yaml", ".yml", ".txt", ".conf"}
    files = {}
    for file in course.rglob("*"):
        relative = file.relative_to(course)
        if not file.is_file() or file.is_symlink() or any(part.startswith(".") for part in relative.parts):
            continue
        if str(relative) == "course-runtime.json":
            continue
        if str(relative) == "flake.lock" or file.suffix.lower() in extensions:
            files[relative.as_posix()] = file.read_text()
    digest = hashlib.sha256("".join(f"{name}:{hashlib.sha256(content.encode()).hexdigest()}\n" for name, content in sorted(files.items())).encode()).hexdigest()
    if digest != manifest["sourceHash"]:
        raise SystemExit("Course bundle is stale. Run course-sync bundle again.")
    archives = {"/" + item["path"] for item in manifest["archives"]}

    class Handler(SimpleHTTPRequestHandler):
        def do_GET(self):
            request_path = unquote(urlsplit(self.path).path)
            if request_path == "/environment.json":
                data = json.dumps({"manifest": manifest, "files": files, "workspace": "preview-" + digest,
                                   "assetBaseURL": f"http://127.0.0.1:{args.port}/"}).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Cache-Control", "no-store")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)
                return
            if request_path == "/":
                file = static / "course-vm-preview.html"
            elif request_path.startswith("/static/"):
                file = (static / request_path[len("/static/"):]).resolve()
                if static not in file.parents:
                    self.send_error(404)
                    return
            elif request_path in archives:
                file = (course / request_path.lstrip("/")).resolve()
                if course not in file.parents:
                    self.send_error(404)
                    return
            else:
                self.send_error(404)
                return
            if not file.is_file():
                self.send_error(404)
                return
            self.send_response(200)
            content_type = "text/javascript" if file.suffix == ".mjs" else self.guess_type(str(file))
            self.send_header("Content-Type", content_type)
            self.send_header("Content-Length", str(file.stat().st_size))
            self.end_headers()
            with file.open("rb") as source:
                self.copyfile(source, self.wfile)

    print(f"Course environment preview: http://127.0.0.1:{args.port}", flush=True)
    ThreadingHTTPServer(("127.0.0.1", args.port), Handler).serve_forever()

if __name__ == "__main__":
    main()
