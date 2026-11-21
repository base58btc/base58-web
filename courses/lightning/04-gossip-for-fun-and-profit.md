# Lightning gossip for fun and profit

A Lightning node learns about public channels from gossip. A channel announcement identifies the two endpoints. Each endpoint can publish a channel update describing its policy for forwarding in one direction. Each direction is a **half-channel**.

## Open the gossip store

+++ load-gossip python-vm
from pyln.client import Gossmap, GossmapNodeId, GossmapStats
from gossip import load_gossip

gossmap = load_gossip()
print(len(gossmap.nodes), "nodes;", len(gossmap.channels), "channels")
+++

+++ gossip-example-channel python-vm
channel = gossmap.get_channel("803775x1091x1")
channel.satoshis, channel.features
+++

+++ gossip-channel-directions python-vm
print("Source:", channel.half_channels[0].source)
print("Destination:", channel.half_channels[0].destination.node_id)
print("Incoming:", gossmap.get_neighbors_hc(destination=channel.half_channels[0].destination.node_id))
print("Outgoing:", gossmap.get_neighbors_hc(source=channel.half_channels[0].destination.node_id))
+++

## Find neighbors

+++ gossip-neighbors python-vm
my_source = GossmapNodeId("0235acd5e943cbe5387815a8fb15ebc109b26556f82fa28cfcb0c2a3fda7da80fa")
my_destination = GossmapNodeId("02c9477078f21e6f17816257132678d2d27f4c7074c8b0114d8be5ce9cee14bc26")

def find_neighbors(gossmap, node, depth=0):
    return gossmap.get_neighbors(node, depth=depth)

find_neighbors(gossmap, my_source, depth=1)
+++

## Find a route

What channels should we use to get from the source to the destination? Search one hop at a time, keeping the half-channels that connect the nodes. A visited set prevents loops and lets the search finish when no route exists.

???
id: find-route
runtime: python-vm

Complete `find_route`. Return a list of enabled half-channels forming a route with the fewest hops. Return an empty list when source and destination match, and raise `ValueError` when no route exists.
---
from collections import deque

def find_route(gossmap, source, destination):
    if source == destination:
        return []
    queue = deque([(source, [])])
    visited = {source}
    while queue:
        node, path = queue.popleft()
        for half_channel in sorted(gossmap.get_neighbors_hc(source=node), key=lambda hc: str(hc.channel.scid)):
            # Skip disabled directions and nodes already visited.
            # Extend the path; return it when the destination is reached.
            # Otherwise mark the neighbor visited and queue it.
            pass
    raise ValueError("No enabled route exists in this graph")
---
from types import SimpleNamespace

class TestGraph:
    def __init__(self, edges):
        self.edges, self.visits = edges, 0
    def get_neighbors_hc(self, source):
        self.visits += 1
        assert self.visits <= 100, "Search must terminate on cycles"
        return [edge for edge in self.edges if edge.source.node_id == source]

def edge(source, destination, scid, disabled=False):
    return SimpleNamespace(source=SimpleNamespace(node_id=source),
                           destination=SimpleNamespace(node_id=destination),
                           channel=SimpleNamespace(scid=scid), disabled=disabled)

ab, bc, cd = edge("A", "B", "1x1x0"), edge("B", "C", "2x1x0"), edge("C", "D", "3x1x0")
ac = edge("A", "C", "4x1x0")
ba, disabled = edge("B", "A", "5x1x0"), edge("A", "D", "6x1x0", True)
graph = TestGraph([ab, bc, cd, ac, ba, disabled])
assert find_route(graph, "A", "D") == [ac, cd]
assert find_route(TestGraph([ab, ba]), "A", "A") == []
assert find_route(TestGraph([ab]), "A", "B") == [ab]
for graph, source, destination in [(TestGraph([ab, ba]), "A", "D"), (TestGraph([ab]), "B", "A")]:
    try:
        find_route(graph, source, destination)
    except ValueError:
        pass
    else:
        raise AssertionError("Unreachable destinations must raise ValueError")
---
Find the shortest enabled route, respect channel direction, and terminate when a destination is unreachable.
???

