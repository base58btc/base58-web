#!/usr/bin/env python3
"""Exercise the preview's runtime/content boundary without Nix or a VM."""
import json
from pathlib import Path
import tempfile
import unittest

from course_preview import content_hash, load_bundle, runtime_hash


class BundleTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.files = {"flake.nix": "{}", "flake.lock": "{}", "lab.py": "pass\n",
                      "01_intro.md": "# Lesson\r\n", "lessons/02.MD": "# Next\n",
                      "nix/notes.md": "build input", "data/é.json": "[1]"}
        for name, content in self.files.items():
            self.write(name, content)
        self.prepare(2)

    def write(self, name, content):
        file = self.root / name
        file.parent.mkdir(parents=True, exist_ok=True)
        file.write_bytes(content.encode())

    def prepare(self, version):
        digest = content_hash(self.files) if version == 1 else runtime_hash(self.files)
        archive = f"runtime/{digest}/example.nar.gz"
        self.write(archive, "archive")
        self.manifest = {"format": version, "system": "aarch64-linux",
                         "sourceHash" if version == 1 else "runtimeHash": digest,
                         "archives": [{"path": archive, "size": 7}]}
        self.write("course-runtime.json", json.dumps(self.manifest))

    def test_lesson_edits_reuse_archive_and_change_workspace(self):
        before, archives = load_bundle(self.root, 8099)
        self.write("01_intro.md", "# Updated exercise\n")
        self.write("new/chapter.md", "# New chapter\n")
        (self.root / "lessons/02.MD").unlink()
        after, updated = load_bundle(self.root, 8099)
        self.assertEqual(archives, updated)
        self.assertEqual(before["manifest"], after["manifest"])
        self.assertNotEqual(before["contentHash"], after["contentHash"])
        self.assertNotEqual(before["workspace"], after["workspace"])
        self.assertEqual(after["files"]["01_intro.md"], "# Updated exercise\n")

    def test_fallback_keeps_the_same_workspace_and_checks_its_fingerprint(self):
        before, _ = load_bundle(self.root, 8099)
        fallback = {**self.manifest, "system": "x86_64-linux"}
        self.write("course-runtime-x86_64.json", json.dumps(fallback))
        after, _ = load_bundle(self.root, 8099)
        self.assertEqual(before["workspace"], after["workspace"])
        self.assertEqual(after["fallbackManifest"], fallback)
        self.assertNotIn("course-runtime-x86_64.json", after["files"])
        fallback["runtimeHash"] = "0" * 64
        self.write("course-runtime-x86_64.json", json.dumps(fallback))
        with self.assertRaises(ValueError):
            load_bundle(self.root, 8099)

    def test_runtime_changes_additions_and_deletions_are_stale(self):
        for name in ("flake.nix", "flake.lock", "lab.py", "nix/notes.md", "data/é.json", "new.conf"):
            with self.subTest(name=name):
                self.write(name, "changed")
                with self.assertRaises(ValueError):
                    load_bundle(self.root, 8099)
                (self.root / name).unlink()
                if name in self.files:
                    with self.assertRaises(ValueError):
                        load_bundle(self.root, 8099)
                    self.write(name, self.files[name])

    def test_legacy_upgrade_preserves_workspace(self):
        current, _ = load_bundle(self.root, 8099)
        self.prepare(1)
        legacy, _ = load_bundle(self.root, 8099)
        self.assertEqual(current["workspace"], legacy["workspace"])
        self.write("01_intro.md", "# Changed\n")
        with self.assertRaises(ValueError):
            load_bundle(self.root, 8099)

    def test_archive_size_check_remains_required(self):
        self.write(self.manifest["archives"][0]["path"], "short")
        with self.assertRaisesRegex(ValueError, "archive"):
            load_bundle(self.root, 8099)


if __name__ == "__main__":
    unittest.main()
