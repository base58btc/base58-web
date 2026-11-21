# Lightning invoices

What information do you need to make a Lightning payment?

A BOLT 11 invoice puts that information in one string. Earlier Bitcoin payment formats include BIP 21 payment URIs and the BIP 70 payment protocol.

Lightning payments exchange money for a preimage. The invoice commits to that preimage with a **payment hash**. The separately named **payment secret** helps the recipient recognize an intended payment; it is not the preimage.

## Information in an invoice

| Field | Meaning |
| --- | --- |
| Human-readable amount | Amount requested, or no fixed amount |
| `p` | Payment hash |
| `s` | Payment secret |
| `n` | Payee public key, when explicitly included; otherwise recover it from the signature |
| `c` | Minimum final CLTV delta, default 18 |
| `x` | Expiry in seconds after the timestamp, default 3600 |
| `d` or `h` | Description or description hash |
| `f` | On-chain fallback address |
| `r` | Route hints |
| `m` | Payment metadata |
| `9` | Feature bits |

This lesson covers BOLT 11. BOLT 12 uses a different encoding and can carry blinded paths; it is not decoded by this parser.

## The human-readable part

`lnbc2500u` means a Lightning invoice on Bitcoin mainnet for 2500 microbitcoin. Convert amounts to **millisatoshis**, the unit used in Lightning’s payment messages.

+++ invoice-amounts python-vm
import re

def parse_amt(text):
    if not text:
        return None
    match = re.fullmatch(r"([0-9]+)([munp]?)", text)
    if not match:
        raise ValueError("Invalid invoice amount")
    number, suffix = int(match[1]), match[2]
    if number == 0:
        raise ValueError("Invoice amount must be positive")
    if suffix == "p":
        if number % 10:
            raise ValueError("Pico-BTC amount must be a whole millisatoshi")
        return number // 10
    return number * {"": 10**11, "m": 10**8, "u": 10**5, "n": 100}[suffix]

def parse_hrp(hrp):
    if not hrp.startswith("ln"):
        raise ValueError("Missing ln prefix")
    for network in ("bcrt", "tbs", "bc", "tb"):
        if hrp[2:].startswith(network):
            return network, parse_amt(hrp[2 + len(network):])
    raise ValueError("Unknown network")

assert parse_hrp("lnbc2500u") == ("bc", 250_000_000)
assert parse_hrp("lntb") == ("tb", None)
assert parse_amt("10p") == 1
parse_hrp("lnbcrt100n")
+++

## Bech32: five-bit words

The part after the final separator `1` contains a timestamp, tagged fields, signature, and checksum. The timestamp takes seven five-bit words. The recoverable signature takes 104 words, or 65 bytes. Unlike Bech32 addresses, invoices can exceed 90 characters.

+++ invoice-bech32 python-vm
CHARSET = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

def bech32_decode(text):
    if text.lower() != text and text.upper() != text:
        raise ValueError("Mixed-case Bech32")
    text = text.lower()
    separator = text.rfind("1")
    if separator < 1 or separator + 7 > len(text):
        raise ValueError("Invalid separator or checksum length")
    hrp = text[:separator]
    if any(not 33 <= ord(c) <= 126 for c in hrp):
        raise ValueError("Invalid human-readable character")
    words = [CHARSET.index(c) for c in text[separator + 1:]]
    expanded = [ord(c) >> 5 for c in hrp] + [0] + [ord(c) & 31 for c in hrp]
    checksum = 1
    generators = (0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3)
    for word in expanded + words:
        top = checksum >> 25
        checksum = ((checksum & 0x1ffffff) << 5) ^ word
        for bit, generator in enumerate(generators):
            if (top >> bit) & 1:
                checksum ^= generator
    if checksum != 1:
        raise ValueError("Invalid BOLT 11 Bech32 checksum")
    return hrp, words[:-6]

def words_to_bytes(words, pad=False):
    accumulator, bits, result = 0, 0, bytearray()
    for word in words:
        if not 0 <= word < 32:
            raise ValueError("Not a five-bit word")
        accumulator = ((accumulator << 5) | word) & 0xfff
        bits += 5
        if bits >= 8:
            bits -= 8
            result.append((accumulator >> bits) & 255)
    if pad and bits:
        result.append((accumulator << (8 - bits)) & 255)
    elif not pad and (bits >= 5 or ((accumulator << (8 - bits)) & 255)):
        raise ValueError("Invalid padding")
    return bytes(result)

