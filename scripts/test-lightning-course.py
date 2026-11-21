#!/usr/bin/env python3
"""Execute the actual Markdown cells, optionally including a fresh regtest lab.

Run from the repo root with the course's Nix shell. Uses temporary data and
separate local ports. It never touches an existing wallet or course checkpoint.
"""
import argparse
import importlib.util
import os
from pathlib import Path
import re
import sys
import tempfile
import time

REPO = Path(__file__).resolve().parents[1]
COURSE = REPO / "courses/lightning"
CELL = re.compile(r"^\+\+\+ ([\w-]+) python-vm\n(.*?)^\+\+\+\s*$", re.M | re.S)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--regtest", action="store_true", help="also execute Bitcoin and Lightning cells")
    args = parser.parse_args()
    spec = importlib.util.spec_from_file_location("course_python", REPO / "static/guest/course-python.py")
    kernel = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(kernel)
    os.chdir(COURSE)
    sys.path.insert(0, str(COURSE))
    with tempfile.TemporaryDirectory(prefix="base58-lightning-test-") as directory:
        os.environ.update(BASE58_LAB=directory, BASE58_BITCOIN_RPC_PORT="18643", BASE58_LIGHTNING_PORT="29731")
        namespace = {"__name__": "__course__"}
        import lab
        count = 0
        try:
            for lesson in sorted(COURSE.glob("*.md")):
                for block_id, code in CELL.findall(lesson.read_text()):
                    compile(code, f"{lesson.name}:{block_id}", "exec")
                    if not args.regtest and lesson.name.startswith(("00-", "01-", "05-")):
                        continue
                    if block_id == "revocation-vectors":
                        # The notebook exercise intentionally starts with the
                        # missing assignment. Verify that it fails, then test a
                        # minimal student correction against the same vectors.
                        attempt = kernel.run_cell(code, namespace)
                        assert not attempt["ok"] and "AssertionError" in attempt["output"], attempt
                        generator = next(source for name, source in CELL.findall(lesson.read_text()) if name == "revocation-generator")
                        correction = generator.replace("            flip(secret, bit)", "            secret = flip(secret, bit)")
                        assert correction != generator
                        fixed = kernel.run_cell(correction, namespace)
                        assert fixed["ok"], fixed
                        print("PASS revocation exercise rejects starter and accepts corrected implementation", flush=True)
                    result = kernel.run_cell(code, namespace)
                    if not result["ok"]:
                        raise AssertionError(f"{lesson.name}:{block_id}\n{result['output']}")
                    count += 1
                    print(f"PASS {lesson.name}:{block_id}", flush=True)
            # Edges cases and independent known encodings, beyond the happy-path cells.
            for invalid in ("1p", "0", "-1u", "12z"):
                try:
                    namespace["parse_amt"](invalid)
                except ValueError:
                    pass
                else:
                    raise AssertionError(f"Accepted invalid amount {invalid}")
            invoice = namespace["examples"]["2500u"]
            assert namespace["parse_invoice"](invoice.upper())["amt_msat"] == 250_000_000
            try:
                namespace["parse_invoice"](invoice[:-1] + ("q" if invoice[-1] != "q" else "p"))
            except ValueError:
                pass
            else:
                raise AssertionError("Accepted a corrupt invoice checksum")
            try:
                namespace["find_route"](namespace["gossmap"], namespace["my_source"], "missing")
            except ValueError:
                pass
            else:
                raise AssertionError("Accepted a disconnected destination")
            assert namespace["payload"](1000, 151, channel="118x1x0") == "11020203e804019706080000760000010000"
            assert namespace["bigsize"](253).hex() == "fd00fd"
            assert namespace["truncated_integer"](0, 8) == b""
            print(f"Validated {count} executable Markdown cells and protocol edge cases.", flush=True)
        finally:
            if args.regtest:
                for node in lab.NODES:
                    try:
                        lab.lightning(node, "stop")
                    except Exception:
                        pass
                try:
                    lab.bitcoin("stop", wallet=False)
                except Exception:
                    pass
                # Let daemons release their temporary files before deleting the lab.
                time.sleep(2)


if __name__ == "__main__":
    main()
