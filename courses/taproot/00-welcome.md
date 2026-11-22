# Taproot Intensive

Taproot is a series of Bitcoin Improvement Proposals activated in November 2021.

## What did Taproot change?

Taproot added and modified many things in Bitcoin, including:

- A new signature algorithm (Schnorr).
- A new witness version and script rules (SegWit v1 and Tapscript).
- A new address checksum (Bech32m).
- Removal of OP_CHECKMULTISIG in Tapscript.
- A new opcode, OP_CHECKSIGADD.
- Script trees and a key-path spending option.

The examples use public demonstration keys and regtest coins. The workshop signing implementations are for learning.

1. [Finding Taproot in the wild](01-finding-taproot)
2. [Schnorr signatures](02-schnorr-signatures)
3. [Build a Taproot output](03-build-a-taproot)
4. [Spend a Tapscript](04-spend-a-tapscript)
5. [Balanced and unbalanced script trees](05-script-trees)
6. [Multisig with OP_CHECKSIGADD](06-checksigadd)
7. [MuSig2 and Tapscript spends](07-musig2)
8. [FROST threshold signatures](08-frost)

::: references
1. [BIP 340 — Schnorr signatures](https://github.com/bitcoin/bips/blob/master/bip-0340.mediawiki)

    X-only public keys, signing, and verification.

2. [BIP 341 — Taproot](https://github.com/bitcoin/bips/blob/master/bip-0341.mediawiki)

    Output keys, script trees, and spending rules.

3. [BIP 342 — Tapscript](https://github.com/bitcoin/bips/blob/master/bip-0342.mediawiki)

    Taproot script validation and OP_CHECKSIGADD.

4. [BIP 350 — Bech32m](https://github.com/bitcoin/bips/blob/master/bip-0350.mediawiki)

    The address checksum used for witness version 1.
:::
