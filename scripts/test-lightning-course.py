#!/usr/bin/env python3
"""Execute the actual Markdown cells, optionally including a fresh regtest lab.

Run from the repo root with the course's Nix shell. Uses temporary data and
separate local ports. It never touches an existing wallet or course checkpoint.
"""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import sys
import tempfile
import time

REPO = Path(__file__).resolve().parents[1]
COURSE = REPO / "courses/lightning"
CELL = re.compile(r"^\+\+\+ ([\w-]+) python-vm\n(.*?)^\+\+\+\s*$", re.M | re.S)
BLOCK = re.compile(r"^\+\+\+ ([\w-]+) python-vm\n(.*?)^\+\+\+\s*$|^\?\?\?\n(.*?)^\?\?\?\s*$", re.M | re.S)


def check_full_gossip(gossmap):
    # Fingerprint of every node/channel announcement and directional update,
    # computed with the original unpatched pyln against the pinned LFS object.
    assert (len(gossmap.nodes), len(gossmap.channels)) == (14137, 62309)
    digest = hashlib.sha256()
    for node in sorted(gossmap.nodes.values(), key=str):
        digest.update(json.dumps([str(node), node.fields], sort_keys=True, default=lambda value: value.hex()).encode())
    for channel in sorted(gossmap.channels.values(), key=str):
        fields = [str(channel), channel.fields, channel.satoshis,
                  [hc.fields if hc else None for hc in channel.half_channels]]
        digest.update(json.dumps(fields, sort_keys=True, default=lambda value: value.hex()).encode())
    assert digest.hexdigest() == "336d7c628dd0303860ed267e3aba4506e5348971dc639a2c044411b4244de1e6"


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
        challenges = 0
        solutions = json.loads((REPO / "scripts/testdata/lightning-challenge-solutions.json").read_text())
        try:
            for lesson in sorted(COURSE.glob("*.md")):
                for match in BLOCK.finditer(lesson.read_text()):
                    block_id, code, challenge = match.groups()
                    check = None
                    if challenge is not None:
                        prompt, code, check, *_ = re.split(r"^---\s*$", challenge, flags=re.M)
                        block_id = re.search(r"^id: ([\w-]+)$", prompt, re.M)[1]
                        assert "runtime: python-vm" in prompt
                    compile(code, f"{lesson.name}:{block_id}", "exec")
                    if not args.regtest and lesson.name.startswith(("00-", "01-", "05-")):
                        continue
                    if check is not None:
                        attempt = kernel.run_cell(code, namespace, check=check)
                        assert not attempt["ok"] and attempt["phase"] == "check", attempt
                        code = solutions[block_id]
                        challenges += 1
                        print(f"PASS {block_id} rejects the unfinished starter", flush=True)
                    result = kernel.run_cell(code, namespace, check=check)
                    if not result["ok"]:
                        raise AssertionError(f"{lesson.name}:{block_id}\n{result['output']}")
                    count += 1
                    print(f"PASS {lesson.name}:{block_id}", flush=True)
            # Edges cases and independent known encodings, beyond the happy-path cells.
            check_full_gossip(namespace["gossmap"])
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
            print(f"Validated {count} executable Markdown blocks, including {challenges} code challenges, and protocol edge cases.", flush=True)
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
