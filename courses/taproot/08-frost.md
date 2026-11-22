# FROST threshold signatures

FROST is a threshold aggregated signature protocol. This lets you produce a 2-of-3 threshold signature as a single Schnorr signature.

It uses Shamir secret sharing to distribute key shares and Lagrange coefficients to combine a signing quorum's contributions.

For this we're going to use Jesse Posner's toy BIP340-compatible FROST implementation included with the workshop.

## Generate and distribute key shares

+++ taproot-m045 python-vm
from frost import FROST

p1 = FROST.Participant(index=1, threshold=2, participants=3)
p2 = FROST.Participant(index=2, threshold=2, participants=3)
p3 = FROST.Participant(index=3, threshold=2, participants=3)

p1.init_keygen()
p2.init_keygen()
p3.init_keygen()

p1.generate_shares()
p2.generate_shares()
p3.generate_shares()

p1.aggregate_shares([p2.shares[p1.index-1], p3.shares[p1.index-1]])
p2.aggregate_shares([p1.shares[p2.index-1], p3.shares[p2.index-1]])
p3.aggregate_shares([p1.shares[p3.index-1], p2.shares[p3.index-1]])

pubkey = p1.derive_public_key([p2.coefficient_commitments[0], p3.coefficient_commitments[0]])
p2.derive_public_key([p1.coefficient_commitments[0], p3.coefficient_commitments[0]])
p3.derive_public_key([p1.coefficient_commitments[0], p2.coefficient_commitments[0]])


pubkey.xonly_serialize().hex()
+++

## Fund the group key

+++ taproot-m046 python-vm
spk = "5120" + pubkey.xonly_serialize().hex()
addr = bitcoin("decodescript", spk)["address"]
spend_txid = bitcoin("sendtoaddress", addr, 1.0)
spend_txid
+++

+++ taproot-m047 python-vm
spend_tx = bitcoin("getrawtransaction", spend_txid)
spend_tx_parsed = parse_tx_bytes_mine(spend_tx)
vout = find_output(spend_txid, spk)
vout
+++

## Build the key-path spend

+++ taproot-m048 python-vm
unlock_tx_stuffs = f"""
version: 0200 0000
inputs: 01
    txid: {bytes.fromhex(spend_txid)[::-1].hex()}
    vout: {vout.to_bytes(4, 'little').hex()}
    scriptSig: 00
    sequence: ffff ffff
outputs: 01
    amount: {(1 * 10 ** 8 - 400).to_bytes(8, 'little').hex()}
    spk: 22 {spk}
locktime: 0000 0000
"""

unlock_tx = cleanup_tx(unlock_tx_stuffs)
+++

+++ taproot-m049 python-vm
extension_data = b''
amounts_bytes = [spend_tx_parsed['outputs'][vout]['amount']]
scriptpubkeys_bytes = [spend_tx_parsed['outputs'][vout]['scriptPubKey']]
print(amounts_bytes[0].hex())
print(scriptpubkeys_bytes[0].hex())

ext_flag = 1 if extension_data else 0
annex_bytes = None
input_index = 0

sigmsg = sigmsg_default(unlock_tx, input_index, annex_bytes, amounts_bytes, scriptpubkeys_bytes, ext_flag)

# add extension data to end of what's passed into tag_hash
sighash = tag_hash(b'TapSighash', bytes([0x00]) + sigmsg + extension_data)
sighash.hex()
+++

## Sign with two participants

+++ taproot-m050 python-vm
# NonceGen
p1.generate_nonces(1)
p2.generate_nonces(1)
p3.generate_nonces(1)

# Sign
participant_indexes = [2, 3]
agg = FROST.Aggregator(pubkey, sighash, [None, p2.nonce_commitment_pairs, p3.nonce_commitment_pairs], participant_indexes)
message, nonce_commitment_pairs = agg.signing_inputs()
assert message == sighash

s1 = p2.sign(sighash, nonce_commitment_pairs, participant_indexes)
s2 = p3.sign(sighash, nonce_commitment_pairs, participant_indexes)

# σ = (R, z)
from codes import p, n
sig = bytes.fromhex(agg.signature([s1, s2]))

# verify
assert coincurve.PublicKeyXOnly(pubkey.xonly_serialize()).verify(sig, sighash)
sig.hex()
+++

## Broadcast the threshold spend

+++ taproot-m051 python-vm
witnesses = [
    sig
]
+++

+++ taproot-m052 python-vm
signed_tx = cleanup_tx(f"""
version: 0200 0000
marker+flag: 0001
inputs: 01
    txid: {bytes.fromhex(spend_txid)[::-1].hex()}
    vout: {vout.to_bytes(4, 'little').hex()}
    scriptSig: 00
    sequence: ffff ffff
outputs: 01
    amount: {(1 * 10 ** 8 - 400).to_bytes(8, 'little').hex()}
    spk: 22 {spk}
witness:
    {build_witness_str(witnesses)}
locktime: 0000 0000
""")
+++

+++ taproot-m053 python-vm
acceptance = bitcoin("testmempoolaccept", [signed_tx])
assert acceptance[0]["allowed"], acceptance
acceptance
+++

+++ taproot-m054 python-vm
frost_spend_txid = bitcoin("sendrawtransaction", signed_tx)
mine(1)
bitcoin("getrawtransaction", frost_spend_txid, True)["confirmations"]
+++

~~~
id: taproot-frost-quorum
How many participants sign in this 2-of-3 FROST example?
= Two. [Participants 2 and 3 contribute shares to one valid Schnorr signature.]
- All three. [The threshold is two, even though three participants joined key generation.]
- One. [One participant does not hold enough shares to sign.]
~~~

::: references
1. [FROST — Flexible Round-Optimized Schnorr Threshold Signatures](https://crysp.uwaterloo.ca/software/frost/)

    The threshold signature protocol by Chelsea Komlo and Ian Goldberg.
:::
