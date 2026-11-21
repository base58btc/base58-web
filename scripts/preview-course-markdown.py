#!/usr/bin/env python3
"""Serve lesson pages exported by TestLightningCourseMarkdown for visual review.

This previews production Markdown and lesson templates; executing VM cells still
requires a published or draft runtime bundle on the application server.
"""
import argparse
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import unquote, urlsplit


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--dir", type=Path, default=Path("target/lightning-preview"))
    parser.add_argument("--port", type=int, default=8099)
    args = parser.parse_args()
    rendered = args.dir.resolve()
    static = Path(__file__).resolve().parents[1] / "static"
    if not (rendered / "courses/lightning/00-welcome").is_file():
        parser.error("Export the lesson pages first; see docs/lightning-port.md")

    class Handler(SimpleHTTPRequestHandler):
        def do_GET(self):
            path = unquote(urlsplit(self.path).path)
            if path == "/":
                self.send_response(302)
                self.send_header("Location", "/courses/lightning/00-welcome")
                self.end_headers()
                return
            if path.startswith("/static/"):
                root, relative = static, path[len("/static/"):]
            elif path.startswith("/courses/"):
                root, relative = rendered, path.lstrip("/")
            else:
                self.send_error(404)
                return
            file = (root / relative).resolve()
            if root not in file.parents or not file.is_file():
                self.send_error(404)
                return
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8" if path.startswith("/courses/") else self.guess_type(str(file)))
            self.send_header("Content-Length", str(file.stat().st_size))
            self.end_headers()
            with file.open("rb") as source:
                self.copyfile(source, self.wfile)

    print(f"Markdown rendering preview: http://127.0.0.1:{args.port}", flush=True)
    ThreadingHTTPServer(("127.0.0.1", args.port), Handler).serve_forever()


if __name__ == "__main__":
    main()
