# Writing Fruits to Send

Now that we've got the sizes of bytes for each fruit, we can start building our raw protocol structure.

When you and your friend talked, before you left, you figured out how many bytes each fruit could take up and what order those fruits would appear in the list.

- bananas: 1,000 (2 bytes)
- mangos: 70,000 (4 bytes)
- apples: 260 (2 bytes)
- pineapple: 20,000 (2 bytes)
- watermelon: 15 (1 byte)

Each byte is two-characters, written in hex. Let's put empty bytes next to each of the fields we're sending. I've put a `|` character between the bytes, so you can easily see each byte count.

- bananas: 00|00
- mangos: 00|00l00|00
- apples: 00|00 
- pineapple: 00|00
- watermelon: 00

## Building a Request

Let's say we wanted to ask our friend to bring us back 5 bananas, 10k mangos, 30 apples, 10,001 pineapples, and 5 watermelon. How would we format this request?

First we'd need to convert each of our requested fruit count numbers to bytes. Then we'd put all the bytes together, in order, and send them to our friend.

We can convert a number to bytes in python using the `int.to_bytes()` call. 

Note: For the purposes of this protocol, we're going to be using something called 'little endian'. This tells the encoder how to write the bytes down.

Here's how you'd convert the number 5 to two little-endian bytes.

+++
x = (5).to_bytes(2, 'little')
print(x)
+++

Hint: to make it readable, print it as hex.

+++
x = (5).to_bytes(2, 'little').hex()
print(x)
+++

## Exercise
Now lets write a function that given a dictionary of fruits, will return one single bytestring with all of the requested amounts converted to bytes. The bytes should appear in the same order as our given list. 

Note: you can add byte lists together with `+`.

+++
(5).to_bytes(2, 'little') + (10000).to_bytes(4, 'little')
+++

Here's an example input and what we'd expect to get out.

+++
input = {
    'bananas': 500,
    'mangos': 249103,
    'apples': 22,
    'pineapple': 2944,
    'watermelon': 1
}

out = to_bytes(input)
print(out.hex())
+++


Implement `to_bytes`,

//FIXME: turn into assertion check

+++
def to_bytes(fruits):
    out = b''
    return out

to_bytes(input)
+++

- Run the Unit Tests, the `to_bytes` test should pass now.

// FIXME: recover unit tests + add here
// FIXME: button to press to check that they pass
