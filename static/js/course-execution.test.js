const test = require("node:test");
const assert = require("node:assert/strict");
const execution = require("./course-execution.js");

test("detects remote lines in mixed notebook cells", () => {
  assert.equal(execution.hasRemoteShellLine("height = 100\n!bitcoin-cli getblockhash {height}"), true);
  assert.equal(execution.hasRemoteShellLine("print('!local text')"), false);
  assert.equal(execution.hasRemoteShellLine("if ready:\n    !not-a-command"), false);
});

test("tokenizes quotes and escapes into argument boundaries", () => {
  assert.deepEqual(
    execution.tokenizeCommand('printf "hello %s\\n" "two words"'),
    ["printf", "hello %s\\n", "two words"]
  );
  assert.deepEqual(execution.tokenizeCommand("printf '%s\\n' value"), ["printf", "%s\\n", "value"]);
});

test("rejects malformed and empty commands", () => {
  assert.throws(() => execution.tokenizeCommand("printf 'missing"), /Unclosed quote/);
  assert.throws(() => execution.tokenizeCommand("   "), /empty/);
});

test("interpolates scalar variables without creating new arguments", async () => {
  const variables = {height: 100, filename: "output; echo unsafe"};
  assert.equal(await execution.interpolateToken("{height}", async name => variables[name]), "100");
  assert.equal(
    await execution.interpolateToken("prefix-{filename}", async name => variables[name]),
    "prefix-output; echo unsafe"
  );
});

test("rejects non-scalar interpolation", async () => {
  await assert.rejects(execution.interpolateToken("{items}", async () => ["one", "two"]), /scalar/);
});

test("missing invoice helpers point to their definition, not later function calls", () => {
  const sources = [
    'def bech32_decode(text):\n    return text',
    'def parse_invoice(text):\n    return bech32_decode(text)'
  ];
  assert.deepEqual(execution.missingPythonDefinition("Traceback (most recent call last):\nNameError: name 'bech32_decode' is not defined\n", sources), {name:"bech32_decode",index:0});
  assert.deepEqual(execution.missingPythonDefinition("NameError: name 'CHARSET' is not defined", ['CHARSET = "qpzry"']), {name:"CHARSET",index:0});
});

test("unrelated errors, nested definitions, and misspelled names do not suggest an earlier cell", () => {
  assert.equal(execution.missingPythonDefinition("NameError: name 'bech32_decode' is not defined", ['def outer():\n    def bech32_decode(text): pass']), null);
  assert.equal(execution.missingPythonDefinition("NameError: name 'bech32_decode' is not defined", ['def bech32_decoder(text): pass']), null);
  assert.equal(execution.missingPythonDefinition("ValueError: Invalid BOLT 11 Bech32 checksum", ['def bech32_decode(text): pass']), null);
  assert.equal(execution.missingPythonDefinition("NameError: name 'value' is not defined", ['value == 3']), null);
});

test("definition lookup recognizes imported aliases and multiline imports", () => {
  assert.deepEqual(execution.pythonDefinitions(`import os.path, json as json_codec
from coincurve import (
    PublicKey as SigningKey,
    PrivateKey,
)
from .helpers import decode
from helpers import *`), ["os", "json_codec", "SigningKey", "PrivateKey", "decode"]);
});

test("definition lookup recognizes unpacking, chained and annotated assignments", () => {
  assert.deepEqual(execution.pythonDefinitions(`network, amount = parse_hrp(hrp)
(left, [right, *rest]) = values
first = second = 1
answer: dict[str, int] = {}
uninitialized: int
one = 1; two = 2
async def fetch(): pass
class Invoice: pass`), ["network", "amount", "left", "right", "rest", "first", "second", "answer", "one", "two", "fetch", "Invoice"]);
});

test("comments, strings, locals, comparisons and attribute updates aren't definitions", () => {
  const source = `# fake = 1
doc = '''
fake = 1
def false_definition(): pass
'''
text = "ignored = 2; import nope"
def outer():
    local = 3
    import hidden
class Container:
    member = 4
result = call(
keyword = 5,
)
value == 3
value += 1
obj.attribute = 2
items[index] = 3
obj.attribute: int = 3`;
  assert.deepEqual(execution.pythonDefinitions(source), ["doc", "text", "outer", "Container", "result"]);
});

test("missing names use the latest actual definition and Python's suggested-name suffix", () => {
  const error = "NameError: name 'decode' is not defined. Did you mean: 'encode'?";
  assert.deepEqual(execution.missingPythonDefinition(error, ["def decode(): pass", "from helper import decode", "print(decode())"]), {name:"decode",index:1});
  assert.equal(execution.missingPythonDefinition("UnboundLocalError: local variable 'decode' referenced before assignment", ["decode = 1"]), null);
});
