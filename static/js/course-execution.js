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

  // Mask strings/comments and join continued lines before looking for bindings.
  // This is deliberately conservative: dynamic globals and wildcard imports
  // can't identify a dependable destination without executing student code.
  function pythonDefinitions(source) {
    const lines = [];
    let line = "", depth = 0;
    for (let i = 0; i < source.length; i++) {
      const ch = source[i];
      if (ch === "#") {
        while (i + 1 < source.length && source[i + 1] !== "\n") i++;
      } else if (ch === "'" || ch === '"') {
        const quote = source.slice(i, i + 3) === ch.repeat(3) ? ch.repeat(3) : ch;
        line += "?";
        i += quote.length;
        while (i < source.length) {
          if (source[i] === "\\") { i += 2; continue; }
          if (source.slice(i, i + quote.length) === quote) { i += quote.length; break; }
          i++;
        }
        i--;
      } else if (ch === "\\" && /\r?\n/.test(source.slice(i + 1, i + 3))) {
        i += source[i + 1] === "\r" ? 2 : 1;
        line += " ";
      } else if (ch === "\n" && depth === 0) {
        lines.push(line); line = "";
      } else {
        if ("([{".includes(ch)) depth++;
        if (")]}".includes(ch)) depth = Math.max(0, depth - 1);
        line += ch === "\n" ? " " : ch;
      }
    }
    lines.push(line);
    const names = new Set();
    const identifier = /^[A-Za-z_][A-Za-z0-9_]*$/;
    function targetNames(target) {
      target = target.trim();
      // A bare annotation does not initialize a variable. This helper is only
      // called for an actual assignment, so an annotation here has a value.
      const annotated = target.match(/^([A-Za-z_][A-Za-z0-9_]*)\s*:/);
      if (annotated) { names.add(annotated[1]); return; }
      if (/\w\s*[[(]/.test(target)) return; // Subscripts/calls are not name bindings.
      const parts = target.replace(/[()[\]*]/g, "").split(",").map(part => part.trim()).filter(Boolean);
      if (parts.length && parts.every(part => identifier.test(part))) parts.forEach(name => names.add(name));
    }
    for (const logicalLine of lines) {
      if (!/^\S/.test(logicalLine)) continue; // Locals in functions/classes aren't shared cell globals.
      for (const raw of logicalLine.split(";")) {
        const statement = raw.trim();
        const declaration = statement.match(/^(?:(?:async\s+)?def|class)\s+([A-Za-z_][A-Za-z0-9_]*)\b/);
        if (declaration) { names.add(declaration[1]); break; }
        const imported = statement.match(/^(?:from\s+[.\w]+\s+)?import\s+(.+)$/);
        if (imported) {
          for (const item of imported[1].replace(/[()]/g, "").split(",")) {
            const binding = item.trim().match(/^([A-Za-z_]\w*(?:\.\w+)*)(?:\s+as\s+([A-Za-z_]\w*))?$/);
            if (binding) names.add(binding[2] || binding[1].split(".")[0]);
          }
          continue;
        }
        let start = 0, nesting = 0;
        for (let i = 0; i < statement.length; i++) {
          const ch = statement[i];
          if ("([{".includes(ch)) nesting++;
          if (")]}".includes(ch)) nesting--;
          if (ch === "=" && nesting === 0 && !/[=!:<>+*/%&|^@-]/.test(statement[i - 1] || "") && statement[i + 1] !== "=") {
            targetNames(statement.slice(start, i));
            start = i + 1;
          }
        }
      }
    }
    return [...names];
  }

  // Only link a NameError to an earlier cell that supplies that name.
  function missingPythonDefinition(output, sources) {
    const lastLine = output.trim().split(/\r?\n/).at(-1) || "";
    const name = lastLine.match(/^NameError: name '([A-Za-z_][A-Za-z0-9_]*)' is not defined\b/)?.[1];
    if (!name) return null;
    for (let index = sources.length - 1; index >= 0; index--) {
      if (pythonDefinitions(sources[index]).includes(name)) return {name, index};
    }
    return null;
  }

  return {hasRemoteShellLine, tokenizeCommand, interpolateToken, pythonDefinitions, missingPythonDefinition};
});
