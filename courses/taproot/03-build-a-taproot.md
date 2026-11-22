# Build a Taproot output

We've just done a Taproot keypath spend verification (~P2PK)

Now we're going to look at how to build out scripts for taproot!

We're going to make our first taproot script. (~P2WSH)

What are the steps we need to do for this:

- figure out what scripts can spend our bitcoin
- make a tree out of them (just like christmas!)
- build a merkle root
- pick a internal pubkey
- make a tweak from the merkle root
- add the tweak to the internal pubkey to get our external pubkey
- lock money up to external pubkey

## Step 1: Figure out what scripts can spend our bitcoin

- Everyone in class should make a script!
- Each individually lock up coins to the script (single script spend)
- Send all the scripts to Lisa, and then we'll build a tree out of them

#### Writing a taproot script

nifty's script: OP_4 OP_ADD OP_8 OP_EQUAL  (requires OP_4 to unlock)

Convert to script:
        OP_4 OP_ADD OP_8 OP_EQUAL
        54935887

+++ taproot-w051 python-vm
nifty = '54935887'
c_nym =  'AA201ccf6c5e6212f524600fb6b20275cd6ae26dc6b812cd1f84dd3ff7d86b1937d187'
j_nym = '54935887'
d_nym = '930200088763a820150faa5b485225f681b179f710cb169b92b401f954392eb30e677624135233f08768'
m_nym = '76769393010987'
da_nym = 'a820528f99cd13f2d438556c6f6a803458a963519d2419a40dd685597c0919d81f588754935587'
ch_nym = '53935987'
n_nym = 'a82041ef4bb0b23661e66301aac36066912dac037827b4ae63a7b1165a5aa93ed4eb87'
+++

+++ taproot-w052 python-vm
scripts = [('nifty', nifty), ('c_nym', c_nym), ('j_nym', j_nym), ('d_nym', d_nym), ('m_nym', m_nym), ('da_nym', da_nym), ('ch_nym', ch_nym), ('n_nym', n_nym)]
+++

## Step 2: Make a tree out of your scripts

For this first round, we're just going to use a single script!

Every tree has leaves! Every script in our tree is a leaf.

Our tree is just [nifty]

+++ taproot-w054 python-vm
tree = [nifty]
+++

## Build a Merkle root

### Write a function to compute a leaf hash

???
id: taproot-w057
runtime: python-vm
Write a function that computes a TapLeaf hash from a script.
---
from codes import size_compact_size

def make_leaf(script_bytes):
    # Commit to the leaf version, CompactSize length, and script bytes.
    return b''
---
from hashlib import sha256
for script, length in [(b'', b'\x00'), (bytes.fromhex('54935887'), b'\x04'), (b'\x51' * 253, b'\xfd\xfd\x00')]:
    tag = sha256(b'TapLeaf').digest()
    expected = sha256(tag + tag + b'\xc0' + length + script).digest()
    assert make_leaf(script) == expected, 'Include 0xc0 and the CompactSize script length.'
---
Review the input bytes and try again.
???

## Hash the class scripts

+++ taproot-w058 python-vm
tree_bytes = [ bytes.fromhex(x) for x in tree ]
+++

+++ taproot-w059 python-vm
from codes import size_compact_size
leaf = make_leaf(tree_bytes[0])
print(leaf.hex())
+++

+++ taproot-w060 python-vm
for name, script in [ (a,bytes.fromhex(x)) for a,x in scripts]:
    int_leaf = make_leaf(script)
    print(name, int_leaf.hex())
+++

In this first example, where we only have one script, the "merkle root" is the leaf of the single script.

Our root = leaf.

+++ taproot-w062 python-vm
root = leaf
print(leaf.hex())
+++

## Step 4: Pick an internal pubkey

Considerations to make when picking an *internal* pubkey:

- Does it need to be spendable?
- If yes, who has the keys to it?
- If no, how can I prove it's unspendable?

In our case, our key needs to be unspendable. We need to pick a point that we don't know the private key to.

The spec (BIP341) recommends using the following algorithm (or something like it) to pick a point where the private key is unknowns.

> One example of such a point is H = lift_x(0x50929b74c1a04954b78b4b6035e97a5e078a5a0f28ec96d547bfee9ace803ac0) which is constructed by taking the hash of the standard uncompressed encoding of the secp256k1 base point G as X coordinate. In order to avoid leaking the information that key path spending is not possible it is recommended to pick a fresh integer r in the range 0...n-1 uniformly at random and use H + rG as internal key. It is possible to prove that this internal key does not have a known discrete logarithm with respect to G by revealing r to a verifier who can then reconstruct how the internal key was created.

+++ taproot-w065 python-vm
# We define the nums_point as
# H(G) => sha256(uncompressed)
def nums_point(r=0):
    Gx, Gy = coincurve.PrivateKey.from_int(1).public_key.point()
    data = bytes([0x04]) + Gx.to_bytes(32, 'big') + Gy.to_bytes(32, 'big')
    h = sha256(data).digest()
    h_int = int.from_bytes(h, 'big')
    H = lift_x(h_int)
    if r > 0:
        assert r < n
        rG = coincurve.PrivateKey.from_int(r).public_key
        return coincurve.PublicKey.combine_keys([rG, H])
    return H
