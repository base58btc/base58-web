# Revocation generation

Let’s implement the per-commitment secret generator from BOLT 3. Starting with a 32-byte seed and a 48-bit index, visit the bits from 47 down to 0. For each set bit, flip that bit in the current secret and then hash the result.

```text
P = seed
for B from 47 down to 0:
    if bit B is set in I:
        flip bit B in P
        P = SHA256(P)
return P
```

## Flip a bit

The bit number counts from the least-significant bit of the first byte. Python `bytes` are immutable, so the function returns a new value. Remember to use that return value.

+++ revocation-bit-flip python-vm
from hashlib import sha256

def flip(secret, bit):
    changed = bytearray(secret)
    changed[bit // 8] ^= 1 << (bit % 8)
    return bytes(changed)

assert flip(bytes(32), 0) == b"\x01" + bytes(31)
assert flip(bytes(32), 47) == bytes(5) + b"\x80" + bytes(26)
+++

## Generate a secret

Make the five test vectors below pass. Find and fix the error in this implementation. Index zero is valid: no bits are set, so its secret equals the seed. A channel starts allocating secrets from index `2**48 - 1` and counts down.

+++ revocation-generator python-vm
def generate_from_seed(seed, index):
    if len(seed) != 32 or not 0 <= index < 2**48:
        raise ValueError("Expected a 32-byte seed and a 48-bit index")
    secret = seed
    for bit in range(47, -1, -1):
        if index & (1 << bit):
            flip(secret, bit)
            secret = sha256(secret).digest()
    return secret

seed = bytes.fromhex("01" * 32)
assert generate_from_seed(seed, 0) == seed
generate_from_seed(seed, 1).hex()
+++

## Make the test vectors pass

+++ revocation-vectors python-vm
vectors = [
    ("00", 281474976710655, "02a40c85b6f28da08dfdbe0926c53fab2de6d28c10301f8f7c4073d5e42e3148"),
    ("ff", 281474976710655, "7cc854b54e3e0dcdb010d7a3fee464a9687be6e8db3be6854c475621e007a5dc"),
    ("ff", 0xaaaaaaaaaaa, "56f4008fb007ca9acf0e15b054d5c9fd12ee06cea347914ddbaed70d1c13a528"),
    ("ff", 0x555555555555, "9015daaeb06dba4ccc05b91b2f73bd54405f2be9f217fbacd3c5ac2e62327d31"),
    ("01", 1, "915c75942a26bb3a433a8ce2cb0427c29ec6c1775cfc78328b57f6ba7bfeaa9c"),
]
for byte, index, expected in vectors:
    actual = generate_from_seed(bytes.fromhex(byte * 32), index).hex()
    assert actual == expected, (byte, index, actual)
print("All five secret-generation vectors pass.")
+++

## Write the commitment number into a transaction

The commitment transaction carries an obscured 48-bit commitment number, split between `nSequence` and `nLockTime`. This increasing commitment number is distinct from the decreasing secret-generation index.

The obscurer is the last six bytes of `SHA256(opener_payment_basepoint || accepter_payment_basepoint)`. Ordering matters: these are the payment basepoints in opener/accepter order, not public keys sorted by value.

+++ commitment-basepoints python-vm
from coincurve import PrivateKey

opener_payment_basepoint = PrivateKey.from_int(666).public_key.format()
accepter_payment_basepoint = PrivateKey.from_int(777).public_key.format()
print("Opener:", opener_payment_basepoint.hex())
print("Accepter:", accepter_payment_basepoint.hex())
obscurer = int.from_bytes(sha256(opener_payment_basepoint + accepter_payment_basepoint).digest()[-6:], "big")
print(f"Obscurer: {obscurer:012x}")
+++

+++ commitment-fields python-vm
commitment_number = 42
obscured = commitment_number ^ obscurer
sequence = 0x80000000 | (obscured >> 24)
locktime = 0x20000000 | (obscured & 0xffffff)

print(f"nSequence: {sequence:08x}; transaction bytes: {sequence.to_bytes(4, 'little').hex()}")
print(f"nLockTime: {locktime:08x}; transaction bytes: {locktime.to_bytes(4, 'little').hex()}")
recovered = (((sequence & 0xffffff) << 24) | (locktime & 0xffffff)) ^ obscurer
assert recovered == commitment_number
print("Recovered commitment number:", recovered)
+++

Change `commitment_number` and rerun the final cell. Which bytes change? Why does setting the high bit of `nSequence` matter for relative locktime interpretation?

~~~
id: zero-secret-index
What does the secret generator return for index zero?
= The original seed. [No bits are set, so no flip or hash step runs.]
- SHA256 of the seed. [Hashing happens only after processing a set bit.]
- An error. [Zero is a valid 48-bit index.]
~~~

::: references
1. [BOLT 3 — Commitment transactions and secrets](https://github.com/lightning/bolts/blob/master/03-transactions.md)

    Specifies per-commitment secret generation and how commitment numbers are encoded in transaction fields.
:::
