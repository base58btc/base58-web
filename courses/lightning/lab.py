"""Local regtest helpers. No IPython, HTTP proxy, credentials, or external peers.

RPCs use the installed CLIs with argv arrays; student values never become shell
programs. Services, wallets, and Python globals all live in the browser VM.
"""
import json
import os
from pathlib import Path
import shutil
import subprocess
import time

ROOT = Path(os.environ.get("BASE58_LAB", str(Path.cwd() / ".lab"))).resolve()
NODES = ("alice", "bob", "carol")
RPC_PORT = int(os.environ.get("BASE58_BITCOIN_RPC_PORT", "18443"))
PEER_PORT = int(os.environ.get("BASE58_LIGHTNING_PORT", "19731"))


def cli_json(argv, timeout=45):
    result = subprocess.run(argv, capture_output=True, text=True, timeout=timeout)
    if result.returncode:
        raise RuntimeError(result.stderr.strip() or result.stdout.strip())
    try:
        return json.loads(result.stdout)
    except json.JSONDecodeError:
        return result.stdout.strip()


def bitcoin(method, *args, wallet=True):
    command = ["bitcoin-cli", "-regtest", f"-datadir={ROOT / 'bitcoin'}"]
    if wallet:
        command.append("-rpcwallet=base58")
    return cli_json(command + [method] + [json.dumps(arg) if not isinstance(arg, str) else arg for arg in args])


def lightning(node, method, **params):
    if node not in NODES:
        raise ValueError(f"Choose a node from {NODES}")
    return cli_json(["lightning-cli", f"--lightning-dir={ROOT / node}", "--network=regtest", "-k", method]
                    + [f"{key}={json.dumps(value)}" for key, value in params.items()])


def wait_for(check, description, timeout=40):
    deadline, last = time.monotonic() + timeout, None
    while time.monotonic() < deadline:
        try:
            result = check()
            if result:
                return result
        except (RuntimeError, OSError) as error:
            last = error
        time.sleep(0.2)
    raise TimeoutError(f"Timed out waiting for {description}. Last error: {last}. Logs: {ROOT}")


def start_bitcoin():
    datadir = ROOT / "bitcoin"
    datadir.mkdir(parents=True, exist_ok=True)
    try:
        bitcoin("getblockchaininfo", wallet=False)
    except RuntimeError:
        (datadir / "bitcoin.conf").write_text(
            "regtest=1\nserver=1\nlisten=0\ndnsseed=0\ndiscover=0\n"
            "txindex=1\nfallbackfee=0.00001\ndbcache=32\nmaxmempool=16\npar=1\n"
            f"[regtest]\nrpcbind=127.0.0.1\nrpcallowip=127.0.0.1\nrpcport={RPC_PORT}\n")
        with (datadir / "startup.log").open("ab") as log:
            subprocess.Popen(["bitcoind", f"-datadir={datadir}"], stdin=subprocess.DEVNULL,
                             stdout=log, stderr=log, start_new_session=True)
        wait_for(lambda: bitcoin("getblockchaininfo", wallet=False), "Bitcoin startup")
    if "base58" not in bitcoin("listwallets", wallet=False):
        existing = [item["name"] for item in bitcoin("listwalletdir", wallet=False)["wallets"]]
        bitcoin("loadwallet" if "base58" in existing else "createwallet", "base58", wallet=False)
    height = bitcoin("getblockcount")
    if height < 101:
        mine(101 - height)
    return {"chain": "regtest", "height": bitcoin("getblockcount"), "balance": bitcoin("getbalance")}


def mine(blocks=1):
    if not isinstance(blocks, int) or not 1 <= blocks <= 1000:
        raise ValueError("Mine between 1 and 1000 blocks at a time")
    return bitcoin("generatetoaddress", blocks, bitcoin("getnewaddress"))


def start_node(node):
    if node not in NODES:
        raise ValueError(node)
    try:
        return lightning(node, "getinfo")
    except RuntimeError:
        pass
    directory = ROOT / node
    directory.mkdir(parents=True, exist_ok=True)
    port = PEER_PORT + NODES.index(node)
    with (directory / "startup.log").open("ab") as log:
        process = subprocess.Popen([
            "lightningd", f"--lightning-dir={directory}", "--network=regtest",
            f"--bitcoin-datadir={ROOT / 'bitcoin'}", f"--bitcoin-cli={shutil.which('bitcoin-cli')}",
            f"--bind-addr=127.0.0.1:{port}", "--autolisten=false",
            "--developer", "--dev-bitcoind-poll=1", "--log-level=info", f"--log-file={directory / 'lightning.log'}",
        ], stdin=subprocess.DEVNULL, stdout=log, stderr=log, start_new_session=True)
    def ready():
        if process.poll() is not None:
            detail = (directory / "startup.log").read_text(errors="replace")[-3000:]
            raise ValueError(f"{node} stopped during startup: {detail}")
        return lightning(node, "getinfo")
    return wait_for(ready, f"{node} startup")


def sync_nodes():
    height = bitcoin("getblockcount")
    for node in NODES:
        wait_for(lambda: lightning(node, "getinfo")["blockheight"] >= height, f"{node} at height {height}")


def fund_nodes():
    """Idempotent initial funding; mine once, then wait for each node to catch up."""
    for node in NODES:
        if not lightning(node, "listfunds")["outputs"] and not lightning(node, "listpeerchannels")["channels"]:
            address = lightning(node, "newaddr", addresstype="bech32")["bech32"]
            bitcoin("sendtoaddress", address, 1)
    mine(1)
    sync_nodes()
    return {node: lightning(node, "listfunds") for node in NODES}


def open_channel(source, destination):
    """Run one opening per cell; repeated execution reuses the existing channel."""
    peer = lightning(destination, "getinfo")["id"]
    existing = [c for c in lightning(source, "listpeerchannels")["channels"] if c["peer_id"] == peer]
    if existing:
        return existing[0]["state"]
    lightning(source, "connect", id=peer, host="127.0.0.1", port=PEER_PORT + NODES.index(destination))
    return lightning(source, "fundchannel", id=peer, amount=1_000_000, feerate="1000perkw", announce=False)


def confirm_channels():
    mine(6)
    sync_nodes()
    for node in NODES:
        wait_for(lambda: (channels := lightning(node, "listpeerchannels")["channels"])
                 and all(c["state"] == "CHANNELD_NORMAL" for c in channels), f"{node} channels ready")
    return {node: [(c["peer_id"], c["state"]) for c in lightning(node, "listpeerchannels")["channels"]]
            for node in NODES}
