#!/usr/bin/env python3
"""Execute every Taproot lesson cell against a fresh, isolated regtest wallet."""
import importlib.util
import json
import os
from pathlib import Path
import re
import socket
import sys
import tempfile
import time

REPO = Path(__file__).resolve().parents[1]
COURSE = REPO / "courses/taproot"
BLOCK = re.compile(r"^\+\+\+ ([\w-]+) python-vm\n(.*?)^\+\+\+\s*$|^\?\?\?\n(.*?)^\?\?\?\s*$", re.M | re.S)


def main():
    spec = importlib.util.spec_from_file_location("course_python", REPO / "static/guest/course-python.py")
    kernel = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(kernel)
    os.chdir(COURSE)
    sys.path.insert(0, str(COURSE))
    with socket.socket() as port:
        port.bind(("127.0.0.1", 0))
        rpc_port = port.getsockname()[1]
    with tempfile.TemporaryDirectory(prefix="base58-taproot-test-") as directory:
        os.environ.update(BASE58_LAB=directory, BASE58_BITCOIN_RPC_PORT=str(rpc_port))
        import lab
        from codes import size_compact_size, parse_compact_size
        import coincurve
        namespace = {"__name__": "__course__"}
        solutions = json.loads((REPO / "scripts/testdata/taproot-challenge-solutions.json").read_text())
        count, challenges, spends = 0, 0, 0
        try:
            for lesson in sorted(COURSE.glob("*.md")):
                for match in BLOCK.finditer(lesson.read_text()):
                    block_id, code, challenge = match.groups()
                    check = None
                    if challenge is not None:
                        prompt, code, check, *_ = re.split(r"^---\s*$", challenge, flags=re.M)
                        block_id = re.search(r"^id: ([\w-]+)$", prompt, re.M)[1]
                        assert "runtime: python-vm" in prompt
                        result = kernel.run_cell(code, namespace, check=check)
                        assert not result["ok"] and result["phase"] == "check", (block_id, result)
                        code = solutions[block_id]
                        challenges += 1
                        print(f"PASS {block_id} rejects the unfinished starter", flush=True)
                    if block_id == "taproot-w041":
                        # Exploring a different address in chapter 1 must not
                        # replace the key used for the historical signature.
                        namespace["x_val"] = coincurve.PrivateKey.from_int(17).public_key.point()[0]
                    result = kernel.run_cell(code, namespace, check=check)
                    assert result["ok"], f"{lesson.name}:{block_id}\n{result['output']}"
                    count += 1
                    if block_id == "taproot-w045":
                        assert namespace["_"] is True, "Historical Schnorr signature did not verify"
                        assert coincurve.PublicKeyXOnly(namespace["x_val"].to_bytes(32, "big")).verify(namespace["sig"], namespace["m"])
                    if block_id in {"taproot-w105", "taproot-w141", "taproot-w150", "taproot-w183", "taproot-m043", "taproot-m053"}:
                        assert namespace["acceptance"][0]["allowed"]
                        spends += 1
                    if block_id == "taproot-w141":
                        assert len(namespace["control_block"]) == 33 + 3 * 32
                    if block_id == "taproot-w150":
                        assert len(namespace["control_block"]) == 33 + 7 * 32
                    if block_id == "taproot-m053":
                        # A changed signature must fail before the valid spend is broadcast.
                        sig = namespace["sig"]
                        altered = bytes([sig[0] ^ 1]) + sig[1:]
                        bad_tx = namespace["signed_tx"].replace(sig.hex(), altered.hex())
                        assert bad_tx != namespace["signed_tx"]
                        assert not lab.bitcoin("testmempoolaccept", [bad_tx])[0]["allowed"]
                    print(f"PASS {lesson.name}:{block_id}", flush=True)
            for number in (0, 252, 253, 65535, 65536, 4294967295, 4294967296):
                encoded = size_compact_size(number)
                assert parse_compact_size(encoded) == (number, len(encoded))
            assert challenges == 3 and spends == 6, (challenges, spends)
            assert lab.bitcoin("getrawtransaction", namespace["frost_spend_txid"], True)["confirmations"] >= 1
            print(f"Passed {count} executable blocks, {challenges} challenges, and {spends} valid Taproot spends.", flush=True)
        finally:
            try:
                lab.bitcoin("stop", wallet=False)
                # Wait for this test's process to release its temporary data.
                pid = Path(directory) / "bitcoin/regtest/bitcoind.pid"
                deadline = time.monotonic() + 10
                while pid.exists() and time.monotonic() < deadline:
                    time.sleep(0.1)
            except (RuntimeError, OSError):
                pass


if __name__ == "__main__":
    main()
