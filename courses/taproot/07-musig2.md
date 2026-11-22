# MuSig2 and Tapscript spends

In this module we'll be making a series of MuSig2 scripts.

We'll then build a MAST of the scripts, and lock up some bitcoin to them.
Finally we'll practice unlocking the script using a tapscript spend.

## Picking our private keys

We'll need three private keys. Pick three keys (integers) that are greater than 1 and less than `n`,

`n = 115792089237316195423570985008687907852837564279074904382605163141518161494337`

+++ taproot-m004 python-vm
privkeys = [444, 555, 666]
+++

+++ taproot-m005 python-vm
from coincurve import PrivateKey

pubkeys = [ PrivateKey.from_int(x).public_key for x in privkeys ]
+++

+++ taproot-m006 python-vm
[ x.format().hex() for x in pubkeys ]
+++

One thing to know about MuSig2 is that it is NOT a threshold signature scheme.

```
N of N

2 of 2 || 3 of 3

2 of 3 with MuSig2  -> BitGo

Keys: 444, 555, 666

"Aggregate key".
    444 + 555 -> 999

Three scripts to spend:
444 + 555:     <pubkey999> OP_CHECKSIG
444 + 666:     <pubkey1110> OP_CHECKSIG
555 + 666:     <pubkey1221> OP_CHECKSIG
```

The numbers 999, 1110, and 1221 label the signer pairs. MuSig2 aggregates public keys with coefficients; it does not simply add their private integers.

## Aggregate the signer pairs

+++ taproot-m008 python-vm
from bip327 import individual_pk, get_xonly_pk, key_agg_and_tweak

pk444 = individual_pk((444).to_bytes(32, 'big'))
pk555 = individual_pk((555).to_bytes(32, 'big'))
pk666 = individual_pk((666).to_bytes(32, 'big'))

pubkey999 = get_xonly_pk(key_agg_and_tweak([pk444, pk555], [], []))
pubkey999.hex()
+++

+++ taproot-m009 python-vm
leaf_999 = '20' + pubkey999.hex() + 'ac'
leaf_999
+++

+++ taproot-m010 python-vm
bitcoin("decodescript", leaf_999)
+++

+++ taproot-m011 python-vm
pubkey1110 = get_xonly_pk(key_agg_and_tweak([pk444, pk666], [], []))
leaf_1110 = '20' + pubkey1110.hex() + 'ac'
+++

+++ taproot-m012 python-vm
pubkey1221 = get_xonly_pk(key_agg_and_tweak([pk555, pk666], [], []))
leaf_1221 = '20' + pubkey1221.hex() + 'ac'
+++

## Build the script tree

+++ taproot-m013 python-vm
tree = [ [ leaf_999, leaf_1110 ], leaf_1221 ]
+++

+++ taproot-m014 python-vm
from codes import size_compact_size
from hashlib import sha256

def tag_hash(tag, data_bytes):
    taghash = sha256(tag).digest()
    return sha256(taghash + taghash + data_bytes).digest()

def make_leaf(script_bytes):
    leaf_version = 0xc0
    # leaf version + len script + script
    data = bytes([leaf_version]) + size_compact_size(len(script_bytes)) + script_bytes
    return tag_hash(b'TapLeaf', data)

def taptree_builder(tree):
    if isinstance(tree, str):
        script_bytes = bytes.fromhex(tree)
        leaf_hash = make_leaf(script_bytes)
        print("leaf:", leaf_hash.hex())
        return leaf_hash

    # calculate the branch hash.
    assert len(tree) == 2
    left = taptree_builder(tree[0])
    right = taptree_builder(tree[1])

    # we have to order the left + right "alphabetically"
    # thank you arik :)
    if left > right:
        right, left = left, right

    branch_hash = tag_hash(b'TapBranch', left + right)
    print("branch:", branch_hash.hex())
    return branch_hash
+++

+++ taproot-m015 python-vm
root = taptree_builder(tree)
root.hex()
+++

## Choose the internal key

+++ taproot-m016 python-vm
from codes import nums_point as musig_nums_point

internal_pubkey = musig_nums_point()
internal_pubkey.format().hex()
+++

+++ taproot-m017 python-vm
from codes import make_tweak_pubkey as musig_tweak, make_external_pubkey as musig_external
pubkey_bytes = internal_pubkey.point()[0].to_bytes(32, 'big')
tweak_pubkey = musig_tweak(pubkey_bytes, root)
Q = musig_external(pubkey_bytes, tweak_pubkey)

Q.format().hex()
+++

+++ taproot-m018 python-vm
def make_p2tr(Q):
    Qx_bytes = Q.point()[0].to_bytes(32, 'big')
    witness_version = 0x51
    return bytes([witness_version]) + size_compact_size(len(Qx_bytes)) + Qx_bytes
+++

+++ taproot-m019 python-vm
scriptpubkey = make_p2tr(Q).hex()
scriptpubkey
+++

## Fund the MuSig2 output

+++ taproot-m020 python-vm
send_to_addr = bitcoin("decodescript", scriptpubkey)["address"]
send_to_addr
+++

