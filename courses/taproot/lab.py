"""Bitcoin CLI calls for the workshop's isolated regtest wallet."""
import json
import os
from pathlib import Path
import subprocess
import time

ROOT = Path(os.environ.get("BASE58_LAB", str(Path.cwd() / ".lab"))).resolve()
RPC_PORT = int(os.environ.get("BASE58_BITCOIN_RPC_PORT", "18443"))


def bitcoin(method, *args, wallet=True):
    argv = ["bitcoin-cli", "-regtest", f"-datadir={ROOT / 'bitcoin'}"]
    if wallet:
        argv.append("-rpcwallet=base58")
    argv += [method] + [arg if isinstance(arg, str) else json.dumps(arg) for arg in args]
    result = subprocess.run(argv, capture_output=True, text=True, timeout=45)
    if result.returncode:
        raise RuntimeError(result.stderr.strip() or result.stdout.strip())
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError:
        return result.stdout.strip()


def start_bitcoin():
    directory = ROOT / "bitcoin"
    directory.mkdir(parents=True, exist_ok=True)
    try:
        bitcoin("getblockchaininfo", wallet=False)
    except RuntimeError:
        (directory / "bitcoin.conf").write_text(
            "regtest=1\nserver=1\nlisten=0\ndnsseed=0\ndiscover=0\n"
            "txindex=1\nfallbackfee=0.00001\ndbcache=32\nmaxmempool=16\npar=1\n"
            f"[regtest]\nrpcbind=127.0.0.1\nrpcallowip=127.0.0.1\nrpcport={RPC_PORT}\n")
        with (directory / "startup.log").open("ab") as log:
            process = subprocess.Popen(["bitcoind", f"-datadir={directory}"],
                                       stdin=subprocess.DEVNULL, stdout=log, stderr=log,
                                       start_new_session=True)
        deadline = time.monotonic() + 40
        while True:
            try:
                bitcoin("getblockchaininfo", wallet=False)
                break
            except RuntimeError:
                if process.poll() is not None or time.monotonic() >= deadline:
                    raise RuntimeError(f"Bitcoin startup failed; see {directory / 'startup.log'}")
                time.sleep(0.2)
    if "base58" not in bitcoin("listwallets", wallet=False):
        wallets = bitcoin("listwalletdir", wallet=False)["wallets"]
        method = "loadwallet" if any(w["name"] == "base58" for w in wallets) else "createwallet"
        bitcoin(method, "base58", wallet=False)
    height = bitcoin("getblockcount")
    if height < 101:
        mine(101 - height)
    return {"chain": "regtest", "height": bitcoin("getblockcount"), "balance": bitcoin("getbalance")}


def mine(blocks=1):
    if not isinstance(blocks, int) or not 1 <= blocks <= 1000:
        raise ValueError("Mine between 1 and 1000 blocks")
    return bitcoin("generatetoaddress", blocks, bitcoin("getnewaddress"))


def find_output(txid, scriptpubkey):
    outputs = bitcoin("getrawtransaction", txid, True)["vout"]
    return next(out["n"] for out in outputs if out["scriptPubKey"]["hex"] == scriptpubkey)
