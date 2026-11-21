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
