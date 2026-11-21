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
