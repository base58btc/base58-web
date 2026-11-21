// Nix ARchive reader. Format and flat-entry approach follow Try Nix's MIT
// implementation: https://github.com/fzakaria/trynix/blob/main/site/js/nar.js
// See /static/licenses/trynix.txt. Bounds and path checks are enforced here.
export function parseNar(bytes) {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const decoder = new TextDecoder("utf-8", {fatal: true});
  let offset = 0;
  const entries = [];
  function blob() {
    if (offset + 8 > bytes.length) throw new Error("Truncated Nix archive");
    const size = Number(view.getBigUint64(offset, true));
    offset += 8;
    const end = offset + size;
    if (!Number.isSafeInteger(size) || end > bytes.length) throw new Error("Invalid Nix archive length");
    const data = bytes.subarray(offset, end);
    offset = end + (8 - size % 8) % 8;
    if (offset > bytes.length) throw new Error("Truncated Nix archive padding");
    return data;
  }
  const token = () => decoder.decode(blob());
  function expect(want) {
    if (token() !== want) throw new Error("Invalid Nix archive: expected " + want);
  }
  function node(path, depth = 0) {
    if (depth > 128) throw new Error("Nix archive nesting exceeds limit");
    expect("("); expect("type");
    const type = token();
    if (type === "regular") {
      let field = token();
      const executable = field === "executable";
      if (executable) { expect(""); field = token(); }
      if (field !== "contents") throw new Error("Invalid Nix file contents");
      entries.push({path, type, executable, data: blob()});
      expect(")");
    } else if (type === "symlink") {
      expect("target"); entries.push({path, type, target: token()}); expect(")");
    } else if (type === "directory") {
      entries.push({path, type});
      const names = new Set();
      for (let field = token(); field !== ")"; field = token()) {
        if (field !== "entry") throw new Error("Invalid Nix directory entry");
        expect("("); expect("name");
        const name = token();
        if (!name || name === "." || name === ".." || /[\x00/]/.test(name) || names.has(name)) throw new Error("Invalid Nix archive path");
        names.add(name);
        expect("node"); node(path ? path + "/" + name : name, depth + 1); expect(")");
      }
    } else throw new Error("Unsupported Nix archive entry");
  }
  expect("nix-archive-1"); node("");
  if (offset !== bytes.length) throw new Error("Unexpected bytes after Nix archive");
  return entries;
}

export function mountArchive(files, storePath, bytes) {
  if (!/^\/nix\/store\/[0-9a-z]{32}-[A-Za-z0-9+._?=-]+$/.test(storePath)) throw new Error("Invalid Nix store path");
  for (const entry of parseNar(bytes)) {
    const path = storePath + (entry.path ? "/" + entry.path : "");
    if (entry.type === "directory") files.mkdirp(path);
    else if (entry.type === "regular") files.writeFile(path, entry.data, {mode: entry.executable ? 0o555 : 0o444});
    else files.writeLink(path, entry.target);
  }
}
