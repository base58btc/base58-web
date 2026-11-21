# CSV and CLTV

I want to make two outputs: one locked with **CLTV**, and one locked with **CSV**. This is the experiment from the original notebook, with fresh transaction IDs and heights from your own regtest chain.

`OP_CHECKLOCKTIMEVERIFY` checks an absolute locktime. `OP_CHECKSEQUENCEVERIFY` checks a relative locktime. We will use block heights for both and keep the delay short enough to explore interactively.

## Start a funded wallet

+++ timelock-wallet python-vm
from lab import bitcoin, start_bitcoin, mine
from hashlib import sha256

start_bitcoin()
+++

## Build the locking scripts

Both scripts end with `OP_DROP OP_1`: after the timelock check, they discard the number and leave a true value on the stack. They require no signature. Use these scripts only with the workshop’s regtest coins.

Bitcoin Script numbers use little-endian signed magnitude. The helper below pushes a positive integer using its minimal encoding.

+++ timelock-scripts python-vm
def push_number(number):
    if not 0 <= number < 500_000_000:
        raise ValueError("This exercise uses nonnegative block heights")
    if number == 0:
        return b"\x00"
    if number <= 16:
        return bytes([0x50 + number])
    encoded = number.to_bytes((number.bit_length() + 7) // 8, "little")
    if encoded[-1] & 0x80:
        encoded += b"\x00"
    return bytes([len(encoded)]) + encoded

csv_delay = 6
cltv_height = bitcoin("getblockcount") + 8
csv_script = push_number(csv_delay) + bytes.fromhex("b27551")
cltv_script = push_number(cltv_height) + bytes.fromhex("b17551")

for name, script in (("CSV", csv_script), ("CLTV", cltv_script)):
    print(name, script.hex(), bitcoin("decodescript", script.hex())["asm"])
+++

The opcodes are `b2` for CSV, `b1` for CLTV, `75` for DROP, and `51` for OP_1. Change the delay and run the cell again to inspect a different script.

## Fund two P2WSH outputs

The witness program is `00 20 <SHA256(script)>`. Bitcoin Core can derive the corresponding address. Let the wallet choose and sign the funding inputs, then locate our outputs by their scripts rather than assuming an output index.

+++ fund-timelocks python-vm
csv_address = bitcoin("decodescript", csv_script.hex())["segwit"]["address"]
cltv_address = bitcoin("decodescript", cltv_script.hex())["segwit"]["address"]
funding_txid = bitcoin("sendmany", "", {csv_address: 0.01, cltv_address: 0.01})
funding = bitcoin("getrawtransaction", funding_txid, True)

def find_output(script):
    program = (b"\x00\x20" + sha256(script).digest()).hex()
    return next(output["n"] for output in funding["vout"] if output["scriptPubKey"]["hex"] == program)

csv_vout = find_output(csv_script)
cltv_vout = find_output(cltv_script)
mine(1)
funding_height = bitcoin("getblockcount")
print("Funding transaction:", funding_txid)
print("Confirmed at height", funding_height, "CSV output", csv_vout, "CLTV output", cltv_vout)
+++

## Serialize the spending transactions

A transaction spending these outputs supplies the locking script as its witness. CSV needs transaction version 2 and a sequence that enables the relative lock. CLTV needs a suitable `nLockTime` and a non-final sequence.

+++ serialize-timelock-spends python-vm
def compact_size(value):
    if value < 253:
        return bytes([value])
    if value <= 0xffff:
        return b"\xfd" + value.to_bytes(2, "little")
    if value <= 0xffffffff:
        return b"\xfe" + value.to_bytes(4, "little")
    return b"\xff" + value.to_bytes(8, "little")

destination = bitcoin("getnewaddress")
destination_script = bytes.fromhex(bitcoin("getaddressinfo", destination)["scriptPubKey"])

def spending_transaction(vout, witness_script, sequence, locktime):
    return b"".join([
        (2).to_bytes(4, "little"), b"\x00\x01",  # version, SegWit marker and flag
        b"\x01", bytes.fromhex(funding_txid)[::-1], vout.to_bytes(4, "little"),
        b"\x00", sequence.to_bytes(4, "little"),  # empty scriptSig, sequence
        b"\x01", (1_000_000 - 1_000).to_bytes(8, "little"),  # one output, 1000 sat fee
        compact_size(len(destination_script)), destination_script,
        b"\x01", compact_size(len(witness_script)), witness_script,  # one witness item
        locktime.to_bytes(4, "little"),
    ]).hex()

csv_spend = spending_transaction(csv_vout, csv_script, csv_delay, 0)
cltv_spend = spending_transaction(cltv_vout, cltv_script, 0xfffffffe, cltv_height)
print(bitcoin("decoderawtransaction", csv_spend))
+++

Before mining, predict which transaction the mempool will accept. `testmempoolaccept` checks without broadcasting.

+++ immature-timelocks python-vm
for name, transaction in (("CSV", csv_spend), ("CLTV", cltv_spend)):
    result = bitcoin("testmempoolaccept", [transaction])[0]
    print(name, result)
    assert not result["allowed"], "Run this check immediately after funding"
+++

## Wait for the relative lock

If the funding output confirmed at height `H`, a relative delay of six permits a spend in block `H + 6`. The mempool evaluates the next candidate block, so the tip only needs to reach `H + 5`.

+++ mature-csv python-vm
target = funding_height + csv_delay - 1
if bitcoin("getblockcount") < target:
    mine(target - bitcoin("getblockcount"))
result = bitcoin("testmempoolaccept", [csv_spend])[0]
assert result["allowed"], result
csv_spending_txid = bitcoin("sendrawtransaction", csv_spend)
print("CSV spend accepted:", csv_spending_txid)
+++

## Wait for the absolute lock

An absolute height locktime of `L` permits confirmation in a block whose height is **greater than** `L`. Once the tip reaches `L`, the mempool can accept the transaction for the next block.

+++ mature-cltv python-vm
if bitcoin("getblockcount") < cltv_height:
    mine(cltv_height - bitcoin("getblockcount"))
result = bitcoin("testmempoolaccept", [cltv_spend])[0]
assert result["allowed"], result
cltv_spending_txid = bitcoin("sendrawtransaction", cltv_spend)
mine(1)
print("CLTV spend confirmed:", cltv_spending_txid)
print("Current height:", bitcoin("getblockcount"))
+++

~~~
id: relative-lock-start
When does this CSV output’s six-block clock start?
= When its funding transaction confirms. [CSV expresses the age of the output being spent.]
- When you write the script. [Writing a script does not create an on-chain output.]
- At a fixed block height chosen before funding. [That describes the absolute height used by CLTV.]
~~~

Try Undo after the final cell. Inspect the height and mempool before running it again. The wallet and transaction IDs should return with the checkpoint.

::: references
1. [BIP 65 — Absolute locktimes](https://github.com/bitcoin/bips/blob/master/bip-0065.mediawiki)

   Introduces `OP_CHECKLOCKTIMEVERIFY`: require a transaction's locktime to reach a specified block height or time before an output can be spent.

2. [BIP 68 — Relative locktimes](https://github.com/bitcoin/bips/blob/master/bip-0068.mediawiki)

   Defines how transaction input sequence numbers encode a delay measured from the confirmation of the output being spent.

3. [BIP 112 — Checking relative locktimes](https://github.com/bitcoin/bips/blob/master/bip-0112.mediawiki)

   Introduces `OP_CHECKSEQUENCEVERIFY`: enforce a minimum relative locktime from within a Bitcoin script.
:::
