# CSV and CLTV

I want to make two outputs: one locked with **CLTV**, and one locked with **CSV**.

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

## Choose a funding input

Inspect the wallet’s unspent outputs and choose one to spend.

+++ timelock-unspent python-vm
from decimal import Decimal

unspent = bitcoin("listunspent")
unspent
+++

+++ timelock-select-input python-vm
coin = next(output for output in unspent if output["spendable"] and output["solvable"])
txid_to_spend = coin["txid"]
vout_to_spend = coin["vout"]
total_amt = int(Decimal(str(coin["amount"])) * 100_000_000)
funding_output_amount = (total_amt - 1_000) // 2
assert funding_output_amount > 1_000
print("Input:", txid_to_spend, vout_to_spend, total_amt, "sat")
print("Each timelocked output:", funding_output_amount, "sat")
+++

## Assemble the funding transaction

The witness program is `00 20 <SHA256(script)>`. Construct one input and two outputs. Transaction IDs are reversed in the input; the output index and amounts use little-endian encoding.

+++ timelock-funding-transaction python-vm
from codes import cleanup_tx

csv_wsh = sha256(csv_script).hexdigest()
cltv_wsh = sha256(cltv_script).hexdigest()
lockup_tx = cleanup_tx(f"""
version: 0200 0000
inputs: 01
    txid: {bytes.fromhex(txid_to_spend)[::-1].hex()}
    vout: {vout_to_spend.to_bytes(4, 'little').hex()}
    scriptSig: 00
    sequence: ffff ffff
outputs: 02
    amount: {funding_output_amount.to_bytes(8, 'little').hex()}
    scriptPubKey: 22 00 20 {cltv_wsh}
    amount: {funding_output_amount.to_bytes(8, 'little').hex()}
    scriptPubKey: 22 00 20 {csv_wsh}
locktime: 0000 0000
""")
print(lockup_tx)
bitcoin("decoderawtransaction", lockup_tx)
+++

## Sign and broadcast

The wallet owns the funding input. Ask it to sign the transaction, inspect the result, and broadcast it.

+++ timelock-sign-funding python-vm
signed = bitcoin("signrawtransactionwithwallet", lockup_tx)
assert signed["complete"], signed
signed_lockup_tx = signed["hex"]
bitcoin("decoderawtransaction", signed_lockup_tx)
+++

+++ fund-timelocks python-vm
funding_txid = bitcoin("sendrawtransaction", signed_lockup_tx)
funding = bitcoin("getrawtransaction", funding_txid, True)
cltv_vout, csv_vout = 0, 1
assert funding["vout"][cltv_vout]["scriptPubKey"]["hex"] == "0020" + cltv_wsh
assert funding["vout"][csv_vout]["scriptPubKey"]["hex"] == "0020" + csv_wsh
mine(1)
funding_height = bitcoin("getblockcount")
print("Funding transaction:", funding_txid)
print("Confirmed at height", funding_height)
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
        b"\x01", (funding_output_amount - 1_000).to_bytes(8, "little"),  # one output, 1000 sat fee
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

::: references
1. [BIP 65 — Absolute locktimes](https://github.com/bitcoin/bips/blob/master/bip-0065.mediawiki)

    Defines `OP_CHECKLOCKTIMEVERIFY`, which prevents spending before a specified block height or time.

2. [BIP 68 — Relative locktimes](https://github.com/bitcoin/bips/blob/master/bip-0068.mediawiki)

    Explains how input sequence numbers encode a delay measured from an output's confirmation.

3. [BIP 112 — Checking relative locktimes](https://github.com/bitcoin/bips/blob/master/bip-0112.mediawiki)

    Defines `OP_CHECKSEQUENCEVERIFY`, which enforces a minimum relative locktime within a Bitcoin script.
:::
