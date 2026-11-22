#!/usr/bin/env python3
"""Test the actual guest transport with native Python, without starting a VM."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

GUEST = Path(__file__).resolve().parents[1] / "static/guest"


class QemuProtocol(unittest.TestCase):
    def test_persistent_python_checks_shell_argv_and_private_transport(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            shutil.copyfile(GUEST / "course-python.py", root / ".base58-python.py")
            shutil.copyfile(GUEST / "course-qemu.py", root / "agent.py")
            requests = [
                {"type": "python", "code": "import os; pid = os.getpid(); value = 41"},
                {"type": "python", "code": "print(os.getpid() == pid); value + 1"},
                {"type": "python", "code": "value = 8; raise ValueError('expected')"},
                {"type": "python", "code": "value", "check": "assert value == 8\nprivate_check = 1"},
                {"type": "python", "code": "print('private_check' in globals()); os.write(1, b'bad frame\\n'); print('⚡')"},
                {"type": "exec", "program": sys.executable, "args": ["-c", "import sys; print(sys.argv[1]); sys.exit(3)", "$(touch nope); ' ⚡"]},
                {"type": "python", "code": "value + 1"},
            ]
            process = subprocess.run([sys.executable, str(root / "agent.py")], cwd=root,
                env={**os.environ, "BASE58_WORKSPACE": str(root)},
                input="".join(json.dumps({"id": i, **message}) + "\n" for i, message in enumerate(requests)),
                capture_output=True, text=True, timeout=15)
            self.assertEqual(process.returncode, 0, process.stderr)
            lines = process.stdout.splitlines()
            self.assertTrue(all(line.startswith("BASE58_QEMU:") for line in lines))
            frames = [json.loads(line.removeprefix("BASE58_QEMU:")) for line in lines]
            self.assertEqual(frames.pop(0), {"ready": 1})
            self.assertEqual([frame["id"] for frame in frames], list(range(len(requests))))
            results = [frame["result"] for frame in frames]
            self.assertEqual(results[1]["stdout"], "True\n42\n")
            self.assertEqual(results[2]["exitCode"], 1)
            self.assertIn("expected", results[2]["stdout"])
            self.assertEqual(results[3]["phase"], "check")
            self.assertEqual(results[4]["stdout"], "False\n⚡\n")
            self.assertEqual(results[5]["stdout"], "$(touch nope); ' ⚡\n")
            self.assertEqual(results[5]["exitCode"], 3)
            self.assertFalse((root / "nope").exists())
            self.assertEqual(results[6]["stdout"], "9\n")


if __name__ == "__main__":
    unittest.main()
