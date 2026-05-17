# Welcome to Protocol Thinking

Hi welcome to Base58's course on "protocol thinking". This is some preview work for our walk-thru on how to parse bitcoin transactions.

In this module, we're going to start thinking about what a protocol is and the kind of work that it does.

Protocols are all about *communication*. Designing good protocols is the art of figuring out what needs to be communicated, and the most compact way to send it. A good protocol is both extensible as well as terse -- it says what needs to be said, and no more, but leaves the option to say more later, if the conversation changes.

To do explore this, we'll use the example of communicating to your friend how many and what type of fruit to buy at the market.  

In this mini-course, we'll design a byte-level protocol and parser that lets us write information to bytes and then read it back into programming data.

By the end, you should be able to:
- calculate how many bytes are necessary for sending data
- be able to write variable length datafields over bytes
- have implemented a custom TLV data struct
