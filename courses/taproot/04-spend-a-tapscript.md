# Spend a Tapscript

We should have a transaction output with bitcoin in it that's locked to your custom written script.

Now we want to create a transaction that spends that output. Its outpoint is the transaction ID and output index saved as single_txid and single_vout.

Let's build a transaction that spends this!

+++ taproot-w084 python-vm
txid = single_txid
reverse_txid = bytes.fromhex(txid)[::-1].hex()
print(reverse_txid)

amount = (1 * 10 ** 8 - 400).to_bytes(8, 'little')
print(amount.hex())
+++

+++ taproot-w085 python-vm
destination = bitcoin("getnewaddress", "", "bech32m")
destination
+++

+++ taproot-w086 python-vm
destination_script = bitcoin("getaddressinfo", destination)["scriptPubKey"]
destination_script
+++

## Assemble the spending transaction

Use the funding outpoint and the destination script from the previous cells. Leave room for the witness: an unlocking value, the leaf script, and a control block.

+++ taproot-w088 python-vm
import re
def cleanup_tx(hmn_read_tx):
    """ Given a block of text, strip out everything except 
        the hex strings
    """
    ret_val = []
    lines = hmn_read_tx.split('\n')
    for line in lines:
        substr = line.split(':')[-1]  # suggested-by @chrisguida + @macaki
        ret_val += re.findall(r'[0-9a-fA-F]{2}', substr)
    return ''.join(ret_val)

tx = cleanup_tx(f"""
version: 02000000
inputs: 01
    txid: {bytes.fromhex(single_txid)[::-1].hex()}
    vout: {single_vout.to_bytes(4, "little").hex()}
    scriptSig: 00
    sequence: ffffffff
outputs: 01
    amount: {(100000000 - 400).to_bytes(8, "little").hex()}
    scriptPubKey: 22 {destination_script}
locktime: 00000000
""")
bitcoin("decoderawtransaction", tx)
+++

## What's in the witness stack for a leaf spend?

We need to put into the witness stack:
 - the unlocking script
 - original leaf script
 - "control block": this has all the data you need to verify the external public key
 
 #### Nifty's Unlock Script is...
 
 Well my original leaf script

+++ taproot-w090 python-vm
bitcoin("decodescript", nifty)["asm"]
+++

Q: How do I unlock this? Great question.

Short A: 04
Long A: The number "4". In script, we'd use the opcode 'OP_4'
but in witnesses we can't use opcodes, so we just do a "push of the byte 04".

+++ taproot-w092 python-vm
bitcoin("decodescript", "0104")
+++

- Unlocking value: 04
- Original leaf script: 54935887
- Control block: leaf version/parity, internal key, and Merkle proof.

#### How do we figure out a control block?


A control block has three parts.

- version byte
- the x-only pubkey of the internal key
- proof of inclusion

##### Version Byte

We're going to need Q to figure this out.

The base value for the version byte is `c0`.
With the following modification: if Q is odd, add 1 to the base value.

If Q is even (02): 0xc0

If Q is odd (03):  0xc1

## Build the control block

+++ taproot-w097 python-vm
def control_block_version_byte(Q):
    val = 0xc0
    if Q.point()[1] % 2 != 0:
        val += 1
    return bytes([val])

print(control_block_version_byte(nifty_Q).hex())
+++

##### The x-only pubkey of the internal key

This should just be the internal pubkey, 32 bytes.

+++ taproot-w099 python-vm
print(internal_pubkey.format()[1:].hex())
+++

###### Proof of Inclusion

The proof of inclusion for a leaf-only spend is empty.

+++ taproot-w101 python-vm
control_block = control_block_version_byte(nifty_Q) + internal_pubkey.format()[1:]

print('witness data!')
print('03')
print('01 04')
print('04 ' + nifty)
print('21 ' + control_block.hex())
+++

## Add the witness

Serialize each witness item with its CompactSize length. The witness contains the script itself; its leaf version belongs in the control block.

+++ taproot-w103 python-vm
def build_witness_str(wits):
    wit_str = size_compact_size(len(wits)).hex()
    for witness in wits:
        wit_str += size_compact_size(len(witness)).hex() + witness.hex()
    return wit_str


witnesses = [bytes.fromhex("04"), bytes.fromhex(nifty), control_block]
spend_tx = cleanup_tx(f"""
version: 02000000
marker+flag: 0001
inputs: 01
    txid: {bytes.fromhex(single_txid)[::-1].hex()}
    vout: {single_vout.to_bytes(4, "little").hex()}
    scriptSig: 00
    sequence: ffffffff
outputs: 01
    amount: {(100000000 - 400).to_bytes(8, "little").hex()}
    scriptPubKey: 22 {destination_script}
witnesses:
    {build_witness_str(witnesses)}
locktime: 00000000
""")
+++

+++ taproot-w104 python-vm
spend_tx
+++

+++ taproot-w105 python-vm
acceptance = bitcoin("testmempoolaccept", [spend_tx])
assert acceptance[0]["allowed"], acceptance
acceptance
+++

~~~
id: taproot-witness-four
What witness item unlocks OP_4 OP_ADD OP_8 OP_EQUAL?
= The byte 04. [Witness items are data; the script adds four to the provided number.]
- The byte 54 (OP_4). [Witness data is not executed as opcodes.]
- An empty item. [An empty item represents zero, so the equality fails.]
~~~

::: references
1. [BIP 341 — Script validation rules](https://github.com/bitcoin/bips/blob/master/bip-0341.mediawiki#script-validation-rules)

    Control blocks, Merkle proofs, and the witness stack.
:::
