# Schnorr signatures

## Parse a SegWit transaction

+++ taproot-w018 python-vm
from pprint import pprint
from codes import parse_tx_bytes

tx = "020000000001013c29211e2e6b9a594f28fd9e8b650dd2f64e7024d398fbc4c98f3b8800cef1350000000000ffffffff0358020000000000002251203905d81b457b42e8d5eb1ae5280c83c74fbdd20a2e401c0077bce5417909c95358020000000000002251203905d81b457b42e8d5eb1ae5280c83c74fbdd20a2e401c0077bce5417909c953eba60d00000000002251203905d81b457b42e8d5eb1ae5280c83c74fbdd20a2e401c0077bce5417909c9530140bc93e2b4ae198c6326737b3e1525715109b6fa65cac52be9c1889408ba2d3ddeac9af8c944513905275280c9e8ac5fb6c83a7100137173c3e6a402857cb4c6de00000000"
+++

First things first, let's update the `parse_tx_bytes` method to be able to handle segwit (any version) transactions.

+++ taproot-w020 python-vm
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
+++

+++ taproot-w021 python-vm
from codes import parse_compact_size, parse_input_bytes, parse_output_bytes

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

+++ taproot-w022 python-vm
tx_bytes = bytes.fromhex(tx)
ptr = 4
print(tx_bytes.hex())
print(tx_bytes[ptr:].hex())
+++

+++ taproot-w023 python-vm
pprint(parse_tx_bytes_mine(tx))
+++

+++ taproot-w024 python-vm
txdata = parse_tx_bytes_mine(tx)
+++

+++ taproot-w025 python-vm
assert len(txdata['witnesses'][0]) == 1
+++

+++ taproot-w026 python-vm
print(txdata['witnesses'][0][0].hex())
+++

+++ taproot-w027 python-vm
sig = txdata['witnesses'][0][0]
+++

+++ taproot-w028 python-vm
print(sig.hex())
+++

+++ taproot-w029 python-vm
print(len(sig))
+++

## Validating a Signature

What message did this signature sign?

Finding the "sighash" of that message.

Take the bitcoin transaction that the signature was embedded into. From this transaction, we'll create a hash.

That hash of the bitcoin transaction "message digest" which is the hash that the signature "signed".

+++ taproot-w031 python-vm
print(tx)
+++

+++ taproot-w032 python-vm
pprint(txdata)
+++

## Find the SigMsg for a transaction

