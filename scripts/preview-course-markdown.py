#!/usr/bin/env python3
"""Serve rendered course lessons and their prepared browser runtime locally."""
import argparse
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
from urllib.parse import unquote, urlsplit
from course_preview import load_bundle


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dir", type=Path, default=Path("target/lightning-preview"))
    parser.add_argument("--course-dir", type=Path, default=Path("courses/lightning"))
    parser.add_argument("--course", default="lightning", help="course slug in the rendered URLs")
    parser.add_argument("--port", type=int, default=8099)
    args = parser.parse_args()
    rendered = args.dir.resolve()
    static = Path(__file__).resolve().parents[1] / "static"
    if not args.course or any(c not in "abcdefghijklmnopqrstuvwxyz0123456789-_" for c in args.course):
        parser.error("Invalid course slug")
    course_path = f"/courses/{args.course}"
    environment_path = f"/api/courses/{args.course}/environment"
    if not (rendered / f"courses/{args.course}/00-welcome").is_file():
        parser.error("Export the lesson pages first; see the course port documentation")
    try:
        environment, archives = load_bundle(args.course_dir, args.port, "lesson-preview-")
    except (OSError, ValueError) as error:
        parser.error(str(error))

    class Handler(SimpleHTTPRequestHandler):
        def do_GET(self):
            path = unquote(urlsplit(self.path).path)
            # Refresh sources on each lesson/environment request so re-rendered
            # Markdown can reuse the bundle without restarting the preview.
            current = environment
            if path == environment_path or path.startswith(course_path + "/"):
                try:
                    current, _ = load_bundle(args.course_dir, args.port, "lesson-preview-")
                except (OSError, ValueError) as error:
                    self.send_error(409, str(error))
                    return
            if path == environment_path:
                data = json.dumps(current).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/json")
                self.send_header("Cache-Control", "no-store")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)
                return
            if path in ("/", course_path, course_path + "/"):
                self.send_response(302)
                self.send_header("Location", course_path + "/00-welcome")
                self.end_headers()
                return
            if path in archives:
                root = args.course_dir.resolve()
                file = archives[path]
            elif path.startswith("/static/"):
                root, relative = static, path[len("/static/"):]
                file = (root / relative).resolve()
            elif path.startswith(course_path + "/"):
                root, relative = rendered, path.lstrip("/")
                file = (root / relative).resolve()
            else:
                self.send_error(404)
                return
            if root not in file.parents or not file.is_file():
                self.send_error(404)
                return
            html = None
            if path.startswith("/courses/"):
                html = file.read_text(encoding="utf-8").replace(
                    'data-local-preview="true"',
                    'data-local-preview="true" data-course-source="' + current["contentHash"] + '"'
                ).encode("utf-8")
            self.send_response(200)
            content_type = "text/html; charset=utf-8" if path.startswith("/courses/") else "text/javascript" if file.suffix == ".mjs" else self.guess_type(str(file))
            self.send_header("Content-Type", content_type)
            self.send_header("Cache-Control", "no-store")
            self.send_header("Content-Length", str(len(html) if html is not None else file.stat().st_size))
            self.end_headers()
            if html is not None:
                self.wfile.write(html)
                return
            with file.open("rb") as source:
                self.copyfile(source, self.wfile)

    print(f"Course preview: http://127.0.0.1:{args.port}{course_path}/00-welcome", flush=True)
    ThreadingHTTPServer(("127.0.0.1", args.port), Handler).serve_forever()


if __name__ == "__main__":
    main()
