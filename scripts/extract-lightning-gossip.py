#!/usr/bin/env python3
"""Extract a reproducible teaching graph from the November 2023 course store.

Input is the actual Git LFS object, not its pointer. The hash pins the parser to
this historical format; this is deliberately not a general gossip-store reader.
Wire layout: BOLT 7. Store framing: original pyln_client at 250b8a2.
"""
import argparse
from collections import deque
import hashlib
import json
from pathlib import Path
import struct

SHA256 = "8db52d5a9b4ac8c637bafd82447a84c74fc7c866ca3c73ae3fdce817c609e8cd"
REVISION = "62491b76177123c8cebcc0fadccc93ca826d8dc7"
SOURCE = "0235acd5e943cbe5387815a8fb15ebc109b26556f82fa28cfcb0c2a3fda7da80fa"
DESTINATION = "02c9477078f21e6f17816257132678d2d27f4c7074c8b0114d8be5ce9cee14bc26"


def scid(value):
    return f"{value >> 40}x{(value >> 16) & 0xffffff}x{value & 0xffff}"


def extract(path):
    raw = path.read_bytes()
    if hashlib.sha256(raw).hexdigest() != SHA256:
        raise ValueError("Expected the pinned November 2023 LFS object")
    channels, offset, last = {}, 1, None
    while offset < len(raw):
        flags, length, _, _ = struct.unpack_from(">HHII", raw, offset)
        record = raw[offset + 12:offset + 12 + length]
        offset += 12 + length
        if len(record) != length:
            raise ValueError("Truncated record")
        if flags & 0x9000:  # deleted or zombie
            continue
        kind = int.from_bytes(record[:2], "big")
        if kind == 256:  # channel_announcement: signatures, features, chain, scid, keys
            feature_length = int.from_bytes(record[258:260], "big")
            position = 260 + feature_length + 32
            last = scid(int.from_bytes(record[position:position + 8], "big"))
            channels[last] = {
                "short_channel_id": last,
                "node1": record[position + 8:position + 41].hex(),
                "node2": record[position + 41:position + 74].hex(),
                "features": record[260:260 + feature_length].hex(),
                "satoshis": None, "updates": {},
            }
        elif kind == 4101 and last in channels:
            channels[last]["satoshis"] = int.from_bytes(record[2:10], "big")
        elif kind == 4103:
            channels.pop(scid(int.from_bytes(record[2:10], "big")), None)
        elif kind == 258:  # channel_update
            key = scid(int.from_bytes(record[98:106], "big"))
            if key not in channels:
                continue
            timestamp, message_flags, channel_flags, delta, minimum, base, ppm = struct.unpack_from(">IBBHQII", record, 106)
            direction = channel_flags & 1
            maximum = int.from_bytes(record[130:138], "big") if message_flags & 1 else None
            channels[key]["updates"][direction] = {
                "direction": direction, "timestamp": timestamp, "disabled": bool(channel_flags & 2),
                "cltv_expiry_delta": delta, "htlc_minimum_msat": minimum,
                "htlc_maximum_msat": maximum, "fee_base_msat": base,
                "fee_proportional_millionths": ppm,
            }
    edges = []
    for channel in channels.values():
        for direction, update in channel["updates"].items():
            edges.append({**{k: v for k, v in channel.items() if k not in ("updates", "node1", "node2")},
                          **update,
                          "source": channel["node2" if direction else "node1"],
                          "destination": channel["node1" if direction else "node2"]})
    edges.sort(key=lambda e: (e["source"], e["destination"], e["short_channel_id"]))
    adjacency = {}
    for edge in edges:
        if not edge["disabled"]:
            adjacency.setdefault(edge["source"], []).append(edge["destination"])
    queue, parents = deque([SOURCE]), {SOURCE: None}
    while queue and DESTINATION not in parents:
        node = queue.popleft()
        for neighbor in adjacency.get(node, []):
            if neighbor not in parents:
                parents[neighbor] = node
                queue.append(neighbor)
    if DESTINATION not in parents:
        raise ValueError("Original class endpoints are not connected")
    path_nodes, node = [], DESTINATION
    while node is not None:
        path_nodes.append(node)
        node = parents[node]
    selected = set(path_nodes)
    # Keep the notebook's example channel, too, then a bounded neighborhood.
    example = channels["803775x1091x1"]
    selected.update((example["node1"], example["node2"]))
    queue = deque(sorted(selected))
    while queue and len(selected) < 64:
        for neighbor in adjacency.get(queue.popleft(), []):
            if neighbor not in selected:
                selected.add(neighbor)
                queue.append(neighbor)
                if len(selected) == 64:
                    break
    return {
        "provenance": {"repository": "https://github.com/base58btc/lightning_class", "revision": REVISION,
                       "file": "gossip_stores/nov_17.gossip_store", "sha256": SHA256,
                       "bytes": len(raw), "store_version": raw[0], "full_channel_count": len(channels),
                       "selection": "Original route and example channel, then sorted enabled neighbors up to 64 nodes; induced directed graph."},
        "source": SOURCE, "destination": DESTINATION, "nodes": sorted(selected),
        "edges": [e for e in edges if e["source"] in selected and e["destination"] in selected],
    }


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("store", type=Path)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    graph = extract(args.store)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(graph, indent=2) + "\n")
    print(f"{len(graph['nodes'])} nodes, {len(graph['edges'])} directed channel updates; {args.output}")
