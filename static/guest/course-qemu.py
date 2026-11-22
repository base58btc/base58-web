"""Private serial transport for the QEMU course guest; no host or network access.

Uses the same cell evaluator and persistent namespace as the unkernel runner.
Only commands and their results are saved by the browser, never this VM's RAM.
"""
import json
import os
from pathlib import Path
import runpy
import signal
import subprocess
import sys
import tempfile

LIMIT = 1 << 20
PREFIX = "BASE58_QEMU:"
WORKSPACE = os.environ.get("BASE58_WORKSPACE", "/data/workspace")


def run_command(message):
    program, args = message["program"], message["args"]
    if not isinstance(program, str) or not program or not isinstance(args, list) or any(not isinstance(arg, str) for arg in args):
        raise ValueError("Invalid course command")
    timeout = min(120, max(1, message.get("timeoutMs", 30000) / 1000))
    # File-backed capture keeps unbounded subprocess output out of Python's RAM.
    with tempfile.TemporaryFile() as stdout, tempfile.TemporaryFile() as stderr:
        process = subprocess.Popen([program, *args], cwd=WORKSPACE, stdin=subprocess.DEVNULL,
                                   stdout=stdout, stderr=stderr, start_new_session=True)
        timed_out = False
        try:
            process.wait(timeout=timeout)
        except subprocess.TimeoutExpired:
            timed_out = True
            os.killpg(process.pid, signal.SIGKILL)
            process.wait()
        def read(file):
            file.seek(0)
            data = file.read(LIMIT + 1)
            return data[:LIMIT].decode("utf-8", errors="replace") + ("\n[output truncated]\n" if len(data) > LIMIT else "")
        return {"stdout": read(stdout), "stderr": read(stderr), "exitCode": process.returncode, "timedOut": timed_out}


def serve():
    runner = runpy.run_path(str(Path(__file__).with_name(".base58-python.py")))
    sys.path.insert(0, os.getcwd())
    namespace = {"__name__": "__course__", "__builtins__": __builtins__}
    with os.fdopen(os.dup(0), "rb") as reader, os.fdopen(os.dup(1), "w", buffering=1) as writer:
        with open(os.devnull, "r+b") as empty:
            for fd in (0, 1, 2):
                os.dup2(empty.fileno(), fd)
        writer.write(PREFIX + '{"ready":1}\n')
        while request := reader.readline(131073):
            if len(request) > 131072 or not request.endswith(b"\n"):
                raise ValueError("Invalid QEMU request frame")
            message = json.loads(request)
            try:
                if message.get("type") == "python":
                    result = runner["run_request"](message, namespace)
                    result = {"stdout": result["output"], "stderr": "", "exitCode": 0 if result["ok"] else 1,
                              "timedOut": False, **({"phase": result["phase"]} if "phase" in result else {})}
                elif message.get("type") == "exec":
                    result = run_command(message)
                else:
                    raise ValueError("Unknown QEMU command")
            except Exception as error:
                result = {"stdout": "", "stderr": str(error), "exitCode": 1, "timedOut": False}
            writer.write(PREFIX + json.dumps({"id": message["id"], "result": result}, ensure_ascii=True) + "\n")


if __name__ == "__main__":
    serve()