+++ gossip-route python-vm
route_channels = find_route(gossmap, my_source, my_destination)
path = [my_source] + [hc.destination.node_id for hc in route_channels]
[str(node) for node in path]
+++
## Which half-channels are we using?

Each channel has two directions. Direction 0 goes from the smaller public key to the larger; direction 1 goes the other way.

+++ gossip-half-channels python-vm
def find_direction(source, destination):
    return 0 if source < destination else 1

for hc in route_channels:
    direction = find_direction(hc.source.node_id, hc.destination.node_id)
    assert hc.channel.half_channels[direction] is hc
    print(hc.source, "->", hc.destination, hc.channel.scid, "/", direction)

def half_channel_policy(hc):
    return {
        "source": str(hc.source.node_id), "destination": str(hc.destination.node_id),
        "short_channel_id": str(hc.channel.scid), "direction": hc.direction,
        "satoshis": hc.channel.satoshis, "timestamp": hc.timestamp, "disabled": hc.disabled,
        "htlc_minimum_msat": hc.htlc_minimum_msat, "htlc_maximum_msat": hc.htlc_maximum_msat,
        "fee_base_msat": hc.fee_base_msat, "fee_proportional_millionths": hc.fee_proportional_millionths,
        "cltv_expiry_delta": hc.cltv_expiry_delta,
    }

half_channels = [half_channel_policy(hc) for hc in route_channels]
half_channels
+++

~~~
id: forwarding-direction
For Alice → Bob → Carol, which policy determines the fee Bob charges?
= Bob’s outgoing direction on the Bob–Carol channel. [Bob charges for forwarding over its outgoing channel.]
- Alice’s outgoing direction on the Alice–Bob channel. [That is the sender’s first channel; Alice does not charge itself a forwarding fee.]
- Carol’s outgoing direction on the Bob–Carol channel. [That policy applies to payments traveling from Carol to Bob.]
~~~

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

~~~
id: capacity-versus-liquidity
A channel advertises enough total capacity for your payment. Is success guaranteed?
= No. The sending side may lack balance, or the route may otherwise be unavailable. [Gossip does not publish current channel balances.]
- Yes, as long as the channel is not disabled. [Advertised policies and total capacity are not a liquidity guarantee.]
~~~

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

~~~
id: routing-fee
Bob forwards 10,000,000 msat with a base fee of 1,000 msat and a proportional fee of 100 ppm. What fee does Bob charge?
= 2,000 msat. [1,000 + floor(10,000,000 × 100 / 1,000,000) = 2,000.]
- 1,000 msat. [This includes the base fee but omits the proportional fee.]
- 1,001,000 msat. [Parts per million requires division by 1,000,000.]
~~~

~~~
id: routing-cltv
For Alice → Bob → Carol, the invoice requires a final CLTV delta of 18 and Bob’s outgoing policy requires 40. What first-hop delay should Alice use?
= 58 blocks. [Add the recipient’s final delta and Bob’s forwarding delta.]
- 18 blocks. [Bob also needs a 40-block margin between its incoming and outgoing HTLCs.]
- 40 blocks. [The invoice’s final delta must also be included.]
- 202 blocks if Alice’s outgoing policy advertises 144. [Alice does not add its own forwarding policy to a payment it originates.]
~~~

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

~~~
id: final-onion-payload
Which data belongs in the final hop’s payload instead of an outgoing short channel ID?
= The payment secret and total amount in payment_data. [The recipient needs payment data; there is no next channel to forward through.]
- The payment preimage. [The recipient reveals the preimage only when settling the payment.]
- The sender’s private key. [Private keys are never included in onion payloads.]
~~~

Use the same functions with a fresh invoice and the current block height to send a payment in the next lesson.


::: references
1. [BOLT 7 — Gossip and forwarding policies](https://github.com/lightning/bolts/blob/master/07-routing-gossip.md)

    Specifies node and channel announcements, channel updates, and the fees and timelock policies used to construct a route.

2. [BOLT 4 — Onion routing and payloads](https://github.com/lightning/bolts/blob/master/04-onion-routing.md)

    Defines the onion packet format and the per-hop payment instructions encoded in the payloads you build in this lesson.
:::
