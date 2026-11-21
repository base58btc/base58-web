(function(root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) module.exports = api;
  if (root) root.CourseExecution = api;
})(typeof window !== "undefined" ? window : globalThis, function() {
  "use strict";

  function hasRemoteShellLine(code) {
    return code.split(/\r?\n/).some(function(line) {
      return line.startsWith("!");
    });
  }

  function tokenizeCommand(source) {
    const tokens = [];
    let token = "";
    let quote = "";
    let escaped = false;
    let active = false;
    for (let i = 0; i < source.length; i++) {
      const ch = source[i];
      if (escaped) {
        token += ch;
        escaped = false;
        active = true;
        continue;
      }
      if (ch === "\\" && quote !== "'") {
        if (quote === '"' && i + 1 < source.length && !['$', '`', '"', '\\', '\n'].includes(source[i + 1])) {
          token += "\\";
          active = true;
          continue;
        }
        escaped = true;
        active = true;
        continue;
      }
      if (quote) {
        if (ch === quote) quote = "";
        else token += ch;
        active = true;
        continue;
      }
      if (ch === "'" || ch === '"') {
        quote = ch;
        active = true;
        continue;
      }
      if (/\s/.test(ch)) {
        if (active) {
          tokens.push(token);
          token = "";
          active = false;
        }
        continue;
      }
      token += ch;
      active = true;
    }
    if (escaped || quote) throw new Error("Unclosed quote or escape in shell command");
    if (active) tokens.push(token);
    if (!tokens.length) throw new Error("Shell command is empty");
    return tokens;
  }

  async function interpolateToken(token, resolve) {
    const pattern = /\{([A-Za-z_][A-Za-z0-9_]*)\}/g;
    const matches = Array.from(token.matchAll(pattern));
    if (!matches.length) return token;
    let result = "";
    let offset = 0;
    for (const match of matches) {
      result += token.slice(offset, match.index);
      const value = await resolve(match[1]);
      if (!["string", "number", "boolean", "bigint"].includes(typeof value)) {
        throw new Error("Interpolated variable '" + match[1] + "' must be a scalar value");
      }
      result += String(value);
      offset = match.index + match[0].length;
    }
    return result + token.slice(offset);
  }

  // A navigation hint, not a Python dependency resolver. Only suggest an
  // earlier cell when its source contains a matching top-level definition.
  function missingPythonDefinition(output, sources) {
    const lastLine = output.trim().split(/\r?\n/).at(-1) || "";
    const name = lastLine.match(/^NameError: name '([A-Za-z_][A-Za-z0-9_]*)' is not defined\b/)?.[1];
    if (!name) return null;
    const definition = new RegExp("^(?:(?:async[ \\t]+)?def[ \\t]+" + name + "[ \\t]*\\(|class[ \\t]+" + name + "[ \\t]*[:(]|" + name + "[ \\t]*=(?!=))", "m");
    for (let index = sources.length - 1; index >= 0; index--) {
      if (definition.test(sources[index])) return {name, index};
    }
    return null;
  }

  return {hasRemoteShellLine, tokenizeCommand, interpolateToken, missingPythonDefinition};
});
