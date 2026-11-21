# Build and send an onion

The original gossip notebook finishes by building an onion and calling `sendonion`. Here we complete that experiment with a payable invoice on a local network: **Alice → Bob → Carol**.

Run the previous lesson first. This lesson reuses its `make_route`, `payload`, and `make_onion_hops` functions. Your Python variables persist between lessons in the same course attempt.

## Start the three nodes

Each node has its own directory and RPC socket. Connections stay on loopback inside the VM. Start each node separately so its startup is saved as its own checkpoint.

+++ onion-start-bitcoin python-vm
from lab import bitcoin, lightning, start_bitcoin, start_node, fund_nodes, open_channel, confirm_channels, sync_nodes
start_bitcoin()
+++

+++ onion-start-alice python-vm
start_node("alice")
+++

+++ onion-start-bob python-vm
start_node("bob")
+++

+++ onion-start-carol python-vm
start_node("carol")
+++

## Fund and connect

Give each node a confirmed Bitcoin output. Alice funds the first channel, and Bob funds the second, so the initial balances support a payment toward Carol. The helpers reuse existing nodes and channels when you rerun them.

+++ onion-fund-nodes python-vm
fund_nodes()
+++

+++ onion-channel-alice-bob python-vm
open_channel("alice", "bob")
+++

+++ onion-channel-bob-carol python-vm
open_channel("bob", "carol")
+++

+++ onion-confirm-channels python-vm
confirm_channels()
+++

## Inspect the two forwarding directions

Private channels are sufficient: each node knows its own channels. Read each source’s local policy from `listpeerchannels` and convert it to the half-channel structure used in the historical graph. A private channel can use a short-channel-ID alias; prefer the source’s local alias for its outgoing hop.

+++ onion-local-policies python-vm
def local_half_channel(source, destination):
    source_id = lightning(source, "getinfo")["id"]
    destination_id = lightning(destination, "getinfo")["id"]
    channel = next(c for c in lightning(source, "listpeerchannels")["channels"] if c["peer_id"] == destination_id)
    scid = channel.get("alias", {}).get("local", channel["short_channel_id"])
    policy = channel["updates"]["local"]
    return {"source": source_id, "destination": destination_id, "short_channel_id": scid,
            "disabled": not channel["peer_connected"] or channel["state"] != "CHANNELD_NORMAL",
            "satoshis": channel["total_msat"] // 1000,
            "htlc_minimum_msat": policy["htlc_minimum_msat"], "htlc_maximum_msat": policy["htlc_maximum_msat"],
            "fee_base_msat": policy["fee_base_msat"], "fee_proportional_millionths": policy["fee_proportional_millionths"],
            "cltv_expiry_delta": policy["cltv_expiry_delta"]}

local_edges = [local_half_channel("alice", "bob"), local_half_channel("bob", "carol")]
local_edges
+++

## Create Carol’s invoice

Generate a fresh label for each attempt. Ask Carol for a 10,000-satoshi invoice and decode it with Core Lightning. Compare its fields with the invoice parser from the earlier lesson.

+++ onion-invoice python-vm
from uuid import uuid4

invoice_label = "workshop-" + uuid4().hex
created_invoice = lightning("carol", "invoice", amount_msat=10_000_000,
                            label=invoice_label, description="My first hand-built onion")
local_invoice = lightning("alice", "decode", string=created_invoice["bolt11"])
assert local_invoice["valid"] and local_invoice["type"] == "bolt11 invoice", local_invoice
print({key: local_invoice[key] for key in ("payee", "amount_msat", "payment_hash", "payment_secret", "min_final_cltv_expiry")})
+++

## Build the onion

The payloads use **absolute block heights**. The first-hop instruction passed to `sendonion` uses a **relative delay**. Do not mine extra blocks between constructing and sending the onion.

The associated data must be the invoice’s **payment hash**. The payment secret goes inside the final payload’s payment-data field.

+++ onion-create python-vm
sync_nodes()
local_route = make_route(local_invoice["amount_msat"], local_invoice["min_final_cltv_expiry"], local_edges)
onion_height = lightning("alice", "getinfo")["blockheight"]
onion_hops = make_onion_hops(local_route, onion_height, local_invoice["payment_secret"])
onion = lightning("alice", "createonion", hops=onion_hops, assocdata=local_invoice["payment_hash"])
assert len(bytes.fromhex(onion["onion"])) == 1366
print("Two payloads:", onion_hops)
print("Onion packet: 1 version byte + 33-byte key + 1300-byte routing data + 32-byte HMAC")
+++

## Send and settle

Sending initiates an HTLC. `waitsendpay` then waits for a success or failure result. On success, verify that the returned preimage hashes to the invoice’s payment hash.

+++ onion-send python-vm
from hashlib import sha256

assert lightning("alice", "getinfo")["blockheight"] == onion_height, "Height changed: rebuild the onion"
sent = lightning("alice", "sendonion", onion=onion["onion"], first_hop=local_route[0],
                 payment_hash=local_invoice["payment_hash"], shared_secrets=onion["shared_secrets"],
                 bolt11=created_invoice["bolt11"], amount_msat=local_invoice["amount_msat"],
                 destination=local_invoice["payee"])
settled = lightning("alice", "waitsendpay", payment_hash=local_invoice["payment_hash"], timeout=30)
assert settled["status"] == "complete", settled
assert sha256(bytes.fromhex(settled["payment_preimage"])).hexdigest() == local_invoice["payment_hash"]
print("Payment complete:", settled)
+++

+++ onion-check-recipient python-vm
received = lightning("carol", "listinvoices", label=invoice_label)["invoices"][0]
assert received["status"] == "paid", received
print("Carol received:", received["amount_received_msat"], "msat")
+++

Try Undo on the recipient check, then Undo on the send cell. Inspect Carol’s invoice again: the whole VM, including all three nodes, returns to the earlier state. Rebuild and send the onion to repeat the experiment.

~~~
id: onion-associated-data
Which invoice field must the payment onion’s associated data match?
= The payment hash. [This binds the onion to the HTLC’s payment hash.]
- The payment secret. [The secret belongs in the final hop’s payment-data TLV.]
- The recovered signing key. [The public key identifies the recipient; it is not the associated data.]
~~~

This lesson retains `sendonion` to match the original notebook’s exercise.

::: references
1. [Core Lightning — createonion](https://docs.corelightning.org/reference/createonion)

   Builds a custom onion from each hop's public key and payload, binding it to the payment hash through its associated data.

2. [Core Lightning — sendonion](https://docs.corelightning.org/reference/sendonion)

   Sends a payment using your assembled onion packet and first-hop instructions. Documents the arguments used in the send cell above.
:::
