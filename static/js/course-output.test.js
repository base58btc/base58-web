const test = require("node:test");
const assert = require("node:assert/strict");
const {format} = require("./course-output.js");

test("JSON gets indentation without rounding numbers, sorting keys or changing strings", () => {
  const source = '{"channel_id":18446744073709551615,"fee":1.2300e-8,"10":true,"2":null,"label":"⚡ {a,b} \\n \\"quoted\\""}';
  const expected = '{\n  "channel_id": 18446744073709551615,\n  "fee": 1.2300e-8,\n  "10": true,\n  "2": null,\n  "label": "⚡ {a,b} \\n \\"quoted\\""\n}';
  assert.equal(format(source + "\n"), expected + "\n");
  assert.equal(format(expected), expected);
});

test("Python dictionaries, tuples, sets, bytes and booleans retain their types", () => {
  const source = "{'amount': 10000, 'route': [{'node': b'\\x02ab', 'active': True}], 'extra': (None,), 'flags': {1, 2}}";
  const expected = "{\n  'amount': 10000,\n  'route': [\n    {\n      'node': b'\\x02ab',\n      'active': True\n    }\n  ],\n  'extra': (None,),\n  'flags': {1, 2}\n}";
  assert.equal(format(source), expected);
  assert.equal(format(expected), expected);
  assert.equal(format("('bcrt', 10000)\n"), "('bcrt', 10000)\n");
  assert.equal(format("[[], {}, (), {'ready': False, 'other': None},]\n"), "[\n  [],\n  {},\n  (),\n  {\n    'ready': False,\n    'other': None\n  },\n]\n");
});

test("long sequences wrap at item boundaries", () => {
  const items = Array.from({length: 30}, (_, index) => String(index));
  assert.equal(format("[" + items.join(",") + "]"), "[\n  " + items.join(",\n  ") + "\n]");
});

test("printed collections can follow labels and precede challenge results", () => {
  const source = 'Channels:\n{"count":2}\n{\'next\': None}\n\nCorrect.';
  assert.equal(format(source), 'Channels:\n{\n  "count": 2\n}\n{\n  \'next\': None\n}\n\nCorrect.');
  assert.equal(format("  {\"a\": 1}\n"), "  {\n  \"a\": 1\n}\n");
});

test("plain text, tracebacks, unsupported reprs and incomplete output remain unchanged", () => {
  const values = [
    "Hello, Lightning!\n", "", "42\n", "b'\\xff'", "'a string'", "{'a': <object at 0x123>}",
    "{'a': frozenset({1, 2})}", "{'recursive': {...}}", "[1, 2", "{'a': 1, 'b'}",
    "[1: 2]", "(hello)", "(42)", "{'a': 1} extra", "{unquoted: 1}",
    "Traceback (most recent call last):\n  File 'cell', line 1\nValueError: {'bad': 1}\n",
    "{\"a\": \"broken\\\"}", "{'a': 1}\n[output truncated]\n",
    "[".repeat(50) + "1" + "]".repeat(50), "[" + "1,".repeat(21000) + "]",
    '{"large":"' + "x".repeat(1 << 19) + '"}'
  ];
  for (const source of values) assert.equal(format(source), source);
  const failed = '{"message":"broken"}\n';
  assert.equal(format(failed, false), failed);
});

test("output stays inert text, including HTML and code-like values", () => {
  const source = '{"html":"<img src=x onerror=alert(1)>","__proto__":{"polluted":true}}';
  assert.equal(format(source), '{\n  "html": "<img src=x onerror=alert(1)>",\n  "__proto__": {\n    "polluted": true\n  }\n}');
  assert.equal({}.polluted, undefined);
  for (const source of ["[alert(1)]", "{'x': __import__('os').system('echo nope')}", "[1 + 2]"]) {
    assert.equal(format(source), source);
  }
});
