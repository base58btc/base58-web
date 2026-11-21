#!/usr/bin/env python3
"""Exercise the guest Python protocol without a browser or Nix VM."""
import base64
import importlib.util
import json
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
    def test_challenges_preserve_student_definitions_and_isolate_check_names(self):
        state = {"offset": 2}
        source = "def answer():\n    return 40 + offset"
        result = kernel.run_cell(source, state, check="assert answer() == 42\nhidden_fixture = 123")
        self.assertTrue(result["ok"], result)
        self.assertEqual(result["phase"], "check")
        self.assertNotIn("hidden_fixture", state)
        self.assertEqual(kernel.run_cell("answer()", state)["output"], "42\n")
        failed = kernel.run_cell("def answer(): return 0", state, check="assert answer() == 42")
        self.assertFalse(failed["ok"])
        self.assertEqual(failed["phase"], "check")
        self.assertNotIn("course-checks", failed["output"])
        self.assertEqual(kernel.run_cell("answer()", state)["output"], "0\n")
        broken = kernel.run_cell("raise ValueError('fix me')", state, check="assert True")
        self.assertEqual(broken["phase"], "student")
        self.assertIn("fix me", broken["output"])
        timeout = kernel.run_cell("pass", state, timeout=0.05, check="while True: pass")
        self.assertFalse(timeout["ok"])
        self.assertEqual(kernel.run_cell("6 * 7", state)["output"], "42\n")

    def test_stdio_challenge_and_following_cell_share_the_namespace(self):
        messages = [{"id": 1, "code": "def answer(): return 42", "check": "assert answer() == 42"},
                    {"id": 2, "code": "answer()"},
                    {"id": 3, "code": "def answer(): return 0", "check": "assert answer() == 42"}]
        result = subprocess.run([sys.executable, str(RUNNER), "--stdio"],
                                input="".join(json.dumps(m) + "\n" for m in messages),
                                capture_output=True, text=True, timeout=10)
        frames = [json.loads(line) for line in result.stdout.splitlines()][1:]
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(frames[0]["ok"])
        self.assertEqual(frames[0]["phase"], "check")
        self.assertEqual(frames[1]["output"], "42\n")
        self.assertFalse(frames[2]["ok"])

    def test_stdio_reuses_process_and_separates_protocol_from_student_io(self):
        sources = [
            "import os; first_pid = os.getpid(); value = '⚡'",
            "print(os.getpid() == first_pid); value + value",
            "value = 7; raise ValueError('deliberate')",
            "value + 1",
            "import subprocess; subprocess.run(['echo', 'child output']); os.write(1, b'raw output\\n'); print('captured')",
            "input()",
            "value",
            "print('\\0' * 600000)",
        ]
        requests = "".join(json.dumps({"id": index, "code": source}) + "\n" for index, source in enumerate(sources))
        result = subprocess.run([sys.executable, str(RUNNER), "--stdio"], input=requests,
                                capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr)
        frames = [json.loads(line) for line in result.stdout.splitlines()]
        self.assertEqual(frames.pop(0), {"ready": 1})
        self.assertEqual([frame["id"] for frame in frames], list(range(len(sources))))
        self.assertEqual(frames[1]["output"], "True\n'⚡⚡'\n")
        self.assertFalse(frames[2]["ok"])
        self.assertEqual(frames[3]["output"], "8\n")
        self.assertEqual(frames[4]["output"], "captured\n")
        self.assertIn("child output", result.stderr)
        self.assertIn("raw output", result.stderr)
        self.assertIn("EOFError", frames[5]["output"])
        self.assertEqual(frames[6]["output"], "7\n")
        self.assertIn("output truncated", frames[7]["output"])

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
                challenge = base64.b64encode(json.dumps({"code": "def answer(): return value * 6",
                                                        "check": "assert answer() == 42"}).encode()).decode()
                checked = subprocess.run([sys.executable, str(RUNNER), "--check", challenge],
                                         env=env, cwd=directory, capture_output=True, text=True, timeout=5)
                self.assertEqual(checked.returncode, 0, checked.stderr)
                self.assertEqual(run("answer()").stdout.strip(), "42")
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
