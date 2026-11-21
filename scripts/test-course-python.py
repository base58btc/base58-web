#!/usr/bin/env python3
"""Exercise the guest Python protocol without a browser or Nix VM."""
import base64
import importlib.util
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest

RUNNER = Path(__file__).resolve().parents[1] / "static/guest/course-python.py"
spec = importlib.util.spec_from_file_location("course_python", RUNNER)
kernel = importlib.util.module_from_spec(spec)
spec.loader.exec_module(kernel)


class PythonCells(unittest.TestCase):
    def test_persistence_errors_unicode_and_files(self):
        with tempfile.TemporaryDirectory(prefix="b58py-") as directory:
            env = {**os.environ, "BASE58_PYTHON_SOCKET": directory + "/python.sock"}
            server = subprocess.Popen([sys.executable, str(RUNNER), "--serve"], env=env, cwd=directory)
            try:
                for _ in range(100):
                    if Path(env["BASE58_PYTHON_SOCKET"]).exists():
                        break
                    time.sleep(0.02)

                def run(source):
                    return subprocess.run([sys.executable, str(RUNNER), "--execute", base64.b64encode(source.encode()).decode()],
                                          env=env, cwd=directory, capture_output=True, text=True, timeout=5)

                self.assertEqual(run("value = '⚡'; from pathlib import Path; Path('saved').write_text(value)").returncode, 0)
                self.assertEqual(run("value + Path('saved').read_text()").stdout.strip(), "'⚡⚡'")
                self.assertEqual(run("value = 7; raise ValueError('deliberate')").returncode, 1)
                self.assertEqual(run("value").stdout.strip(), "7")
                self.assertEqual(run("raise SystemExit(0)").returncode, 1)
                self.assertEqual(run("value + 1").stdout.strip(), "8")
                # Control characters expand sixfold in JSON but must not break the protocol.
                escaped = run("print('\\0' * 600000)")
                self.assertEqual(escaped.returncode, 0, escaped.stderr)
                self.assertIn("output truncated", escaped.stdout)
            finally:
                server.terminate()
                server.wait(timeout=5)

    def test_timeout_and_output_limit_leave_session_usable(self):
        state = {}
        self.assertFalse(kernel.run_cell("while True: pass", state, timeout=0.05)["ok"])
        self.assertEqual(kernel.run_cell("6 * 7", state)["output"], "42\n")
        result = kernel.run_cell("print('x' * 1000000)", state)
        self.assertIn("output truncated", result["output"])
        self.assertLess(len(result["output"]), 600000)


if __name__ == "__main__":
    unittest.main()
