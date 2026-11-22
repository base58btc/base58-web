#!/usr/bin/env python3
"""Preview a prepared course bundle locally without a database or asset uploads."""
import argparse
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
from urllib.parse import unquote, urlsplit
from course_preview import load_bundle

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--dir", required=True, type=Path)
    parser.add_argument("--port", type=int, default=8097)
    args = parser.parse_args()
    course = args.dir.resolve()
    static = Path(__file__).resolve().parents[1] / "static"
    try:
        environment, archives = load_bundle(course, args.port)
    except (OSError, ValueError) as error:
        parser.error(str(error))

    class Handler(SimpleHTTPRequestHandler):
        def end_headers(self):
            self.send_header("Cross-Origin-Opener-Policy", "same-origin")
            self.send_header("Cross-Origin-Embedder-Policy", "require-corp")
            super().end_headers()

        def do_GET(self):
            request_path = unquote(urlsplit(self.path).path)
            if request_path == "/environment.json":
                try:
                    current, _ = load_bundle(course, args.port)
                except (OSError, ValueError) as error:
                    self.send_error(409, str(error))
                    return
                data = json.dumps(current).encode()
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
                file = archives[request_path]
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