+++ taproot-m021 python-vm
txid_to_spend = bitcoin("sendtoaddress", send_to_addr, 1.0)
txid_to_spend
+++

+++ taproot-m022 python-vm
txid_to_spend
+++

+++ taproot-m023 python-vm
bitcoin("getrawtransaction", txid_to_spend, True)["vout"]
+++

## Build the unsigned spend

+++ taproot-m024 python-vm
vout = find_output(txid_to_spend, scriptpubkey)

unlock_tx_stuffs = f"""
version: 0200 0000
inputs: 01
    txid: {bytes.fromhex(txid_to_spend)[::-1].hex()}
    vout: {vout.to_bytes(4, 'little').hex()}
    scriptSig: 00
    sequence: ffff ffff
outputs: 01
    amount: {(1 * 10 ** 8 - 400).to_bytes(8, 'little').hex()}
    spk: 22 {scriptpubkey}
locktime: 0000 0000
"""
+++

+++ taproot-m025 python-vm
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
+++

+++ taproot-m026 python-vm
unlock_tx = cleanup_tx(unlock_tx_stuffs)
unlock_tx
+++

+++ taproot-m027 python-vm
bitcoin("decoderawtransaction", unlock_tx)
+++

## Compute the signature message

+++ taproot-m028 python-vm
from codes import parse_compact_size, parse_input_bytes, parse_output_bytes

def parse_witness_bytes(data):
    count, size = parse_compact_size(data)
    ptr = size
    witnesses = []
    for _ in range(0, count):
        witlen, size = parse_compact_size(data[ptr:])
        ptr += size
        witnesses.append(data[ptr:ptr+witlen])
        ptr += witlen

    return witnesses, ptr

def parse_tx_bytes_mine(tx_hex):
  tx_bytes = bytes.fromhex(tx_hex)
  tx = {}
  ptr = 0
  tx['version'] = tx_bytes[0:4]
  ptr += 4

  if tx_bytes[ptr] == 0x00:
    assert tx_bytes[ptr+1] == 0x01
    tx['marker_flag'] = bytes([0x00, 0x01])
    ptr += 2

  count, size = parse_compact_size(tx_bytes[ptr:])
  ptr += size
  tx['inputs'] = []
  for _ in range(0, count):
    inputx, size = parse_input_bytes(tx_bytes[ptr:])
    ptr += size
    tx['inputs'].append(inputx)

  count, size = parse_compact_size(tx_bytes[ptr:])
  ptr += size
  tx['outputs'] = []
  for _ in range(0, count):
    outputx, size = parse_output_bytes(tx_bytes[ptr:])
    ptr += size
    tx['outputs'].append(outputx)

  if 'marker_flag' in tx:
    tx['witnesses'] = []
    for _ in range(0, len(tx['inputs'])):
        witness, size = parse_witness_bytes(tx_bytes[ptr:])
        ptr += size
        tx['witnesses'].append(witness)

  tx['locktime'] = tx_bytes[ptr:]
  return tx
+++

+++ taproot-m029 python-vm
from hashlib import sha256
from codes import size_compact_size

def sigmsg_default(tx_hex, input_index, annex_bytes, amounts_bytes, scriptpubkeys_bytes, ext_flag):
    txdata = parse_tx_bytes_mine(tx_hex)

    result = b''
    result += bytes([0x00]) # sighash_flag
    result += txdata['version']
    result += txdata['locktime']

    all_input_outpoints = b''
    for inp in txdata['inputs']:
        # txid || vout
        all_input_outpoints += inp['txid'] + inp['vout']
    sha_prevouts = sha256(all_input_outpoints).digest()
    result += sha_prevouts

    sha_amounts = sha256(b''.join(amounts_bytes)).digest()
    result += sha_amounts

    spks = b''
    for spk in scriptpubkeys_bytes:
        spks += size_compact_size(len(spk)) + spk
    sha_scriptpubkeys = sha256(spks).digest()
    result += sha_scriptpubkeys

    all_sequences = [ i['sequence'] for i in txdata['inputs'] ]
    sha_sequences = sha256(b''.join(all_sequences)).digest()
    result += sha_sequences

    all_outputs = b''
    for o in txdata['outputs']:
        # amount || compact_size(scriptpubkey) || scriptpubkey
        all_outputs += o['amount']
        all_outputs += size_compact_size(len(o['scriptPubKey']))
        all_outputs += o['scriptPubKey']
    sha_outputs = sha256(all_outputs).digest()
    result += sha_outputs

    # data about this input
    annex_present = 1 if annex_bytes else 0
    spend_type = 2 * ext_flag + annex_present
    result += (spend_type).to_bytes(1, 'little')
    result += (input_index).to_bytes(4, 'little')

    if (annex_bytes):
        size_bytes = size_compact_size(len(annex_bytes))
        sha_annex = sha256(size_bytes + annex_bytes).digest()
        result += sha_annex

    # data about this output
    # noop for SIGHASH_DEFAULT

    assert len(result) <= 206
    return result
