# Writing Fruits to Send

Now that we've got the sizes of bytes for each fruit, we can start building our raw protocol structure.

When you and your friend talked, before you left, you figured out how many bytes each fruit could take up and what order those fruits would appear in the list.

- bananas: 1,000 (2 bytes)
- mangos: 70,000 (4 bytes)
- apples: 260 (2 bytes)
- pineapple: 20,000 (2 bytes)
- watermelon: 15 (1 byte)


In the last section, we defined a method that will return a dict with the correct byte count for each fruit.

+++
def how_many_bytes():
    return  {
        'bananas': 2,
        'mangos': 4,
        'apples': 2,
        'pineapple': 2,
        'watermelon': 1,
    }

how_many_bytes()
+++

*By the end of this chapter*, we'll have implemented a method called `to_bytes` which will turn a request for fruits into a bytestring.


### Writing Bytes

We can write each 8-bit byte as two hexadecimal characters. Hex characters are base 16, which means two characters can express up to 255.

To illustrate what these bytes will look like on the wire, I've written out empty bytes of the correct size for each fruit we're sending to our friend. 

I've put a `|` character between the bytes, so you can easily see each byte count.

- bananas: `00|00`
- mangos: `00|00|00|00`
- apples: `00|00`
- pineapple: `00|00`
- watermelon: `00`


Without the pipes `|` between the bytes, this looks like:

- bananas: `0000`
- mangos: `00000000`
- apples: `0000`
- pineapple: `0000`
- watermelon: `00`


## Building a Request

Let's say we wanted to ask our friend to bring us back 5 bananas, 10k mangos, 30 apples, 10,001 pineapples, and 5 watermelon. How would we format this request?

First we'd need to convert each of our requested fruit count numbers to bytes. Then we'd put all the bytes together, in order, and send them to our friend.

We can convert a number to bytes in python using the `int.to_bytes()` call. 

Note: For the purposes of this protocol, we're going to be using something called 'little endian', which we'll add to the `to_bytes` field as the second parameter.


Here's how you'd convert the number 5 to two little-endian bytes.

+++
x = (5).to_bytes(2, 'little')
x
+++

This output is formatted as a python 'byte type'. It isn't very readable. We can make it easier to read by formatting the bytes as `hex` (or hexadecimal) before we print them.

+++
x_bytes = (5).to_bytes(2, 'little')
x_bytes.hex()
+++

The number `5` will also fit in one byte. Let's see what it looks like when we print it out, formatted just as one byte.

+++
x_bytes = (5).to_bytes(1, 'little')
x_bytes.hex()
+++


## Exercise
Let's write a two functions. First one that verifies the dict of fruits is valid (less than the max allowed). Then, we'll make a function that given a dictionary of fruits, will return one single bytestring with all of the requested amounts converted to bytes. The bytes should appear in the same order as our given list. 

You can concatenate bytestrings together into one longer bytestring with `+`.

+++
out = (5).to_bytes(2, 'little') + (10000).to_bytes(4, 'little')
out.hex()
+++

Here's an example input and what we'd expect to get out.

+++ tobytes-demo
input = {
    'bananas': 500,
    'mangos': 249103,
    'apples': 22,
    'pineapple': 2944,
    'watermelon': 1
}

# Note: this block won't work until we complete the challenge below: implementing `to_bytes`
out = to_bytes(input)
out.hex()
+++


???
id: valid-fruits

Before we can implement `to_bytes`, it will be very handy to have a way to check that the input `fruits` request is valid. Valid in this case means that the number of fruits requested are less than our max allowed fruits.

Let's implement `is_valid(fruits)`.

Return `False` if the inputs are invalid (too big). Return `True` otherwise.

---

# As a helper, here's the valid max sizes
maxsize = {
    'bananas': 1000,
    'mangos': 70000,
    'apples': 260,
    'pineapple': 20000,
    'watermelon': 15
}

def is_valid(fruits):
    # TODO: Implement this check
    return False

---

t_input_max = {
    'bananas': 1000,
    'mangos': 70000,
    'apples': 260,
    'pineapple': 20000,
    'watermelon': 15
}

t_input_empty = {
    'bananas': 0,
    'mangos': 0,
    'apples': 0,
    'pineapple': 0,
    'watermelon': 0
}

t_input_invalid = {
    'bananas': 1001,
    'mangos': 70001,
    'apples': 261,
    'pineapple': 21000,
    'watermelon': 22
}

assert is_valid(t_input_max)
assert is_valid(t_input_empty)
assert not is_valid(t_input_invalid)

---

Make sure that you're returning True or False

???

Now that we've got a way to validate fruits, let's implement the `to_bytes(fruits)` function.

???
id: to-bytes

Implement `to_bytes(fruits)`,

Included are a few test cases that we'll use to make sure your function is working as intended.

Hint: Use 'how_many_bytes()' to calculate the byte size for each field and `is_valid()` so you don't return invalid bytestrings.

---
# Here's some test data to practice with!
input_1 = {
    'bananas': 500,
    'mangos': 24103,
    'apples': 22,
    'pineapple': 2944,
    'watermelon': 1
}

input_max = {
    'bananas': 1000,
    'mangos': 70000,
    'apples': 260,
    'pineapple': 20000,
    'watermelon': 15
}

input_empty = {
    'bananas': 0,
    'mangos': 0,
    'apples': 0,
    'pineapple': 0,
    'watermelon': 0
}

input_invalid = {
    'bananas': 1001,
    'mangos': 70001,
    'apples': 261,
    'pineapple': 21000,
    'watermelon': 22
}


# Hint: if there's an error or invalid input, return `None`
def to_bytes(fruits):
    out = b''
    # TODO: FILL THIS IN
    return out


to_bytes(input_1).hex()

--- 

test_input_1 = {
    'bananas': 500,
    'mangos': 24103,
    'apples': 22,
    'pineapple': 2944,
    'watermelon': 1
}

test_input_max = {
    'bananas': 1000,
    'mangos': 70000,
    'apples': 260,
    'pineapple': 20000,
    'watermelon': 15
}

test_input_empty = {
    'bananas': 0,
    'mangos': 0,
    'apples': 0,
    'pineapple': 0,
    'watermelon': 0
}

test_input_invalid = {
    'bananas': 1001,
    'mangos': 70001,
    'apples': 261,
    'pineapple': 21000,
    'watermelon': 22
}

def how_many_bytes():
    return  {
        'bananas': 2,
        'mangos': 4,
        'apples': 2,
        'pineapple': 2,
        'watermelon': 1,
    }

def is_valid(fruits):
    for x in fruits.keys():
        if fruits[x] > test_input_max[x]:
            return False
    return True

def bytecheck(inp):
    if !is_valid(inp):
        return None
    bs = b''
    for x in ['bananas', 'mangos', 'apples', 'pineapple', 'watermelon']:
        bytecount = how_many_bytes()[x]
        bs += inp[x].to_bytes(bytecount, 'little')
    return bs

assert to_bytes(test_input_1) == bytecheck(test_input_1)
assert to_bytes(test_input_max) == bytecheck(test_input_max)
assert to_bytes(test_input_empty) == bytecheck(test_input_empty)
assert to_bytes(test_input_invalid) == bytecheck(test_input_invalid)

---

For `input_1` you should get `f4010fcd03001600800b01`
For `input_empty` you should get `0000000000000000000000`

For `input_max` you should get `e803701101000401204e0f`
For `input_invalid` you should get `None`

???


Got it working? Try heading back up to the [prior block](#tobytes-demo) and seeing what it prints out.
