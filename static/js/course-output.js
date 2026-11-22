(function(root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  if (root) root.CourseOutput = api;
})(typeof window !== "undefined" ? window : globalThis, function() {
  "use strict";

  // Read only built-in collection literals. Keep scalar tokens verbatim: parsing
  // through JSON numbers would round Lightning channel IDs and large integers.
  // Output is never evaluated as code, and unsupported reprs remain plain text.
  function prettyCollection(source) {
    if (!/^[{[(]/.test(source)) return null;
    let offset = 0, count = 0;
    const space = () => { while (/\s/.test(source[offset] || "")) offset++; };
    const invalid = () => { throw new Error("Not a collection literal"); };
    function value(depth) {
      if (depth > 40 || ++count > 20000) invalid();
      space();
      const start = offset, open = source[offset];
      if (open && "{[(".includes(open)) {
        offset++;
        const close = {"{":"}", "[":"]", "(":")"}[open];
        const entries = [];
        let mapping, trailing = false;
        space();
        while (source[offset] !== close) {
          const key = value(depth + 1);
          space();
          const pair = source[offset] === ":";
          if (pair && open !== "{") invalid();
          if (open === "{") {
            if (mapping !== undefined && mapping !== pair) invalid();
            mapping = pair;
          }
          if (pair) {
            offset++;
            entries.push([key, value(depth + 1)]);
          } else entries.push([key]);
          space();
          if (source[offset] !== ",") break;
          offset++;
          space();
          trailing = source[offset] === close;
        }
        if (source[offset++] !== close) invalid();
        if (open === "(" && entries.length === 1 && !trailing) invalid();
        return {open, close, entries, mapping, trailing};
      }
      // Python repr uses single/double quotes, with an optional bytes prefix.
      if (source[offset] === "b" && /['"]/.test(source[offset + 1] || "")) offset++;
      const quote = source[offset];
      if (quote === "'" || quote === '"') {
        offset++;
        while (offset < source.length) {
          const ch = source[offset++];
          if (ch === "\\") offset++;
          else if (ch === quote) return source.slice(start, offset);
          else if (ch === "\n" || ch === "\r") invalid();
        }
        invalid();
      }
      const scalar = source.slice(offset).match(/^(?:True|False|None|true|false|null|[+-]?(?:inf|nan|(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?))\b/);
      if (!scalar) invalid();
      offset += scalar[0].length;
      return scalar[0];
    }
    function render(node, indent = 0) {
      if (typeof node === "string") return node;
      const pad = "  ".repeat(indent), innerPad = pad + "  ";
      const entries = node.entries.map(entry => entry.map(item => render(item, indent + 1)).join(": "));
      const comma = node.trailing ? "," : "";
      const inline = node.open + entries.join(", ") + comma + node.close;
      if (!entries.length || (!node.mapping && !inline.includes("\n") && pad.length + inline.length <= 72)) return inline;
      return node.open + "\n" + innerPad + entries.join(",\n" + innerPad) + comma + "\n" + pad + node.close;
    }
    try {
      const node = value(0);
      space();
      return offset === source.length ? render(node) : null;
    } catch (_) {
      return null;
    }
  }

  function format(text, ok = true) {
    // Leave errors, truncated output and oversized responses exactly as emitted.
    if (!ok || !text || text.length > (1 << 19) || text.includes("[output truncated]")) return text;
    const pretty = prettyCollection(text.trim());
    if (pretty !== null) {
      return text.slice(0, text.length - text.trimStart().length) + pretty + text.slice(text.trimEnd().length);
    }
    // Cells often print a label followed by one or more complete values. Only
    // reformat whole lines, never brace-like fragments embedded in ordinary text.
    return text.split(/(\r?\n)/).map(line => prettyCollection(line) ?? line).join("");
  }

  return {format};
});
