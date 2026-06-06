# Reading Fruit Requests

Once we've translated our fruit counts request into bytes, we'll send the bytes to our friend over the Internet.

When our friend receives the bytes, they'll need to parse the bytes back into numbers so they know what to buy. Let's take a bytestring and convert it back into our fruits dict.

The message we'll receive will look like this:

```
f4010fcd03001600800b01
```

And we want to read out the bytes, convert them into integers, and return a dict with the correct values for each fruit.

```python
fruits = {
    'bananas': 500,
    'mangos': 249103,
    'apples': 22,
    'pineapple': 2944,
    'watermelon': 1
}
```

## Python Tools for Reading Bytes

To do this, we'll take byte data and read out a fixed-length set of bytes. We'll then convert those bytes into a value.

Python gives us `int.from_bytes` for this.

```python
int.from_bytes(data, 'little')
```

You'll need to give the method the bytes that you want to turn into a value.

Here's a short example of how to take a 4-byte and 2-byte value from a hexstring, convert that to bytes, and then pull out two integers.

+++
hexstr = '010101010202'
data = bytes.fromhex(hexstr)

# I want just the first 4 bytes.
first_value = int.from_bytes(data[:4], 'little')
print(first_value)
# 16843009

# Then I want the last 2 bytes.
second_value = int.from_bytes(data[4:4+2], 'little')
print(second_value)
# 514
+++

## Exercise

Your friend just sent you a message: `09031b9104000001d50b0b`.

How many of each fruit should we bring them back?

- Write a function to parse bytes from your friend into a fruit purchase order.
- Fill in `how_many_fruits` so the `how_many_fruits` test passes.

???
id: parse-bytes

Implement `parse_bytes(data)` for the fixed-width fruit request.

The field order is:

- bananas: 2 bytes
- mangos: 4 bytes
- apples: 2 bytes
- pineapple: 2 bytes
- watermelon: 1 byte

Then finish `how_many_fruits()`.

---
def parse_bytes(data):
    fruits = {
        'bananas': 0,
        'mangos': 0,
        'apples': 0,
        'pineapple': 0,
        'watermelon': 0,
    }

    return fruits

def how_many_fruits():
    data = bytes.fromhex('09031b9104000001d50b0b')
    return parse_bytes(data)

how_many_fruits()

---
def _expected_parse_bytes(data):
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
    return fruits

sample = bytes.fromhex('f4010fcd03001600800b01')
friend_msg = bytes.fromhex('09031b9104000001d50b0b')

assert parse_bytes(sample) == {
    'bananas': 500,
    'mangos': 249103,
    'apples': 22,
    'pineapple': 2944,
    'watermelon': 1,
}
assert parse_bytes(friend_msg) == _expected_parse_bytes(friend_msg)
assert how_many_fruits() == {
    'bananas': 777,
    'mangos': 299291,
    'apples': 256,
    'pineapple': 3029,
    'watermelon': 11,
}

---

Make sure `parse_bytes` reads each field in order and advances by the correct number of bytes.

???
