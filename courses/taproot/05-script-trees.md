# Balanced and unbalanced script trees

Let's make a taproot with a bunch of leaves!

We have a set of scripts that we want to allow any one of them to spend this bitcoin.

Build a tree of the scripts, and then compute the merkle root.

+++ taproot-w107 python-vm
scripts
+++

+++ taproot-w108 python-vm
balanced_tree = [[[nifty, j_nym], [c_nym, d_nym]], [[m_nym, da_nym], [ch_nym, n_nym]]]
unbalanced_tree = [ nifty, [ ch_nym, [ j_nym, [ c_nym, [ d_nym, [ m_nym, [ da_nym , n_nym ]]]]]]]
+++

+++ taproot-w109 python-vm
balanced_tree
+++

+++ taproot-w110 python-vm
unbalanced_tree
+++

## Now that our tree(s) are defined, we need to calculate the merkle root

We need a function that given our tree, will return a merkle root.

+++ taproot-w112 python-vm
def make_leaf(script_bytes):
    leaf_version = 0xc0
    # leaf version + len script + script
    data = bytes([leaf_version]) + size_compact_size(len(script_bytes)) + script_bytes
    return tag_hash(b'TapLeaf', data)
+++

???
id: taproot-w113
runtime: python-vm
Build the Merkle root of a binary tree of scripts. Leaves are hex strings; branches contain exactly two children.
---
def taptree_builder(tree):
    if isinstance(tree, str):
        return make_leaf(bytes.fromhex(tree))
    assert len(tree) == 2
    left = taptree_builder(tree[0])
    right = taptree_builder(tree[1])
    # Sort the two hashes, then compute the tagged TapBranch hash.
    return b''
---
from hashlib import sha256
def reference(tree):
    def tagged(tag, data):
        h = sha256(tag).digest()
        return sha256(h + h + data).digest()
    if isinstance(tree, str):
        script = bytes.fromhex(tree)
        return tagged(b'TapLeaf', b'\xc0' + bytes([len(script)]) + script)
    children = sorted([reference(tree[0]), reference(tree[1])])
    return tagged(b'TapBranch', b''.join(children))
for tree in ['51', ['51', '52'], ['52', '51'], [['51', '52'], '53'], ['51', ['52', '53']]]:
    assert taptree_builder(tree) == reference(tree), 'Hash each leaf and sort each pair of child hashes.'
---
Review the input bytes and try again.
???

+++ taproot-w114 python-vm
balanced_root = taptree_builder(balanced_tree)
balanced_root.hex()
+++

+++ taproot-w115 python-vm
unbalanced_root = taptree_builder(unbalanced_tree)
unbalanced_root.hex()
+++

+++ taproot-w116 python-vm
internal_pubkey.format().hex()
+++

+++ taproot-w117 python-vm
balanced_tweak_key = make_tweak_pubkey(internal_pubkey, balanced_root)
balanced_tweak_key.format().hex()
+++

+++ taproot-w118 python-vm
unbalanced_tweak_key = make_tweak_pubkey(internal_pubkey, unbalanced_root)
unbalanced_tweak_key.format().hex()
+++

+++ taproot-w119 python-vm
Q_balanced = make_external_pubkey(internal_pubkey, balanced_tweak_key)
Q_unbalanced = make_external_pubkey(internal_pubkey, unbalanced_tweak_key)
+++

+++ taproot-w120 python-vm
p2tr_balanced = make_p2tr(Q_balanced).hex()
p2tr_balanced
+++

+++ taproot-w121 python-vm
p2tr_unbalanced = make_p2tr(Q_unbalanced).hex()
p2tr_unbalanced
+++

+++ taproot-w122 python-vm
balanced_address = bitcoin("decodescript", p2tr_balanced)["address"]
balanced_address
+++

+++ taproot-w123 python-vm
unbalanced_address = bitcoin("decodescript", p2tr_unbalanced)["address"]
unbalanced_address
+++

## Fund both trees

+++ taproot-w124 python-vm
# Balanced tree address lockup
balanced_txid = bitcoin("sendtoaddress", balanced_address, 1.1)
balanced_txid
+++

+++ taproot-w125 python-vm
# Unbalanced tree address lockup
unbalanced_txid = bitcoin("sendtoaddress", unbalanced_address, 1.2)
unbalanced_txid
+++

+++ taproot-w126 python-vm
bitcoin("getrawtransaction", balanced_txid, True)
+++

+++ taproot-w127 python-vm
balanced_outpoint = (balanced_txid, find_output(balanced_txid, p2tr_balanced))
unbalanced_outpoint = (unbalanced_txid, find_output(unbalanced_txid, p2tr_unbalanced))
txid_balanced = bytes.fromhex(balanced_outpoint[0])[::-1].hex()
txid_unbal = bytes.fromhex(unbalanced_outpoint[0])[::-1].hex()
print(balanced_outpoint)
print(unbalanced_outpoint)
+++

## Let's Spend Some Things!

+++ taproot-w129 python-vm
# amount unbalanced
(12 * 10 ** 7 - 400).to_bytes(8, 'little').hex()
+++

+++ taproot-w130 python-vm
# amount balanced
(11 * 10 ** 7 - 400).to_bytes(8, 'little').hex()
+++

## Balanced spend transaction

Spend the balanced tree output using da_nym's script. The witness needs the unlocking value, leaf script, and a proof of inclusion.

+++ taproot-w132 python-vm
leaf_script = bytes.fromhex(da_nym)
leaf_script.hex()
+++

+++ taproot-w133 python-vm
unlock_script = b'Taproot Rocks!'
unlock_script.hex()
+++

