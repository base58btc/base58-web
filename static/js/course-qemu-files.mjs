// Emscripten readlink expands relative targets, but stat reports their original
// length. 9p then truncates the expanded target. Store it in absolute form so
// the reported length and contents agree (the guest also mounts at /share).
export function qemuLinkTarget(link, target) {
  if (target.startsWith("/")) return target;
  const parts = link.split("/").slice(0, -1);
  for (const part of target.split("/")) {
    if (!part || part === ".") continue;
    if (part === "..") parts.pop();
    else parts.push(part);
  }
  return parts.join("/");
}