def parse_number(words):
    number = 0
    for word in words:
        number = (number << 5) | word
    return number
+++

## Recover the signing key

Hash the human-readable part together with the packed data before the signature. The final signature byte is the recovery ID. `coincurve` performs secp256k1 public-key recovery.

+++ invoice-signature python-vm
from hashlib import sha256
from coincurve import PublicKey

def validate_signature(hrp, words):
    signature = words_to_bytes(words[-104:])
    if len(signature) != 65 or signature[-1] not in range(4):
        raise ValueError("Invalid recoverable signature")
    digest = sha256(hrp.encode() + words_to_bytes(words[:-104], pad=True)).digest()
    return PublicKey.from_signature_and_message(signature, digest, hasher=None).format().hex()
+++

## Read the tagged fields

Each tag starts with a five-bit type and a ten-bit length. The length counts five-bit words. Unknown tags are skipped so later extensions do not break decoding. Route hints and fallback addresses remain raw words in this teaching parser; we are not implementing a wallet’s complete payment validation policy.

+++ invoice-tags python-vm
def parse_tagged_parts(words, invoice):
    invoice.update(expiry=3600, final_cltv_expiry_delta=18)
    offset, seen = 0, set()
    while offset < len(words):
        if offset + 3 > len(words):
            raise ValueError("Truncated tag header")
        tag = CHARSET[words[offset]]
        length = parse_number(words[offset + 1:offset + 3])
        offset += 3
        data = words[offset:offset + length]
        offset += length
        if len(data) != length:
            raise ValueError("Truncated tag data")
        if tag in "psndhxc9m":
            if tag in seen:
                raise ValueError(f"Duplicate {tag} tag in this teaching parser")
            seen.add(tag)
        if tag in "psh":
            if length != 52:
                raise ValueError("Expected a 32-byte field")
            invoice[{"p": "payment_hash", "s": "payment_secret", "h": "description_hash"}[tag]] = words_to_bytes(data).hex()
        elif tag == "d":
            invoice["description"] = words_to_bytes(data).decode("utf-8")
        elif tag == "n":
            if length != 53 or words_to_bytes(data).hex() != invoice["pubkey"]:
                raise ValueError("Explicit payee does not match recovered signing key")
        elif tag in "xc9":
            invoice[{"x": "expiry", "c": "final_cltv_expiry_delta", "9": "feature_bits"}[tag]] = parse_number(data)
        elif tag == "m":
            invoice["metadata"] = words_to_bytes(data).hex()
        else:
            invoice.setdefault("raw_tags", []).append({"tag": tag, "words": data})
    if "p" not in seen or ("d" in seen) == ("h" in seen):
        raise ValueError("Expected payment hash and exactly one description field")
    return invoice

def parse_invoice(text):
    hrp, words = bech32_decode(text)
    if len(words) < 7 + 104:
        raise ValueError("Invoice is too short")
    network, amount = parse_hrp(hrp)
    invoice = {"network": network, "amt_msat": amount, "timestamp": parse_number(words[:7]),
               "pubkey": validate_signature(hrp, words)}
    return parse_tagged_parts(words[7:-104], invoice)
+++

## Decode the notebook’s examples

The examples are in `data/invoices.json`. These are historical test invoices; decoding a signature does not mean an invoice is current or payable.

+++ decode-invoices python-vm
import json
from pathlib import Path

examples = json.loads(Path("data/invoices.json").read_text())
invoice = parse_invoice(examples["2500u"])
assert invoice["amt_msat"] == 250_000_000
assert invoice["payment_hash"] == "0001020304050607080900010203040506070809000102030405060708090102"
print(json.dumps(invoice, indent=2))
print("Second example amount:", parse_invoice(examples["20m"])["amt_msat"])
+++

Experiment: alter one character in the encoded invoice and decode it again. Which check fails first? Restore the original before continuing.

~~~
id: secret-versus-preimage
Does the invoice’s payment secret reveal the preimage of its payment hash?
= No. They serve different purposes. [The recipient releases the preimage when settling the payment.]
- Yes. You can hash the payment secret to obtain the payment hash. [A payment secret is a separate value.]
~~~

::: references
1. [BOLT 11 — Invoice encoding](https://github.com/lightning/bolts/blob/master/11-payment-encoding.md)

    Defines the Bech32 invoice format, including amounts, tagged fields, checksums, and signatures.
:::