+++

+++ taproot-w066 python-vm
internal_pubkey = nums_point()
print(internal_pubkey.format().hex())

pubkey_bytes = internal_pubkey.point()[0].to_bytes(32, 'big')
+++

## Step 5: Make a tweak from the Merkle Root

What is a tweak? Ideally, a tweak is really just a number (scalar).

We're gonna take the root and a pubkey, make a hash out of both of them and then convert this hash to a number.

We'll then use that number (scalar) as if it were a private key and find the curve point at that value.

+++ taproot-w068 python-vm
def make_tweak_pubkey(pubkey, merkle_root_bytes):
    # Find the bytes for the x-value of the internal pubkey
    pubkey_bytes = pubkey.point()[0].to_bytes(32, 'big')

    # Compute the tweak hash (TapTweak tag + pubkey + merkle root)
    tweak_bytes = tag_hash(b'TapTweak', pubkey_bytes + merkle_root_bytes)
    # Convert that hash into a scalar (so we can make it a point)
    tweak_int = int.from_bytes(tweak_bytes, 'big')
    # Make sure our tweak is inside the "point range (n)"
    assert tweak_int < n

    # Multiply the tweak by the Generator Point (G) which gives us T
    T = coincurve.PrivateKey.from_int(tweak_int).public_key

    return T
+++

+++ taproot-w069 python-vm
T = make_tweak_pubkey(internal_pubkey, root)
print(T.format().hex())
+++

+++ taproot-w070 python-vm
for name, script in [ (a,bytes.fromhex(x)) for a,x in scripts]:
    root = make_leaf(script)
    T = make_tweak_pubkey(internal_pubkey, root)
    print(name, T.format().hex())
+++

Now we have our Tweak as a point. All we need to do is combine it with the internal pubkey to get an external pubkey.

## Step 6: Add Internal Pubkey to Tweak Pubkey

For some reason the spec makes everything more fancy that it needs to be.

The biggest difference is that we "promote" the internal pubkey to a x-only pubkey (even) before we add it to the tweak.

+++ taproot-w073 python-vm
def make_external_pubkey(pubkey, T):
    P = lift_x(pubkey.point()[0])
    Q = coincurve.PublicKey.combine_keys([P, T])
    return Q
+++

+++ taproot-w074 python-vm
nifty

nifty_leaf = make_leaf(bytes.fromhex(nifty))
print(internal_pubkey.format().hex())
print('nifty root:', nifty_leaf.hex())
nifty_T = make_tweak_pubkey(internal_pubkey, nifty_leaf)
print('nifty tweak', nifty_T.format().hex())
nifty_Q = make_external_pubkey(internal_pubkey, nifty_T)
print('nifty!', nifty_Q.format().hex())
+++

+++ taproot-w075 python-vm
for name, script in [ (a, bytes.fromhex(x)) for a,x in scripts]:
    root = make_leaf(script)
    T = make_tweak_pubkey(internal_pubkey, root)
    Q = make_external_pubkey(internal_pubkey, T)
    print(name, Q.format().hex())
+++

## Step 7: Lock bitcoin up to your external pubkey! (but first make a P2TR script)

We'll take Q, drop the parity byte at the front and make this into a P2TR script.

+++ taproot-w077 python-vm
def make_p2tr(Q):
    Qx_bytes = Q.point()[0].to_bytes(32, 'big')
    witness_version = 0x51
    return bytes([witness_version]) + size_compact_size(len(Qx_bytes)) + Qx_bytes


print(make_p2tr(nifty_Q).hex())
nifty_script = make_p2tr(nifty_Q).hex()
+++

## Fund the output

+++ taproot-w078 python-vm
address = bitcoin("decodescript", nifty_script)["address"]
address
+++

+++ taproot-w079 python-vm
bitcoin("listwallets", wallet=False)
+++

+++ taproot-w080 python-vm
mine(1)
+++

+++ taproot-w081 python-vm
single_txid = bitcoin("sendtoaddress", address, 1.0)
single_vout = find_output(single_txid, nifty_script)
single_txid, single_vout
+++

+++ taproot-w082 python-vm
bitcoin("getrawtransaction", single_txid, True)
+++

~~~
id: taproot-nums-key
Why choose a NUMS internal key for these script-only examples?
= No participant knows its private key. [Spending requires satisfying one of the committed scripts.]
- It removes the need for a Merkle root. [The output still commits to the script tree.]
- Its private key is zero. [A NUMS point has an unknown discrete logarithm.]
~~~

::: references
1. [BIP 341 — Constructing and spending Taproot outputs](https://github.com/bitcoin/bips/blob/master/bip-0341.mediawiki)

    TapLeaf, TapBranch, TapTweak, and internal keys.
:::