Here we use the BIP341 Signature Validation Rules (https://github.com/bitcoin/bips/blob/master/bip-0341.mediawiki#user-content-Common_signature_message) Common signature message definition to calculate the SIGHASH_DEFAULT for a given transaction.

We'll use this to compute the sighash for this input, which we need to validate the signature.

+++ taproot-w034 python-vm
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

+++ taproot-w035 python-vm
spent_from_tx = parse_tx_bytes_mine("02000000000101d79ca7455775b3fc7bf65eae536faa22e6248b863247cee30765fabb4fafadb80100000000ffffffff02dcab0e00000000002251203905d81b457b42e8d5eb1ae5280c83c74fbdd20a2e401c0077bce5417909c953944be70400000000225120dc2e293b7717ece7898fefc8fd7f6da13e8fb56efac0bc3bd75183304da6ad1701409dab8ddd9c703d1acb6736bc5260b23e64999cb130b9b3ca6c2e69aa084073302b66f0ecb5b4a191618951c022da2c429ac3638613c43bbb88a761634c2cb85300000000")

print(spent_from_tx['outputs'][0])
+++

+++ taproot-w036 python-vm
amounts_bytes = [spent_from_tx['outputs'][0]['amount']]
print("amounts:", [x.hex() for x in amounts_bytes])

scriptpubkeys_bytes = [spent_from_tx['outputs'][0]['scriptPubKey']]
print("scriptpubkeys:", [x.hex() for x in scriptpubkeys_bytes], "\n")

ext_flag = 0
annex_bytes = None
input_index = 0

sigmsg = sigmsg_default(tx, input_index, annex_bytes, amounts_bytes, scriptpubkeys_bytes, ext_flag)

print("sigmsg:", sigmsg.hex())
+++

## Computing the Sighash

Now that we have the sighash message, we can calculate the actual "sighash" for the signature.

The `sighash` for Taproot key path spending signature valdiation is as follows:

```
sighash = tag_hash(b'TapSighash', bytes([0x00]) + sigmsg)
```

We'll need to implement the `tag_hash` function, then pass our `sigmsg` into this to find the `sighash`.

## Implement the tagged hash

???
id: taproot-w038
runtime: python-vm
Implement the tagged hash used by Taproot and Schnorr signatures.
---
from hashlib import sha256

def tag_hash(tag, data_bytes):
    # Hash the tag, then hash taghash || taghash || data_bytes.
    return b''
---
from hashlib import sha256
for tag, data in [(b'TapSighash', b''), (b'TapLeaf', b'\xc0\x01\x51'), (b'BIP0340/challenge', bytes(range(64)))]:
    tag_digest = sha256(tag).digest()
    assert tag_hash(tag, data) == sha256(tag_digest + tag_digest + data).digest(), 'Hash the tag twice before the message.'
---
Review the input bytes and try again.
???

+++ taproot-w039 python-vm
sighash = tag_hash(b'TapSighash', bytes([0x00]) + sigmsg)
print(sighash.hex())
+++

## Verifying the Sighash

Now that we've got the `sighash` we're finally ready to verify the signature.

For this, we'll need the `verify` function, which is defined in BIP340.

+++ taproot-w041 python-vm
# there are three inputs to the verify function: pubkey, message, signature
# Use the key in this transaction's spent output, even if you explored a
# different address in the previous chapter.
x_val = int.from_bytes(scriptpubkeys_bytes[input_index][2:], "big")
pk = x_val
m = sighash
sig = sig

print(x_val)
+++

+++ taproot-w042 python-vm
from codes import n

def verify(x_val, m, sig):
    P = lift_x(x_val)
    assert len(sig) == 64
    r = int.from_bytes(sig[:32], 'big')
    if r >= p:
        return False
    s = int.from_bytes(sig[32:], 'big')
    if s >= n:
        return False

    P_bytes = (x_val).to_bytes(32, 'big')
    challenge_hash = tag_hash(b'BIP0340/challenge', sig[:32] + P_bytes + m)
    e = int.from_bytes(challenge_hash, 'big') % n

    # e * P
    eP = P.multiply(e.to_bytes(32, 'big'))
    # s * G
    sG = coincurve.PrivateKey.from_int(s).public_key
    neg_eP = invert_point(eP)

    # R = sG - eP = sG + -eP
    R = coincurve.PublicKey.combine_keys([sG, neg_eP])

    if not has_even_y(R):
        return False

    return R.point()[0] == r
+++

+++ taproot-w043 python-vm
def invert_point(pt):
    x, y = pt.point()
    yneg = p - y
    return coincurve.PublicKey.from_point(x, yneg)
+++

+++ taproot-w044 python-vm
def has_even_y(pt):
    return pt.point()[1] % 2 == 0
+++

+++ taproot-w045 python-vm
sig = txdata['witnesses'][0][0]
verify(x_val, m, sig)
+++

## Signing a message

Now that we've been able to successfully verify a message, let's finish implementing the second method for Schnorr: the Sign method.

This takes a private key and a message and returns a valid signature.

```
sig = Sign(privkey, msg)
```

+++ taproot-w047 python-vm
def scalar_to_pubkey(scalar_val):
    assert scalar_val != 0 and scalar_val < n
    P = coincurve.PrivateKey.from_int(scalar_val).public_key
    result = scalar_val if has_even_y(P) else n - scalar_val

    return result, P


def sign_msg(privkey_int, msg, nonce_int):
    d, P = scalar_to_pubkey(privkey_int)
    k, R = scalar_to_pubkey(nonce_int)

    bytes_R = (R.point()[0].to_bytes(32, 'big'))
    bytes_P = (P.point()[0].to_bytes(32, 'big'))
    challenge_hash = tag_hash(b'BIP0340/challenge', bytes_R + bytes_P + msg)
    e = int.from_bytes(challenge_hash, 'big') % n

    # Schnorr signature scalar
    s = (k + e * d) % n

    sig = bytes_R + s.to_bytes(32, 'big')
    assert verify(P.point()[0], msg, sig)
    return sig
+++

+++ taproot-w048 python-vm
privkey = 10
nonce = 5
sig = sign_msg(privkey, m, nonce)

print(sig.hex())
assert coincurve.PublicKeyXOnly.from_secret(privkey.to_bytes(32, "big")).verify(sig, m)
+++

~~~
id: taproot-default-signature
How many bytes does a Schnorr signature using SIGHASH_DEFAULT contain?
= 64 [The signature contains two 32-byte values, r and s.]
- 65 [A non-default sighash flag adds a byte.]
- 32 [That is the size of an x-only public key.]
~~~

::: references
1. [BIP 340 — Schnorr signatures](https://github.com/bitcoin/bips/blob/master/bip-0340.mediawiki)

    The signing and verification algorithms.

2. [BIP 341 — Signature validation rules](https://github.com/bitcoin/bips/blob/master/bip-0341.mediawiki#common-signature-message)

    The common signature message and TapSighash.
:::
