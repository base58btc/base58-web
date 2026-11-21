"""Persistent Python cells for a live course session. Resume reconstructs them by replay.

The browser keeps one process open and sends cells over its input/output pipes.
The legacy Unix-socket CLI is retained for standalone command compatibility.
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


def run_cell(source, namespace, timeout=90, check=None):
    output = Output()

    def timed_out(*_):
        raise TimeoutError("Python cell exceeded its time limit")

    previous = signal.signal(signal.SIGALRM, timed_out)
    signal.setitimer(signal.ITIMER_REAL, timeout)
    ok = True
    phase = "student"
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
                if check is not None:
                    phase = "check"
                    # Student definitions stay available to later cells. Test
                    # names stay out of the notebook namespace.
                    exec(compile(check, "<course-checks>", "exec"), dict(namespace))
            except BaseException:
                ok = False
                if check is None or phase == "student":
                    traceback.print_exc()
    finally:
        signal.setitimer(signal.ITIMER_REAL, 0)
        signal.signal(signal.SIGALRM, previous)
    result = {"ok": ok, "output": output.getvalue()}
    if check is not None:
        result["phase"] = phase
    return result


def run_request(message, namespace):
    source, check = message["code"], message.get("check")
    if not isinstance(source, str) or len(source.encode()) > 16384:
        raise ValueError("Python cells must be text under 16 KiB")
    if "check" in message and (not isinstance(check, str) or len(check.encode()) > 16384):
        raise ValueError("Python checks must be text under 16 KiB")
    return run_cell(source, namespace, check=check)


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
                    result = run_request(json.loads(request), namespace)
                except Exception as error:
                    result = {"ok": False, "output": str(error)}
                try:
                    connection.sendall(json.dumps(result).encode() + b"\n")
                except OSError:
                    pass  # Client disappeared; completed cell state remains in the VM.


def serve_stdio():
    # Keep the protocol on private descriptors. Student input() sees EOF, and
    # subprocesses/os.write(1, ...) cannot corrupt the JSON response stream.
    with os.fdopen(os.dup(0), "rb") as reader, os.fdopen(os.dup(1), "w") as writer:
        with open(os.devnull, "rb") as empty:
            os.dup2(empty.fileno(), 0)
        os.dup2(2, 1)
        sys.path.insert(0, os.getcwd())
        namespace = {"__name__": "__course__", "__builtins__": __builtins__}
        writer.write('{"ready":1}\n')
        writer.flush()
        while request := reader.readline(131073):
            if len(request) > 131072 or not request.endswith(b"\n"):
                raise ValueError("Invalid Python request frame")
            message = json.loads(request)
            result = run_request(message, namespace)
            writer.write(json.dumps({"id": message["id"], **result}) + "\n")
            writer.flush()


def execute(encoded, challenge=False):
    source = base64.b64decode(encoded, validate=True).decode("utf-8")
    message = json.loads(source) if challenge else {"code": source}
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
        client.sendall(json.dumps(message).encode() + b"\n")
        with client.makefile("rb") as reader:
            # JSON can expand control characters to six-byte escapes.
            result = json.loads(reader.readline(8 * LIMIT))
    print(result["output"], end="")
    return 0 if result["ok"] else 1


if __name__ == "__main__":
    if len(sys.argv) == 2 and sys.argv[1] == "--stdio":
        serve_stdio()
    elif len(sys.argv) == 2 and sys.argv[1] == "--serve":
        serve()
    elif len(sys.argv) == 3 and sys.argv[1] in ("--execute", "--check"):
        sys.exit(execute(sys.argv[2], challenge=sys.argv[1] == "--check"))
    else:
        sys.exit("Expected --stdio, --serve, --execute BASE64_SOURCE or --check BASE64_REQUEST")
