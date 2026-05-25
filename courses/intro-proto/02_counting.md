# Counting Fruits

Your friend is going to buy fruit for you at the market. Before they left, you talked about it and came to an agreement that they would only pick up 5 types of fruit. 

Those fruits are:
- bananas
- mangos
- apples
- pineapple
- watermelon

You also decided that, at most, they'd be able to carry home 

- 1,000 bananas,
- 70,000 mangos,
- 260 apples,
- 20,0000 pineapples, and
- 15 watermelon.

Let's define a `maxsize` dict for each of these types of fruits.

## A basic byte-level protocol

Since this is a computer protocol, we're going to have to fit the information about each fruit into _bytes_. We'll need to figure out how many bytes we need for each fruit, in order to communicate how many of each that there are.

+++
maxsize = {
    'bananas': 1000,
    'mangos': 70000,
    'apples': 260,
    'pineapples': 20000,
    'watermelons': 15,
}
maxsize
+++

### Max sizes of bytes

A byte, in computer programming, is 8-bits of data or eight 0's and 1's in a row. Typically, a byte of data is encoded using hexadecimal, which lets you write a number from binary such as 00011000 into two digits.

Numbers that are expressed in computer architectures come in byte-sizes. A data field is like a bucket, the number of bytes for that field is the size of the bucket. Every bucket size is measured in bytes.

For this exercise, each type of fruit is a field of data that we want to be able to translate to bytes.

For each type of fruit, we need to pick how many bytes that field will require. We need to pick a number of bytes that that will let us store a number *bigger* than the maximum amount that our friend can carry home -- this way we'll always be able to tell our friend exactly how many fruit to bring home.

We have 4 options for the number of bytes we can use for each field.

- 1 byte.  A 1-byte bucket maxes out at to 2^8 - 1, or 255
- 2 bytes. A 2-byte bucket maxes out at 2^16 - 1, or 65,535
- 4 bytes. A 4-byte bucket maxes out at 2^32 - 1, or 4,294,967,295
- 8 bytes. A 8-byte bucket maxes out at 2^64 - 1, or 18,446,744,073,709,551,615


As an example, let's say that we wanted to pick how many bytes we'd need for the `pineapples` field. The `maxsize['pineapples']` in this case is 20,000. We can't fit the number 20,000 into 1 byte. We can fit it into 2 bytes, however, as the maxsize for this field is 65,535.

+++
255 > maxsize['pineapples']
+++

+++
65353 > maxsize['pineapples']
+++


## Exercise: How many bytes does each fruit need?
Using the max number that each byte count can express, figure out the minimum number of bytes we'd need to tell our friend to get us the most they can of each fruit.

- For each fruit, fill in the max number of bytes we'd need in `how_many_bytes`,
- Run the tests to see if you can make the `how_many_bytes` pass.

In case you need it, here's a reminder of how many of each your friend can bring back.

- bananas: 1,000
- mangos: 70,000
- apples: 260
- pineapple: 20,000
- watermelon: 15


~~~
What byte size can hold `20_000`?

- 1 byte [255 is too small.]
= 2 bytes [Correct: 65,535 can hold it.]
- 4 bytes [This works, but is larger than necessary]
~~~

We have already determined that `pineapples` need 2 bytes.


~~~
What byte size can hold `70_000`?

- 1 byte [255 is too small.]
- 2 bytes [65,535 is too small.]
= 4 bytes [This works! It's smallest set that can hold the highest count.]
~~~

Fill in the rest of the bytes.

???
id: byte-count

Have `bytecounts` return a dict with the correct byte sizes for each of the fruits

---

bytecounts = {
    'bananas': 0,
    'mangos': 0,
    'apples': 0,
    'pineapple': 2,
    'watermelon': 0,
}
bytecounts

---
assert bytecounts == {
    'bananas': 2,
    'mangos': 4,
    'apples': 2,
    'pineapple': 2,
    'watermelon': 1,
}
---
Try again. As a reminder, here's the maximum of each fruit you can bring home.
- 1,000 bananas,
- 70,000 mangos,
- 260 apples,
- 20,0000 pineapples, and
- 15 watermelon.
???

