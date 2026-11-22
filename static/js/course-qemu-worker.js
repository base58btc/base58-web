// Classic worker: Emscripten's pthread bootstrap and xterm-pty use importScripts.
const ROOT = "/static/vendor/qemu-20260913/";
const PREFIX = "BASE58_QEMU:";
let master, module, bootID, ready = false, handshake = false, buffer = "", diagnostics = "";
const decoder = new TextDecoder();
const status = text => postMessage({status:text});
const fail = error => postMessage({error:(error?.message || String(error)) + (!ready && diagnostics ? "\n" + diagnostics.slice(-2000) : "")});

function serial(data) {
  buffer += typeof data === "string" ? data : decoder.decode(Uint8Array.from(data), {stream:true});
  if (buffer.length > 16 * (1 << 20)) throw new Error("Compatibility VM response exceeded its limit");
  if (!handshake && buffer.includes("trynix: waiting for the store")) {
    handshake = true;
    master.ldisc.writeFromLower("\n");
    status("Starting Python…");
  }
  for (let end; (end = buffer.indexOf("\n")) !== -1;) {
    const line = buffer.slice(0, end).replace(/\r$/, "");
    buffer = buffer.slice(end + 1);
    if (!ready && line.includes("Kernel panic")) throw new Error("The compatibility guest could not start");
    if (!line.startsWith(PREFIX)) {
      diagnostics = (diagnostics + line + "\n").slice(-4000);
      if (!ready && line.trim()) console.debug("QEMU boot:", line);
      continue;
    }
    const frame = JSON.parse(line.slice(PREFIX.length));
    if (frame.ready === 1 && !ready) {
      ready = true;
      postMessage({id:bootID, result:{ready:true}});
    } else if (ready && Number.isSafeInteger(frame.id) && frame.result) postMessage(frame);
    else throw new Error("Invalid compatibility VM response");
  }
}

async function bytes(url) {
  const response = await fetch(url);
  if (!response.ok) throw new Error("Unable to load compatibility runtime: " + url);
  return new Uint8Array(await response.arrayBuffer());
}

async function boot({config, id}) {
  bootID = id;
  status("Loading compatibility runtime…");
  importScripts(ROOT + "xterm-pty.js");
  const pty = openpty();
  master = pty.master;
  master.onWrite(([data, drawn]) => { try { serial(data); } catch (error) { fail(error); } finally { drawn?.(); } });
  const guestNames = ["bzImage", "initramfs.cpio.gz", "bios-256k.bin", "vgabios-stdvga.bin", "kvmvapic.bin", "linuxboot_dma.bin"];
  const guest = await Promise.all(guestNames.map(async name => [name, await bytes(ROOT + "guest/" + name)]));
  const {default:createModule} = await import(ROOT + "out.js");
  const {courseQemuWasm} = await import("./course-qemu-memory.mjs");
  const wasmBinary = courseQemuWasm(await bytes(ROOT + "qemu-system-x86_64.wasm"));
  let fsReady, fsFailed;
  const filesystem = new Promise((resolve, reject) => { fsReady = resolve; fsFailed = reject; });
  const args = ["-nographic", "-cpu", "Haswell-v4,+rdrand", "-device", "virtio-rng-pci",
    "-m", "512M", "-accel", "tcg,tb-size=128", "-L", "/pack/", "-nic", "none",
    "-kernel", "/pack/bzImage", "-initrd", "/pack/initramfs.cpio.gz",
    "-fsdev", "local,id=store0,path=/share,security_model=none",
    "-device", "virtio-9p-pci,fsdev=store0,mount_tag=store0,ioeventfd=off",
    "-append", "console=ttyS0 rdinit=/init loglevel=4 mitigations=off"];
  createModule({arguments:args, pty:pty.slave, wasmBinary, INITIAL_MEMORY:1536 * 1024 * 1024,
    mainScriptUrlOrBlob:new URL(ROOT + "out.js", location.href).href,
    locateFile:file => ROOT + file,
    onAbort:reason => {
      const error = new Error(reason === "OOM" ? "The course environment ran out of memory. Try a current desktop browser." : "Compatibility VM stopped: " + reason);
      fsFailed(error); fail(error);
    },
    preRun:[mod => { mod.addRunDependency("course-files"); fsReady(mod); }],
  }).catch(error => { fsFailed(error); fail(error); });
  module = await filesystem;
  const fs = module.FS;
  fs.mkdirTree("/pack");
  fs.mkdirTree("/share/nix/store");
  fs.mkdirTree("/share/course");
  fs.mkdirTree("/share/bin");
  for (const [name, data] of guest) fs.writeFile("/pack/" + name, data, {canOwn:true});
  const {loadArchives} = await import("./course-archives.mjs");
  const {qemuLinkTarget} = await import("./course-qemu-files.mjs");
  const store = {
    mkdirp:path => fs.mkdirTree("/share" + path),
    writeFile:(path, data, options) => {
      const dest = "/share" + path;
      fs.mkdirTree(dest.slice(0, dest.lastIndexOf("/")));
      fs.writeFile(dest, data); fs.chmod(dest, options.mode);
    },
    writeLink:(path, target) => {
      const dest = "/share" + path;
      fs.mkdirTree(dest.slice(0, dest.lastIndexOf("/")));
      fs.symlink(qemuLinkTarget(dest, target), dest);
    },
  };
  const {manifest, files, assetBaseURL} = config;
  await loadArchives(store, manifest.archives, assetBaseURL, done => status(`Loading course tools (${done}/${manifest.archives.length})…`));
  for (const [path, content] of Object.entries(files)) {
    if (path.startsWith("/") || path.split("/").some(part => !part || part === "." || part === "..")) throw new Error("Invalid course source path");
    const dest = "/share/course/" + path;
    fs.mkdirTree(dest.slice(0, dest.lastIndexOf("/")));
    fs.writeFile(dest, content);
  }
  fs.writeFile("/share/course/.base58-setup.sh", manifest.setup);
  fs.writeFile("/share/course/.base58-python.py", await bytes("/static/guest/course-python.py"));
  fs.writeFile("/share/course/.base58-qemu.py", await bytes("/static/guest/course-qemu.py"));
  const quote = value => "'" + value.replaceAll("'", "'\\''") + "'";
  fs.writeFile("/share/manifest", [
    "stty raw -echo",
    "ifconfig lo 127.0.0.1 up",
    "ln -s /share/course /course",
    "mkdir -p /data/workspace",
    "cp -R /course/. /data/workspace/",
    "cd /data/workspace",
    "exec " + quote(manifest.shell) + " --noprofile --norc -c " + quote('. /course/.base58-setup.sh; eval "${shellHook:-}" || exit $?; exec python3 /course/.base58-qemu.py'),
    "",
  ].join("\n"));
  status("Starting compatibility VM…");
  module.removeRunDependency("course-files");
}

self.onmessage = ({data}) => {
  if (data.type === "boot") { boot(data).catch(fail); return; }
  if (!ready) { fail(new Error("Compatibility VM is not ready")); return; }
  // JSON escapes control characters. Raw tty mode avoids its 4 KiB line limit.
  const json = JSON.stringify(data).replace(/[\u007f-\uffff]/g, char => "\\u" + char.charCodeAt(0).toString(16).padStart(4, "0"));
  master.ldisc.writeFromLower(json + "\n");
};
