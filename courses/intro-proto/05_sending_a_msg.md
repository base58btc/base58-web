# Sending A Message

Let's add a new field to our fruit order protocol. This time we'll include a message to our friend to go along with our order.

How many bytes do we need to add to our protocol to hold a message?

Assuming we're sending English text, each letter in a word will take up one byte.

Do we need ten extra bytes? Twenty? How big of a message are we sending? The problem is that we don't know ahead of time how big the message will be, so we can't use a fixed number of bytes for it. We need a way to say how many bytes we're sending in the message itself.

We call fields with an unknown size ahead of time *variable length*.

## Variable Length Fields

Up until this point we've been working with *fixed-length* data. We knew ahead of time how many fruits we wanted to send, and we were able to figure out how many bytes of data we'd need to fit each value.

For variable length fields we'll first send a size field that tells the receiver how many of the next bytes they should read.

```text
<bytes to say length of variable data> <variable data>
```

An example will help here. Let's say I want to send my friend the following message along with my fruit order:

```text
Ripe bananas only please
```

How long is this message? To find out we can use Python to convert it to bytes.

+++
msg = 'Ripe bananas only please'.encode()
len(msg)
+++

+++
msg.hex()
+++

We need to send 24 bytes of message data to our friend. We can use 1 byte to send the number 24. Recall from earlier that 1 byte can fit up to a value of 255.

What's the number 24 written as hex?

+++
hex(24)
+++

+++
msg = 'Ripe bananas only please'.encode()
(24).to_bytes(1, 'little') + msg
+++

Now we've got all the new information to send. First we send bytes that tell the length of the data, and then we send the data.

## Picking a Max Size for a Variable Length Field

What if we need to send more than 255 bytes of data? If we're only sending a single byte for the length, then our max message must be less than 256 bytes.

That's probably fine for many messages. But if we wanted to send longer data, we'd have to increase the size of the length from 1 byte to 2 bytes.

You *must* decide on the fixed length for the size of the field that holds the message length ahead of time. You can't make it 1 byte for some messages and 2 bytes for longer ones. This would break the parser on the other side who's reading your message. They need to know exactly how many bytes to read for the length.

Let's set it at 2 bytes. This will let us send up to 2^16 - 1, or 65,535 bytes, in our variable length field.

Note: The Replit on parsing legacy Bitcoin transactions will introduce CompactSize, which is a useful way of getting around this problem.

## Exercise

- Add a new field to our fruits message: `msg`.
- Implement `to_bytes_msg` so that we add a 2-byte length plus the `msg` field at the end.
- Implement `parse_bytes_msg` so we read off the variable length message.

Note that in the `fruits` object that gets returned, the `msg` field should be decoded to a string, not left as bytes. In other words, call `decode` on it.

+++
b'My message'.decode()
+++

???
id: fruit-message

Implement `parse_bytes_msg(data)` and `to_bytes_msg(fruits)`.

The first 11 bytes are the same fixed-width fruit fields from the last section. After that:

- read 2 bytes for the message length
- read that many message bytes
- decode the message bytes into a string

When writing bytes, encode the message string before adding it to the output.

---
def parse_bytes_msg(data):
    fruits = {}
    return fruits

def to_bytes_msg(fruits):
    out = b''
    return out

order = {
    'bananas': 500,
    'mangos': 249103,
    'apples': 22,
    'pineapple': 2944,
    'watermelon': 1,
    'msg': 'Ripe bananas only please',
}

encoded = to_bytes_msg(order)
encoded.hex()

---
def _encode_msg_check(fruits):
    out = b''
    for name, byte_count in [
        ('bananas', 2),
        ('mangos', 4),
        ('apples', 2),
        ('pineapple', 2),
        ('watermelon', 1),
    ]:
        out += fruits[name].to_bytes(byte_count, 'little')

    msg = fruits['msg'].encode()
    out += len(msg).to_bytes(2, 'little')
    out += msg
    return out

def _decode_msg_check(data):
    fruits = {}
    offset = 0
    for name, byte_count in [
        ('bananas', 2),
        ('mangos', 4),
        ('apples', 2),
        ('pineapple', 2),
        ('watermelon', 1),
    ]:
        fruits[name] = int.from_bytes(data[offset:offset + byte_count], 'little')
        offset += byte_count

    msg_len = int.from_bytes(data[offset:offset + 2], 'little')
    offset += 2
    fruits['msg'] = data[offset:offset + msg_len].decode()
    return fruits

order_1 = {
    'bananas': 500,
    'mangos': 249103,
    'apples': 22,
    'pineapple': 2944,
    'watermelon': 1,
    'msg': 'Ripe bananas only please',
}

order_2 = {
    'bananas': 0,
    'mangos': 1,
    'apples': 2,
    'pineapple': 3,
    'watermelon': 4,
    'msg': 'Bring grapes too',
}

assert to_bytes_msg(order_1) == _encode_msg_check(order_1)
assert to_bytes_msg(order_2) == _encode_msg_check(order_2)
assert parse_bytes_msg(_encode_msg_check(order_1)) == order_1
assert parse_bytes_msg(_encode_msg_check(order_2)) == order_2
assert parse_bytes_msg(to_bytes_msg(order_1)) == order_1

---

Make sure the message length is the length of the encoded bytes, not the length of the whole fruit order.

???