+++

+++ taproot-m030 python-vm
prev_tx = bitcoin("getrawtransaction", txid_to_spend)
spent_from_tx = parse_tx_bytes_mine(prev_tx)
+++

+++ taproot-m031 python-vm
tapleaf_hash = make_leaf(bytes.fromhex(leaf_999))
key_version = bytes([0x00])
codesep_position = bytes([0xff, 0xff, 0xff, 0xff])

extension_data = tapleaf_hash + key_version + codesep_position
extension_data.hex()
+++

+++ taproot-m032 python-vm
amounts_bytes = [spent_from_tx['outputs'][vout]['amount']]
print("amounts:", [x.hex() for x in amounts_bytes])

scriptpubkeys_bytes = [spent_from_tx['outputs'][vout]['scriptPubKey']]
print("scriptpubkeys:", [x.hex() for x in scriptpubkeys_bytes], "\n")

ext_flag = 1 if extension_data else 0
annex_bytes = None
input_index = 0

sigmsg = sigmsg_default(unlock_tx, input_index, annex_bytes, amounts_bytes, scriptpubkeys_bytes, ext_flag)

# add extension data to end of what's passed into tag_hash
sighash = tag_hash(b'TapSighash', bytes([0x00]) + sigmsg + extension_data)
sighash.hex()
+++

## Create and aggregate nonces

+++ taproot-m033 python-vm
from bip327 import nonce_gen, nonce_agg, SessionContext, sign, partial_sig_agg

privkey444 = (444).to_bytes(32, 'big')
privkey555 = (555).to_bytes(32, 'big')

sec444, pub444 = nonce_gen(privkey444, pk444, pubkey999, sighash, b'')
sec555, pub555 = nonce_gen(privkey555, pk555, pubkey999, sighash, b'')
pubnonces = [pub444, pub555]
aggnonce = nonce_agg(pubnonces)
+++

+++ taproot-m034 python-vm
sesh = SessionContext(aggnonce, [pk444, pk555], [], [], sighash)

partial_sig444 = sign(sec444, privkey444, sesh)
partial_sig555 = sign(sec555, privkey555, sesh)
sig = partial_sig_agg([partial_sig444, partial_sig555], sesh)
assert coincurve.PublicKeyXOnly(pubkey999).verify(sig, sighash)
sig.hex()
+++

## Build the proof and witness

+++ taproot-m035 python-vm
leaf_script = bytes.fromhex(leaf_999)
leaf_999
+++

+++ taproot-m036 python-vm
def control_block_version_byte(Q):
    val = 0xc0
    if Q.point()[1] % 2 != 0:
        val += 1
    return bytes([val])
+++

+++ taproot-m037 python-vm
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
+++

+++ taproot-m038 python-vm
_, proof_of_inclusion, _ = build_proof_of_inclusion(tree, leaf_999)
control_block = control_block_version_byte(Q) + internal_pubkey.format()[1:] + b''.join(proof_of_inclusion)
control_block.hex()
+++

+++ taproot-m039 python-vm
witnesses = [
    sig,
    leaf_script,
    control_block
]
+++

+++ taproot-m040 python-vm
def build_witness_str(wits):
    wit_str = size_compact_size(len(wits)).hex()
    for witness in wits:
        wit_str += size_compact_size(len(witness)).hex() + witness.hex()
    return wit_str
+++

## Serialize and test the spend

+++ taproot-m041 python-vm
signed_tx_stuffs = f"""
version: 0200 0000
marker + flag: 0001
inputs: 01
    txid: {bytes.fromhex(txid_to_spend)[::-1].hex()}
    vout: {vout.to_bytes(4, 'little').hex()}
    scriptSig: 00
    sequence: ffff ffff
outputs: 01
    amount: {(1 * 10 ** 8 - 400).to_bytes(8, 'little').hex()}
    spk: 22 {scriptpubkey}
witnesses:
    {build_witness_str(witnesses)}
locktime: 0000 0000
"""

signed_tx = cleanup_tx(signed_tx_stuffs)
+++

+++ taproot-m042 python-vm
bitcoin("decoderawtransaction", signed_tx)["vin"][0]["txinwitness"]
+++

+++ taproot-m043 python-vm
acceptance = bitcoin("testmempoolaccept", [signed_tx])
assert acceptance[0]["allowed"], acceptance
acceptance
+++

~~~
id: taproot-musig-threshold
How does this example implement 2-of-3 spending with MuSig2?
= It commits to three leaves, each requiring a different pair. [Each aggregate key is an all-signers MuSig2 key for that pair.]
- One three-party MuSig2 key can sign with any two parties. [MuSig2 itself is an n-of-n scheme.]
- It adds two private keys to obtain the aggregate key. [MuSig2 uses weighted public-key aggregation.]
~~~

::: references
1. [BIP 327 — MuSig2](https://github.com/bitcoin/bips/blob/master/bip-0327.mediawiki)

    Key aggregation, nonce generation, and partial signatures.
:::
