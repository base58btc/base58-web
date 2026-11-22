import {mountArchive} from "./course-nar.mjs";
const CACHE = "base58-course-archives-v1";

export async function verifiedArchive(archive, baseURL, fetcher = fetch, signal) {
  signal?.throwIfAborted();
  const url = new URL(archive.path, baseURL);
  if (!["https:", "http:"].includes(url.protocol)) throw new Error("Invalid course archive URL");
  let cache;
  try { cache = await globalThis.caches?.open(CACHE); } catch { /* Cache is optional. */ }
  let response = await cache?.match(url.href);
  signal?.throwIfAborted();
  if (!response) response = await fetcher(url.href, {credentials: "omit", signal});
  if (!response.ok) throw new Error("Unable to download course environment (" + response.status + ")");
  const bytes = new Uint8Array(await response.arrayBuffer());
  signal?.throwIfAborted();
  const hash = Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", bytes)), b => b.toString(16).padStart(2, "0")).join("");
  if (hash !== archive.sha256 || bytes.length !== archive.size) {
    await cache?.delete(url.href);
    throw new Error("Course archive failed its integrity check. Reload to retry.");
  }
  try { await cache?.put(url.href, new Response(bytes)); } catch { /* Quota limits do not prevent execution. */ }
  const stream = new Blob([bytes]).stream().pipeThrough(new DecompressionStream("gzip"));
  return new Uint8Array(await new Response(stream).arrayBuffer());
}

export async function loadArchives(files, archives, baseURL, onProgress = () => {}, download = verifiedArchive) {
  // Keep downloads parallel and bounded, and mount in a reproducible order.
  for (let offset = 0; offset < archives.length; offset += 2) {
    const batch = archives.slice(offset, offset + 2);
    const contents = await Promise.all(batch.map(archive => download(archive, baseURL)));
    for (let index = 0; index < batch.length; index++) {
      mountArchive(files, batch[index].storePath, contents[index]);
      onProgress(offset + index + 1);
    }
  }
}
