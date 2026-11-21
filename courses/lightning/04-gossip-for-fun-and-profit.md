# Lightning gossip for fun and profit

A Lightning node learns about public channels from gossip. A channel announcement identifies the two endpoints. Each endpoint can publish a channel update describing its policy for forwarding in one direction. The notebook calls each direction a **half-channel**.

## Open the historical graph

The original November 2023 snapshot is about 175 MB. This browser edition includes a reproducible **64-node extract** containing the notebook’s example channel and a route between its original endpoints. It contains real historical policies, including disabled directions. It is not a current network map and does not reveal channel balances.

+++ load-gossip python-vm
import json
from pathlib import Path

graph = json.loads(Path("data/gossip.json").read_text())
edges = graph["edges"]
print(len(graph["nodes"]), "nodes;", len(edges), "directed channel updates")
print("Original store SHA256:", graph["provenance"]["sha256"])
example_channel = [edge for edge in edges if edge["short_channel_id"] == "803775x1091x1"]
example_channel
+++

The `satoshis` value is the funding capacity shared by the two directions. The advertised policy includes:

| Field | Meaning |
| --- | --- |
| `source`, `destination` | Direction of travel |
| `direction` | 0 from the smaller public key to the larger, otherwise 1 |
| `timestamp` | Time of this policy update |
| `disabled` | Whether this direction is disabled |
| `htlc_minimum_msat`, `htlc_maximum_msat` | Advertised amount bounds |
| `fee_base_msat`, `fee_proportional_millionths` | Base and proportional forwarding fees |
| `cltv_expiry_delta` | Additional incoming timelock required by the forwarding node |

+++ gossip-neighbors python-vm
my_source, my_destination = graph["source"], graph["destination"]

def neighbors(source):
    return sorted({edge["destination"] for edge in edges if edge["source"] == source and not edge["disabled"]})

print("Source:", my_source)
print("Destination:", my_destination)
neighbors(my_source)
+++

## Find a route

Use breadth-first search to find a path with the fewest hops. Keep the actual half-channels, not just node IDs: there can be multiple channels between two nodes. Track visited nodes so a disconnected destination terminates with a useful error.

This is a topology exercise. Fewest hops does not necessarily mean lowest fees or highest payment success probability.

+++ find-route python-vm
from collections import deque

def find_route(edges, source, destination):
    if source == destination:
        return []
    queue = deque([(source, [])])
    visited = {source}
    while queue:
        node, path = queue.popleft()
        for edge in edges:
            if edge["source"] != node or edge["disabled"]:
                continue
            neighbor = edge["destination"]
            if neighbor in visited:
                continue
            next_path = path + [edge]
            if neighbor == destination:
                return next_path
            visited.add(neighbor)
            queue.append((neighbor, next_path))
    raise ValueError("No enabled route exists in this graph")

half_channels = find_route(edges, my_source, my_destination)
for edge in half_channels:
    print(edge["source"], "->", edge["destination"], edge["short_channel_id"], "/", edge["direction"])
+++

Experiment: remove one selected direction from the graph and search again. Does an alternative path exist? What happens if the destination is absent?

## Can the payment fit?

We will deliver 10,000 satoshis, or 10,000,000 millisatoshis. **Every** direction in the route must be enabled. Bounds are inclusive. Funding capacity is only an upper bound: it cannot tell us how much balance is available on the sending side.

+++ route-constraints python-vm
payment_amount = 10_000 * 1000

def check_amount(edge, amount):
    if edge["disabled"]:
        raise ValueError("Channel direction is disabled")
    if amount < edge["htlc_minimum_msat"]:
        raise ValueError("Amount is below the advertised minimum")
    maximum = edge["htlc_maximum_msat"]
    if maximum is not None and amount > maximum:
        raise ValueError("Amount exceeds the advertised maximum")
    if edge["satoshis"] is not None and amount > edge["satoshis"] * 1000:
        raise ValueError("Amount exceeds the channel's total capacity")

for edge in half_channels:
    check_amount(edge, payment_amount)
print("The recipient amount fits the advertised bounds. We still need to add forwarding fees.")
+++

## What does the route cost?

A forwarding node charges `base_fee + floor(amount_to_forward * ppm / 1_000_000)`. Work backward from the recipient: each upstream hop must carry the payment plus the fees owed farther along the route. A sender does not pay itself a forwarding fee on its own first channel.

Timelocks also accumulate backward. The final hop starts with the invoice’s minimum final CLTV delta. Each forwarding node adds the delta for its outgoing channel.