+++ taproot-w134 python-vm
def build_proof_of_inclusion(tree, script):
    if isinstance(tree, str):
        script_bytes = bytes.fromhex(tree)
        leaf_hash = make_leaf(script_bytes)
        return leaf_hash, [], script == tree

    # calculate the branch hash.
    assert len(tree) == 2
    left_hash, left_proof, left_target = build_proof_of_inclusion(tree[0], script)
    right_hash, right_proof, right_target = build_proof_of_inclusion(tree[1], script)

    proof = []
    if left_target:
        left_proof.append(right_hash)
        proof = left_proof
    elif right_target:
        right_proof.append(left_hash)
        proof = right_proof

    # we have to order the left + right "alphabetically"
    # thank you arik :)
    if left_hash > right_hash:
        right_hash, left_hash = left_hash, right_hash
    branch_hash = tag_hash(b'TapBranch', left_hash + right_hash)
    return branch_hash, proof, left_target or right_target


_, proof_of_inclusion, _ = build_proof_of_inclusion(balanced_tree, da_nym)

print('proof', [ x.hex() for x in proof_of_inclusion])
+++

+++ taproot-w135 python-vm
print(b''.join(proof_of_inclusion).hex())

control_block = control_block_version_byte(Q_balanced) + internal_pubkey.format()[1:] + b''.join(proof_of_inclusion)
+++

+++ taproot-w136 python-vm
control_block.hex()
+++

+++ taproot-w137 python-vm
wits = [size_compact_size(len(unlock_script)) + unlock_script,
        size_compact_size(len(leaf_script)) + leaf_script,
        size_compact_size(len(control_block)) + control_block]

print(size_compact_size(len(wits)).hex())
for witness in wits:
    print(witness.hex())
+++

## Serialize the balanced spend

The control block contains three sibling hashes for this leaf.

+++ taproot-w139 python-vm
tx = cleanup_tx(f"""
version: 02000000
marker+flag: 0001
inputs: 01
    txid: {bytes.fromhex(balanced_outpoint[0])[::-1].hex()}
    vout: {(balanced_outpoint[1]).to_bytes(4, "little").hex()}
    scriptSig: 00
    sequence: ffffffff
outputs: 01
    amount: {(110000000 - 400).to_bytes(8, "little").hex()}
    scriptPubKey: 22 {destination_script}
witnesses:
    {(size_compact_size(len(wits)) + b''.join(wits)).hex()}
locktime: 00000000
""")
+++

+++ taproot-w140 python-vm
bitcoin("decoderawtransaction", tx)
+++

+++ taproot-w141 python-vm
acceptance = bitcoin("testmempoolaccept", [tx])
assert acceptance[0]["allowed"], acceptance
acceptance
balanced_spend_size = bitcoin("decoderawtransaction", tx)["vsize"]
+++

## Unbalanced spend transaction

Spend the same leaf from the unbalanced tree. How many sibling hashes does its proof need? Compare the control block length and transaction size with the balanced spend.

+++ taproot-w143 python-vm
leaf_script = bytes.fromhex(da_nym)
leaf_script.hex()
+++

+++ taproot-w144 python-vm
unlock_script = b'Taproot Rocks!'
unlock_script.hex()
+++

+++ taproot-w145 python-vm
_, proof_of_inclusion, _ = build_proof_of_inclusion(unbalanced_tree, da_nym)

print([ x.hex() for x in proof_of_inclusion])
+++

+++ taproot-w146 python-vm
control_block = control_block_version_byte(Q_unbalanced) + internal_pubkey.format()[1:] + b''.join(proof_of_inclusion)
control_block.hex()
+++

+++ taproot-w147 python-vm
from codes import size_compact_size


wits = [size_compact_size(len(unlock_script)) + unlock_script,
        size_compact_size(len(leaf_script)) + leaf_script,
        size_compact_size(len(control_block)) + control_block]

print(size_compact_size(len(wits)).hex())
for witness in wits:
    print(witness.hex())
+++

+++ taproot-w148 python-vm
unbal_tx = cleanup_tx(f"""
version: 02000000
marker+flag: 0001
inputs: 01
    txid: {bytes.fromhex(unbalanced_outpoint[0])[::-1].hex()}
    vout: {(unbalanced_outpoint[1]).to_bytes(4, "little").hex()}
    scriptSig: 00
    sequence: ffffffff
outputs: 01
    amount: {(120000000 - 400).to_bytes(8, "little").hex()}
    scriptPubKey: 22 {destination_script}
witnesses:
    {(size_compact_size(len(wits)) + b''.join(wits)).hex()}
locktime: 00000000
""")
+++

+++ taproot-w149 python-vm
bitcoin("decoderawtransaction", unbal_tx)
+++

+++ taproot-w150 python-vm
acceptance = bitcoin("testmempoolaccept", [unbal_tx])
assert acceptance[0]["allowed"], acceptance
acceptance
print("Balanced:", balanced_spend_size, "vB")
print("Unbalanced:", bitcoin("decoderawtransaction", unbal_tx)["vsize"], "vB")
+++

~~~
id: taproot-proof-depth
Why is the unbalanced spend larger for this leaf?
= Its control block needs more sibling hashes. [Each level on the proof path contributes 32 bytes.]
- Its signature is larger. [Neither of these scripts requires a signature.]
- Every script in the tree must be revealed. [Only the spent script and its proof are revealed.]
~~~

::: references
1. [BIP 341 — Tree construction](https://github.com/bitcoin/bips/blob/master/bip-0341.mediawiki)

    Binary script trees and proof-of-inclusion encoding.
:::
