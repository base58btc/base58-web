# Counting Any

What if we don't want to buy bananas that day? Or what if we don't want to send a message to our friend along with our order? What if we want to order grapes? Our current protocol doesn't allow us to change what we're sending or add and remove fields.

The most common way to make fields optional in a protocol is to use a pattern called TLV, or *Type Length Value*.

Every field we want to send now has 3 parts.

- Type: a code that indicates which field this is. You and your friend must agree on this ahead of time.
- Length: the length of the data in this field.
- Value: the bytes of data for this field.

This is like a variable-length field, except we've added a new item to the front: a type.

For our protocol we won't have more than 255 items, so we can use 1 byte for the type.

We'll leave the length set to 2 bytes, like we did for our other variable length fields.

## Giving Out Types

Let's assign type values to our fields.

- message: type 1
- bananas: type 2
- mangos: type 7
- pineapple: type 11
- watermelon: type 20
- apples: type 22

Now when we get a message in, we'll read off the type value to figure out what field it is. Then we'll read out the length. Finally we'll read off the data. Depending on what type of data it is, we'll either turn that into an integer number for fruits or decode the message into a string.

Here's a quick example of how to read off a single type field for bananas.

+++
data = bytes.fromhex('020200f401')

fruits = {}
field_type = int.from_bytes(data[:1], 'little')
length = int.from_bytes(data[1:3], 'little')
value = data[3:3 + length]

if field_type == 2:
    fruits['bananas'] = int.from_bytes(value, 'little')

fruits
+++

To write data, we'll first look up the type and write that in. Then we'll write the length of the data, and finally the actual byte data.

## Exercise

- Fill in `to_bytes_tlv(fruits)` and `parse_tlv(data)` using the new types.
- What happens if we don't fill in a field?
- Are TLV encoded messages longer or shorter than not using TLV messages?
- Why would you use a TLV instead of a fixed-length message?

???
id: fruit-tlv

Implement `parse_tlv(data)` and `to_bytes_tlv(fruits)`.

For this exercise:

- each TLV type is 1 byte
- each TLV length is 2 bytes
- fruit values should be encoded as little-endian integers
- `msg` should be encoded and decoded as a string
- omit fields that are not present in the input dict
- write fields in this order: `msg`, `bananas`, `mangos`, `pineapple`, `watermelon`, `apples`

Use the type values from the lesson.

---
def parse_tlv(data):
    fruits = {}
    return fruits

def to_bytes_tlv(fruits):
    out = b''
    return out

order = {
    'bananas': 500,
    'watermelon': 1,
    'msg': 'Ripe bananas only please',
}

encoded = to_bytes_tlv(order)
parse_tlv(encoded)

---
TYPE_BY_NAME = {
    'msg': 1,
    'bananas': 2,
    'mangos': 7,
    'pineapple': 11,
    'watermelon': 20,
    'apples': 22,
}

NAME_BY_TYPE = {value: key for key, value in TYPE_BY_NAME.items()}
FIELD_ORDER = ['msg', 'bananas', 'mangos', 'pineapple', 'watermelon', 'apples']

def _int_to_minimal_bytes(value):
    byte_count = max(1, (value.bit_length() + 7) // 8)
    return value.to_bytes(byte_count, 'little')

def _tlv_check(fruits):
    out = b''
    for name in FIELD_ORDER:
        if name not in fruits:
            continue
        if name == 'msg':
            value = fruits[name].encode()
        else:
            value = _int_to_minimal_bytes(fruits[name])
        out += TYPE_BY_NAME[name].to_bytes(1, 'little')
        out += len(value).to_bytes(2, 'little')
        out += value
    return out

order_1 = {
    'bananas': 500,
    'watermelon': 1,
    'msg': 'Ripe bananas only please',
}

order_2 = {
    'mangos': 249103,
    'apples': 22,
    'pineapple': 2944,
}

order_3 = {
    'msg': 'Bring grapes too',
}

assert to_bytes_tlv(order_1) == _tlv_check(order_1)
assert to_bytes_tlv(order_2) == _tlv_check(order_2)
assert to_bytes_tlv(order_3) == _tlv_check(order_3)
assert parse_tlv(_tlv_check(order_1)) == order_1
assert parse_tlv(_tlv_check(order_2)) == order_2
assert parse_tlv(_tlv_check(order_3)) == order_3
assert parse_tlv(to_bytes_tlv(order_1)) == order_1

---

Make sure your parser keeps reading until it reaches the end of the data.

???
