# Finding Taproot in the wild

Let's go to mempool.space and find some taproot addresses in the wild!

## Start a regtest wallet

+++ taproot-start python-vm
from lab import bitcoin, start_bitcoin, mine, find_output

start_bitcoin()
+++

## How to spot a taproot address

The bech32 address has a `p` immediately following the `1`

Find a taproot address on mempool dot space.

What's the "Type" for this address?


Copy paste the "ScriptPubKey (HEX)" for the address you found into the pyblock below.

+++ taproot-w003 python-vm
scriptpubkey = "51203905d81b457b42e8d5eb1ae5280c83c74fbdd20a2e401c0077bce5417909c953"
+++

+++ taproot-w004 python-vm
print(scriptpubkey)
+++

+++ taproot-w005 python-vm
len(scriptpubkey) // 2
+++

This is a series of opcodes. If you pass this to `bitcoin-cli decodescript` what does it print back as the asm?

+++ taproot-w007 python-vm
bitcoin("decodescript", scriptpubkey)
+++

+++ taproot-w008 python-vm
output = bitcoin("decodescript", scriptpubkey)
output
+++

+++ taproot-w009 python-vm
print(output['asm'])
+++

+++ taproot-w010 python-vm
output['asm'].split(" ")
+++

+++ taproot-w011 python-vm
len(output['asm'].split(" ")[1]) // 2
+++

+++ taproot-w012 python-vm
print(output['type'])
+++

+++ taproot-w013 python-vm
x_bytes = bytes.fromhex(output['asm'].split(' ')[1])
print('hex:\t\t', x_bytes.hex())

x_val = int.from_bytes(x_bytes, 'big')
print('int val:\t', x_val)
+++

## Finding the secp256k1 pubkey from a P2TR address

Now that we have the x-only pubkey, let's figure out what point that is on the secp256k1 curve.

We can do that using `lift_x_pubkey`. This takes an integer and returns a point.

In my case, I'll use `coincurve`, a python library for expressing + doing math with public keys.

+++ taproot-w015 python-vm
p = 115792089237316195423570985008687907853269984665640564039457584007908834671663
def lift_x(x_value):
    assert 0 <= x_value < p
    # find y squared (y2 = x^3 + 7)
    y2 = (x_value ** 3 + 7) % p
    # find the square root of y
    y = pow(y2, (p+1)//4, p)
    assert (y ** 2 % p) == y2
    # if y is odd, find even y
    if y % 2 != 0:
        y = p - y
    return coincurve.PublicKey.from_point(x_value, y)
+++

+++ taproot-w016 python-vm
import coincurve

point = lift_x(x_val)
print(point.format().hex())
print(point.point())
+++

~~~
id: taproot-address-type
What type does Bitcoin Core report for this P2TR output?
= witness_v1_taproot [It is a version-1 witness program containing a 32-byte output key.]
- witness_v0_keyhash [That is a version-0 key-hash output.]
- scripthash [P2SH uses a different output script.]
~~~
