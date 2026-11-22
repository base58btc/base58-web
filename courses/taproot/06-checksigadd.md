# Multisig with OP_CHECKSIGADD

Specifically a "threshold multisig".  "2 of 3" // 5 of 7 etc

Why this particular task?
    - Highlights the removal of OP_CHECKMULTISIG and its replacement
    - Why FROST vs MuSig2


What are the three ways we're going to do a threshold spend?

- OP_CHECKSIGADD
- MuSig2 + tapscript leaves
- FROST (external pubkey // internal pubkey)

I'm going to do 2 of 3 for our initial examples.

+++ taproot-w152 python-vm
privkey = 8589393459493948599858588583933824757589382718181271727004040303
assert privkey < n
pubkey = coincurve.PrivateKey.from_int(privkey).public_key
pubkey.format().hex()
+++

+++ taproot-w153 python-vm
private_keys = [444, 555, 666]
private_keys
+++

+++ taproot-w154 python-vm
import coincurve
pubkeys = [ coincurve.PrivateKey.from_int(x).public_key for x in private_keys ]
[ x.format().hex() for x in pubkeys ]
+++

## OP_CHECKSIGADD

`<pubkey> OP_CHECKSIG <pubkey2> OP_CHECKSIGADD <pubkey3> OP_CHECKSIGADD OP_2 OP_EQUAL`


#### OP_CHECKSIGADD from `interpeter.cpp`
```
    const valtype& sig = stacktop(-3);
    const CScriptNum num(stacktop(-2), fRequireMinimal);
    const valtype& pubkey = stacktop(-1);
    EvalChecksig... -> success
    ...
     stack.push_back((num + (success ? 1 : 0))
```

```
20 9e6d1d11b101055620cbf7a8bfda75ac1fac813a96608f9c59eac485c16b9f1f  OP_CHECKSIG
20 d811d8c4323b43702607c25ade936c0ac2e4a44b2ba51f8320c5179e745a7e29  OP_CHECKSIGADD
20 7f75c66c45a52c35ead5970bbfaafdfba626a6ddceabc14e0f8a8c7d88a5772b  OP_CHECKSIGADD OP_2 OP_EQUAL
```

```
20 9e6d1d11b101055620cbf7a8bfda75ac1fac813a96608f9c59eac485c16b9f1f  ac
20 d811d8c4323b43702607c25ade936c0ac2e4a44b2ba51f8320c5179e745a7e29  ba
20 7f75c66c45a52c35ead5970bbfaafdfba626a6ddceabc14e0f8a8c7d88a5772b  ba 52 87
```

+++ taproot-w158 python-vm
lock_script = '209e6d1d11b101055620cbf7a8bfda75ac1fac813a96608f9c59eac485c16b9f1fac20d811d8c4323b43702607c25ade936c0ac2e4a44b2ba51f8320c5179e745a7e29ba207f75c66c45a52c35ead5970bbfaafdfba626a6ddceabc14e0f8a8c7d88a5772bba5287'
+++

+++ taproot-w159 python-vm
bitcoin("decodescript", lock_script)
+++

+++ taproot-w160 python-vm
# Now that I have a script, I need to get an address for it!

tree = lock_script
add_root = taptree_builder(tree)
add_root.hex()
+++

+++ taproot-w161 python-vm
internal_pubkey = nums_point(800)
+++

+++ taproot-w162 python-vm
tweak_key = make_tweak_pubkey(internal_pubkey, add_root)
external_pubkey = make_external_pubkey(internal_pubkey, tweak_key)
v1script = make_p2tr(external_pubkey)
external_script = v1script.hex()
external_script
+++

+++ taproot-w163 python-vm
add_address = bitcoin("decodescript", external_script)["address"]
add_address
+++

## Fund the multisig output

+++ taproot-w164 python-vm
add_txid = bitcoin("sendtoaddress", add_address, 1.0)
add_vout = find_output(add_txid, external_script)
add_txid, add_vout
+++

+++ taproot-w165 python-vm
bitcoin("getrawtransaction", add_txid, True)["vout"]
+++

+++ taproot-w166 python-vm
prev_tx = bitcoin("getrawtransaction", add_txid)
spent_from_tx = parse_tx_bytes_mine(prev_tx)
+++

+++ taproot-w167 python-vm
txid = add_txid
bckwd_txid = bytes.fromhex(txid)[::-1].hex()
bckwd_txid
+++

+++ taproot-w168 python-vm
amount = (1 * 10 ** 8 - 400).to_bytes(8, 'little')
amount.hex()
+++

Now we have a locked up some bitcoin. Let's spend it!

## Build the unsigned spend

+++ taproot-w170 python-vm
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

+++ taproot-w171 python-vm
tx = cleanup_tx(f"""
version: 02000000
inputs: 01
    txid: {bytes.fromhex(add_txid)[::-1].hex()}
    vout: {add_vout.to_bytes(4, "little").hex()}
    scriptSig: 00
    sequence: fdffffff
outputs: 01
    amount: {(100000000 - 400).to_bytes(8, "little").hex()}
    scriptPubKey: 22 {destination_script}
locktime: 00000000
""")
tx
+++

+++ taproot-w172 python-vm
bitcoin("decoderawtransaction", tx)
+++

+++ taproot-w173 python-vm
proof_of_inclusion = []
control_block = control_block_version_byte(external_pubkey) + internal_pubkey.format()[1:] + b''.join(proof_of_inclusion)
control_block.hex()
+++

## Sign the script-path message

+++ taproot-w174 python-vm
lock_script_fr = bytes.fromhex(lock_script)
tapleaf_hash = make_leaf(lock_script_fr)
key_version = bytes([0x00])
codesep_position = bytes([0xff, 0xff, 0xff, 0xff])

extension_data = tapleaf_hash + key_version + codesep_position
extension_data.hex()
+++

+++ taproot-w175 python-vm
vout = add_vout
amounts_bytes = [spent_from_tx['outputs'][vout]['amount']]
print("amounts:", [x.hex() for x in amounts_bytes])

scriptpubkeys_bytes = [spent_from_tx['outputs'][vout]['scriptPubKey']]
print("scriptpubkeys:", [x.hex() for x in scriptpubkeys_bytes], "\n")

ext_flag = 1 if extension_data else 0
annex_bytes = None
input_index = 0

sigmsg = sigmsg_default(tx, input_index, annex_bytes, amounts_bytes, scriptpubkeys_bytes, ext_flag)

# add extension data to end of what's passed into tag_hash
sighash = tag_hash(b'TapSighash', bytes([0x00]) + sigmsg + extension_data)

nonce = 10
sig1 = sign_msg(private_keys[0], sighash, nonce)
assert len(sig1) == 64
sig1.hex()
+++

+++ taproot-w176 python-vm
# now we just sign the same message w/ a new key!
nonce = 11
sig2 = sign_msg(private_keys[1], sighash, nonce)
sig2.hex()
+++

+++ taproot-w177 python-vm
sig3 = b''
+++

## Assemble the witness

+++ taproot-w178 python-vm
lock_script_fr = bytes.fromhex(lock_script)

wits = [
    sig3,
    sig2,
    sig1,
    lock_script_fr,
    control_block
]
+++

+++ taproot-w179 python-vm
def print_witnesses(wits):
    print(size_compact_size(len(wits)).hex())
    for witness in wits:
        print(size_compact_size(len(witness)).hex() + witness.hex())

print_witnesses(wits)
+++

+++ taproot-w180 python-vm
def build_witness_str(wits):
    wit_str = size_compact_size(len(wits)).hex()
    for witness in wits:
        wit_str += size_compact_size(len(witness)).hex() + witness.hex()
    return wit_str

build_witness_str(wits)
+++

+++ taproot-w181 python-vm
candidate_tx = cleanup_tx(f"""
version: 02000000
marker+flag: 0001
inputs: 01
    txid: {bytes.fromhex(add_txid)[::-1].hex()}
    vout: {add_vout.to_bytes(4, "little").hex()}
    scriptSig: 00
    sequence: fdffffff
outputs: 01
    amount: {(100000000 - 400).to_bytes(8, "little").hex()}
    scriptPubKey: 22 {destination_script}
witnesses:
    {build_witness_str(wits)}
locktime: 00000000
""")
candidate_tx
+++

+++ taproot-w182 python-vm
bitcoin("decoderawtransaction", candidate_tx)
+++

+++ taproot-w183 python-vm
acceptance = bitcoin("testmempoolaccept", [candidate_tx])
assert acceptance[0]["allowed"], acceptance
acceptance
+++

~~~
id: taproot-empty-signature
Why does the third signer contribute an empty witness item?
= An absent signature adds zero to the signature count. [The first two valid signatures satisfy the 2-of-3 script.]
- An empty item signs any message. [It does not authorize a spend.]
- OP_CHECKSIGADD ignores the public key. [Each signature opcode still consumes its corresponding key.]
~~~

::: references
1. [BIP 342 — Tapscript signature opcodes](https://github.com/bitcoin/bips/blob/master/bip-0342.mediawiki)

    OP_CHECKSIGADD and threshold scripts.
:::
