# Lightning, from transactions to onions

This workshop ports Base58’s November 2023 Lightning class into browser lessons. You will build timelocked transactions, decode invoices, derive revocation secrets, explore a real gossip graph, and construct a payment onion.

The lessons follow the original [class notebooks](https://github.com/base58btc/lightning_class/tree/62491b76177123c8cebcc0fadccc93ca826d8dc7). Each code cell is editable. Run cells in order within a lesson; later cells use variables created above them.

## Your workshop

1. **CSV and CLTV:** create two outputs, then discover when each can be spent.
2. **Lightning invoices:** read a BOLT 11 invoice and recover its signing key.
3. **Revocation generation:** generate per-commitment secrets and encode a commitment number.
4. **Gossip for fun and profit:** find a route and calculate its fees and timelocks.
5. **Build and send an onion:** use three local nodes to pay a real regtest invoice.

::: note
Bitcoin and Lightning run on **regtest**, with coins mined inside your course environment. Historical mainnet gossip is an offline dataset for analysis. The local payment exercise uses its own nodes and channels.
:::

## Run, save, and undo

Run this first cell to open your environment. The first launch downloads the course tools. Python, Bitcoin, and Lightning execute inside the browser VM.

+++ check-tools python-vm
import platform
import subprocess
from pathlib import Path

print("Python", platform.python_version())
for program in ("bitcoind", "lightningd"):
    print(subprocess.check_output([program, "--version"], text=True).splitlines()[0])
print("Workspace:", Path.cwd())
+++

Each completed cell saves a checkpoint of the VM, including Python variables, files, wallets, and running services. **Save and stop** releases the environment so you can resume on another device when signed in. **Undo** returns to the checkpoint before the most recent execution. A local draft preview saves in that browser only.

Re-running a cell executes it again. That matters for cells that mine blocks, send coins, or generate random values. The command history records each execution; use Undo when you mean to reverse one.

::: note
These cells use ordinary Python. The original notebooks’ `!command`, `$variable`, and `result = !command` expressions have been replaced with explicit helpers such as `bitcoin("getblockcount")`. These helpers call the CLI inside your VM and return decoded JSON.
:::

~~~
id: execution-location
Where do the course’s Bitcoin and Lightning nodes run?
= Inside the browser VM. [Their state is included in the course checkpoint.]
- On the public Bitcoin network. [This workshop uses an isolated regtest chain.]
- On a shared server node. [Each course attempt has its own environment.]
~~~
