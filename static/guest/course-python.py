"""Persistent Python cells for a live course session. Resume reconstructs them by replay.

The CLI connects to the same interpreter for each cell during a session.
Only the guest can access this Unix socket. This is not a host execution service.
"""
import ast
import base64
import contextlib
import io
import json
import os
from pathlib import Path
import signal
import socket
import subprocess
import sys
import time
import traceback

SOCKET = os.environ.get("BASE58_PYTHON_SOCKET", "/tmp/base58-python.sock")
LIMIT = 1 << 19


class Output(io.TextIOBase):
    def __init__(self):
        self.parts, self.size, self.truncated = [], 0, False

    def write(self, value):
        encoded = value.encode("utf-8", errors="replace")
        available = max(0, LIMIT - self.size)
        if available:
            self.parts.append(encoded[:available])
        self.size += min(len(encoded), available)
        self.truncated |= len(encoded) > available
        return len(value)

    def getvalue(self):
        return b"".join(self.parts).decode("utf-8", errors="replace") + (
            "\n[output truncated]\n" if self.truncated else "")


def run_cell(source, namespace, timeout=90):
    output = Output()

    def timed_out(*_):
        raise TimeoutError("Python cell exceeded its time limit")

    previous = signal.signal(signal.SIGALRM, timed_out)
    signal.setitimer(signal.ITIMER_REAL, timeout)
    ok = True
    try:
        with contextlib.redirect_stdout(output), contextlib.redirect_stderr(output):
            try:
                tree = ast.parse(source, filename="<course-cell>", mode="exec")
                expression = tree.body.pop() if tree.body and isinstance(tree.body[-1], ast.Expr) else None
                exec(compile(tree, "<course-cell>", "exec"), namespace)
                if expression:
                    value = eval(compile(ast.Expression(expression.value), "<course-cell>", "eval"), namespace)
                    namespace["_"] = value
                    if value is not None:
                        print(repr(value))
            except BaseException:
                ok = False
                traceback.print_exc()
    finally:
        signal.setitimer(signal.ITIMER_REAL, 0)
        signal.signal(signal.SIGALRM, previous)
    return {"ok": ok, "output": output.getvalue()}


def serve():
    sys.path.insert(0, os.getcwd())
    namespace = {"__name__": "__course__", "__builtins__": __builtins__}
    with socket.socket(socket.AF_UNIX) as server:
        server.bind(SOCKET)
        os.chmod(SOCKET, 0o600)
        server.listen(1)
        while True:
            connection, _ = server.accept()
            with connection:
                connection.settimeout(110)
                try:
                    with connection.makefile("rb") as reader:
                        request = reader.readline(131073)
                    if len(request) > 131072:
                        raise ValueError("Python request is too large")
                    source = json.loads(request)["code"]
                    if not isinstance(source, str) or len(source.encode()) > 16384:
                        raise ValueError("Python cells must be text under 16 KiB")
                    result = run_cell(source, namespace)
                except Exception as error:
                    result = {"ok": False, "output": str(error)}
                try:
                    connection.sendall(json.dumps(result).encode() + b"\n")
                except OSError:
                    pass  # Client disappeared; completed cell state remains in the VM.


def execute(encoded):
    source = base64.b64decode(encoded, validate=True).decode("utf-8")
    if not Path(SOCKET).exists():
        with open(SOCKET + ".log", "ab") as log:
            subprocess.Popen([sys.executable, str(Path(__file__).resolve()), "--serve"],
                             stdin=subprocess.DEVNULL, stdout=log, stderr=log,
                             start_new_session=True)
        for _ in range(100):
            if Path(SOCKET).exists():
                break
            time.sleep(0.05)
    with socket.socket(socket.AF_UNIX) as client:
        client.settimeout(110)
        try:
            client.connect(SOCKET)
        except OSError as error:
            raise RuntimeError("Python session is unavailable. Undo the last cell or reopen the environment to replay its history; "
                               "the interpreter has not been silently restarted.") from error
        client.sendall(json.dumps({"code": source}).encode() + b"\n")
        with client.makefile("rb") as reader:
            # JSON can expand control characters to six-byte escapes.
            result = json.loads(reader.readline(8 * LIMIT))
    print(result["output"], end="")
    return 0 if result["ok"] else 1


if __name__ == "__main__":
    if len(sys.argv) == 2 and sys.argv[1] == "--serve":
        serve()
    elif len(sys.argv) == 3 and sys.argv[1] == "--execute":
        sys.exit(execute(sys.argv[2]))
    else:
        sys.exit("Expected --serve or --execute BASE64_SOURCE")
