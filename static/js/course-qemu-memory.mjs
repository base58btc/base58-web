// The pinned Emscripten module imports a fixed 2300 MiB shared memory. Its
// static data/stack fit far below 1 GiB; the rest is the allocator's heap. We
// give this guest 512 MiB RAM + 128 MiB TCG and a 1536 MiB emulator heap. MEMFS
// file contents live in separate JS arrays. Lower the *import minimum* only;
// Module.INITIAL_MEMORY supplies the smaller, still fixed, actual allocation.
// This is specific to the hash-pinned engine: reject any unexpected binary.
export function courseQemuWasm(original) {
  const bytes = original.slice();
  const signature = [3, 101, 110, 118, 6, 109, 101, 109, 111, 114, 121, 2, 3, 192, 159, 2, 192, 159, 2];
  let found = -1;
  for (let i = 0; i <= bytes.length - signature.length; i++) {
    if (!signature.every((byte, offset) => bytes[i + offset] === byte)) continue;
    if (found !== -1) throw new Error("Unexpected QEMU memory declaration");
    found = i;
  }
  if (found === -1) throw new Error("Unexpected QEMU memory declaration");
  // 36800 -> 24576 wasm pages. Both unsigned LEB128 encodings have 3 bytes.
  bytes.set([128, 192, 1], found + 13);
  return bytes;
}