+++ route-fees python-vm
def compute_fee(amount_msat, edge):
    return edge["fee_base_msat"] + amount_msat * edge["fee_proportional_millionths"] // 1_000_000

def make_route(payment_msat, final_cltv_delta, half_channels):
    if not half_channels:
        raise ValueError("A payment route needs at least one channel")
    amount, delay, route = payment_msat, final_cltv_delta, []
    for index in range(len(half_channels) - 1, -1, -1):
        edge = half_channels[index]
        check_amount(edge, amount)
        route.insert(0, {"id": edge["destination"], "channel": edge["short_channel_id"],
                         "amount_msat": amount, "delay": delay, "style": "tlv"})
        if index > 0:
            amount += compute_fee(amount, edge)
            delay += edge["cltv_expiry_delta"]
    return route

historical_route = make_route(payment_amount, 18, half_channels)
total_fee_msat = historical_route[0]["amount_msat"] - payment_amount
print("Fee:", total_fee_msat, "msat;", total_fee_msat / payment_amount * 10_000, "basis points")
print("First-hop CLTV delay:", historical_route[0]["delay"], "blocks")
historical_route
+++

## Serialize the onion payloads

An intermediate hop needs the amount and expiry to forward, and the outgoing short channel ID. The final hop needs the amount, expiry, and payment data. A payload is a sequence of type-length-value records prefixed with its total length.

`tu64` and `tu32` remove leading zero bytes. The short channel ID remains exactly eight bytes. TLV types and lengths use BigSize integers.

+++ onion-encoding python-vm
def bigsize(value):
    if not 0 <= value < 2**64:
        raise ValueError("BigSize is an unsigned 64-bit integer")
    if value < 0xfd:
        return bytes([value])
    for marker, width in ((0xfd, 2), (0xfe, 4), (0xff, 8)):
        if value < 1 << (width * 8):
            return bytes([marker]) + value.to_bytes(width, "big")

def truncated_integer(value, width):
    return value.to_bytes(width, "big").lstrip(b"\x00")

def encode_scid(scid):
    block, transaction, output = (int(part) for part in scid.split("x"))
    return block.to_bytes(3, "big") + transaction.to_bytes(3, "big") + output.to_bytes(2, "big")

def payload(amount_msat, expiry, channel=None, payment_secret=None, total_msat=None):
    fields = [(2, truncated_integer(amount_msat, 8)), (4, truncated_integer(expiry, 4))]
    if channel is not None:
        fields.append((6, encode_scid(channel)))
    else:
        secret = bytes.fromhex(payment_secret)
        if len(secret) != 32:
            raise ValueError("Payment secret must be 32 bytes")
        fields.append((8, secret + truncated_integer(total_msat, 8)))
    body = b"".join(bigsize(kind) + bigsize(len(value)) + value for kind, value in fields)
    return (bigsize(len(body)) + body).hex()

def make_onion_hops(route, blockheight, payment_secret):
    hops = []
    for index, hop in enumerate(route):
        if index + 1 < len(route):
            next_hop = route[index + 1]
            encoded = payload(next_hop["amount_msat"], blockheight + next_hop["delay"], channel=next_hop["channel"])
        else:
            encoded = payload(hop["amount_msat"], blockheight + hop["delay"],
                              payment_secret=payment_secret, total_msat=hop["amount_msat"])
        hops.append({"pubkey": hop["id"], "payload": encoded})
    return hops

illustrative_hops = make_onion_hops(historical_route, 1_000, "22" * 32)
illustrative_hops
+++

These payloads use the notebook’s illustrative height and secret. They are for inspection only. In the next lesson, use the same functions with real local node IDs, a fresh invoice, and the regtest block height.

~~~
id: capacity-versus-liquidity
A channel advertises enough total capacity for your payment. Is success guaranteed?
= No. The sending side may lack balance, or the route may otherwise be unavailable. [Gossip does not publish current channel balances.]
- Yes, as long as the channel is not disabled. [Advertised policies and total capacity are not a liquidity guarantee.]
~~~

::: references
1. [BOLT 7 — Gossip and forwarding policies](https://github.com/lightning/bolts/blob/master/07-routing-gossip.md)

    Specifies node and channel announcements, channel updates, and the fees and timelock policies used to construct a route.

2. [BOLT 4 — Onion routing and payloads](https://github.com/lightning/bolts/blob/master/04-onion-routing.md)

    Defines the onion packet format and the per-hop payment instructions encoded in the payloads you build in this lesson.
:::
