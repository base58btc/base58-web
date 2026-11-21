import {mountArchive} from "./course-nar.mjs";
import {CommandStorage} from "./course-command-storage.mjs";
import {CommandJournal, newEvent, replayCommands} from "./course-history.mjs";

const SDK = "/static/vendor/unkernel-0.2.0-261119-040e142/web.mjs";
const CACHE = "base58-course-archives-v1";
const MAX_OUTPUT = 1 << 20;
export function boundedOutput(value) {
  const text = String(value || ""), bytes = new TextEncoder().encode(text);
  return bytes.length <= MAX_OUTPUT ? text : new TextDecoder().decode(bytes.subarray(0, MAX_OUTPUT - 32)) + "\n[output truncated]\n";
}

export async function verifiedArchive(archive, baseURL, fetcher = fetch) {
  const url = new URL(archive.path, baseURL);
  if (!["https:", "http:"].includes(url.protocol)) throw new Error("Invalid course archive URL");
  let cache;
  try { cache = await globalThis.caches?.open(CACHE); } catch { /* Cache is optional. */ }
  let response = await cache?.match(url.href);
  if (!response) response = await fetcher(url.href, {credentials: "omit"});
  if (!response.ok) throw new Error("Unable to download course environment (" + response.status + ")");
  const bytes = new Uint8Array(await response.arrayBuffer());
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

// Serialize commands even when students run several cells at once. A rejection
// must not poison the next cell's turn.
export function commandQueue() {
  let tail = Promise.resolve();
  return task => {
    const next = tail.then(task);
    tail = next.catch(() => {});
    return next;
  };
}

async function workspaceLock(name) {
  if (!navigator.locks) throw new Error("This browser cannot safely save the course workspace. Please use a current browser.");
  let release;
  const held = new Promise(resolve => { release = resolve; });
  await new Promise((resolve, reject) => {
    navigator.locks.request(name, {ifAvailable: true}, async lock => {
      if (!lock) { reject(new Error("This course environment is already open in another tab. Close that tab first.")); return; }
      resolve();
      await held;
    }).catch(reject);
  });
  return release;
}

export class CourseVM {
  constructor({environmentURL, environment, csrfToken = "", onStatus = () => {}, onHistory = () => {}}) {
    Object.assign(this, {environmentURL, environment, csrfToken, onStatus, onHistory});
    this.enqueue = commandQueue();
  }

  ready() {
    if (this.replayFailure) return Promise.reject(this.replayFailure);
    if (!this.booting) this.booting = this.boot().catch(async error => {
      if (!this.replayFailure) {
        try { await this.close(); } catch { /* Preserve the initialization error. */ }
      }
      throw error;
    });
    return this.booting;
  }

  async boot() {
    if (typeof WebAssembly.Suspending !== "function" || typeof WebAssembly.promising !== "function") {
      throw new Error("The course VM needs a browser with WebAssembly JSPI support.");
    }
    this.onStatus("Loading course command history…");
    let config = this.environment;
    if (!config) {
      const response = await fetch(this.environmentURL, {credentials:"same-origin", cache:"no-store"});
      if (!response.ok) throw new Error((await response.text()).trim() || "Course environment is unavailable");
      config = await response.json();
    }
    this.config = config;
    const {manifest, files, assetBaseURL, workspace, journalURL} = config;
    if (manifest.format !== 1 || manifest.system !== "aarch64-linux") throw new Error("Unsupported course environment");
    this.workspaceName = "base58-course-" + workspace;
    this.release = await workspaceLock(this.workspaceName);
    this.storage = new CommandStorage({workspace, journalURL, csrfToken:this.csrfToken || config.csrfToken || "", onStatus:this.onStatus});
    await this.storage.open();
    this.journal = new CommandJournal(this.storage, this.onHistory);
    await this.journal.load();
    this.sdk = await import(SDK);
    const store = new this.sdk.MemoryFs(), sources = new this.sdk.MemoryFs();
    store.mkdirp("/nix/store");
    for (const [path, content] of Object.entries(files).sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0)) {
      if (path.startsWith("/") || path.split("/").some(p => !p || p === "." || p === "..")) throw new Error("Invalid course source path");
      sources.writeFile("/" + path, content);
    }
    sources.writeFile("/.base58-setup.sh", manifest.setup);
    const pythonRunner = await fetch("/static/guest/course-python.py");
    if (!pythonRunner.ok) throw new Error("Unable to load course Python runner");
    sources.writeFile("/.base58-python.py", await pythonRunner.text());
    await loadArchives(store, manifest.archives, assetBaseURL, done => {
      this.onStatus("Loading course tools (" + done + "/" + manifest.archives.length + ")…");
    });
    this.mounts = {"/course-store":{fs:store,readOnly:true}, "/course":{fs:sources,readOnly:true}};
    this.shell = manifest.shell;
    // An execution without a durable result is kept in the audit log but is not
    // replayed. Its partial machine state disappears with the old VM.
    if (this.journal.state.pending) {
      await this.journal.append(newEvent("interrupted", {commandId:this.journal.state.pending}));
    }
    await this.rebuild(this.journal.state.head);
    this.onStatus(journalURL ? "History replayed · commands saved to your account" : "History replayed · commands saved in this browser");
  }

  async startKernel() {
    this.onStatus("Starting a fresh course environment…");
    if (!this.kernelModule) {
      this.kernelModule = await this.sdk.loadKernelModule(new URL("unkernel.wasm", new URL(SDK, location.href)));
      this.diskTemplate = this.sdk.decodeKwsparse(await this.sdk.gunzipIfNeeded(this.sdk.bundledBoot(this.kernelModule).datadisk));
    }
    // All guest writes stay in memory. Each rebuild starts from the bundled
    // empty data disk, never a previous session's files or saved machine state.
    const disk = new this.sdk.CowDisk(this.sdk.SparseBase.fromKwsparse(this.diskTemplate));
    this.vm = await this.sdk.createKernel({
      kernel:this.kernelModule, memoryMiB:512, worker:false, mounts:this.mounts,
      disks:[disk],
    });
    let result = await this.vm.exec("/bin/sh",["-c","ln -s /course-store/nix /nix; mkdir -p /data/workspace; cp -R /course/. /data/workspace/"]);
    if (result.exitCode !== 0) throw new Error(result.stderr || "Unable to initialize course workspace");
    result = await this.vm.exec(this.shell,["--noprofile","--norc","-c",'. /course/.base58-setup.sh; eval "${shellHook:-}" || exit $?; export -p > /tmp/base58-environment.sh; declare -f >> /tmp/base58-environment.sh'],{cwd:"/data/workspace",signal:AbortSignal.timeout(60000),maxBuffer:MAX_OUTPUT});
    if (result.exitCode !== 0) throw new Error(result.stderr || result.stdout || "Course initialization failed");
  }

  async stopKernel() {
    try { if (this.vm) await this.vm.stop(); }
    finally { this.vm = null; }
  }

  async runCommand(command, timeoutMs) {
    const signal = AbortSignal.timeout(timeoutMs || command.timeoutMs || (command.program === "python3" ? 120000 : 30000));
    try {
      const result = await this.vm.exec(this.shell,["--noprofile","--norc","-c",'. /tmp/base58-environment.sh; exec "$@"',"course-command",command.program,...command.args],{cwd:"/data/workspace",signal,maxBuffer:MAX_OUTPUT});
      return {stdout:boundedOutput(result.stdout),stderr:boundedOutput(result.stderr),exitCode:result.exitCode ?? -1,timedOut:false};
    } catch (error) {
      return {stdout:"",stderr:signal.aborted ? "Command timed out.\n" : boundedOutput(error.message || error),exitCode:-1,timedOut:signal.aborted};
    }
  }

  async rebuild(head) {
    this.replayFailure = null;
    try {
      await this.stopKernel();
      await this.startKernel();
      const entries = replayCommands(this.journal.state, head);
      for (const [index, entry] of entries.entries()) {
        this.onStatus(`Replaying command ${index + 1}/${entries.length}…`);
        const result = await this.runCommand(entry.command);
        // Generated output can differ. A changed exit status or timeout means
        // later commands may depend on work that was not reconstructed.
        if (result.timedOut || result.exitCode !== entry.result.exitCode || entry.result.timedOut) {
          throw new Error(`Replay stopped at command ${index + 1} (${entry.command.blockId || entry.command.program}). ` +
            (result.stderr || "The command's outcome changed.") + " Your history is intact. Undo commands to return to an earlier point.");
        }
      }
      this.failed = false;
    } catch (error) {
      this.failed = true;
      this.replayFailure = error;
      await this.stopKernel();
      this.onStatus(error.message);
      throw error;
    }
  }

  execute(command, {timeoutMs = 30000, source = "", lessonPath = "", blockId = ""} = {}) {
    return this.enqueue(async () => {
      await this.ready();
      if (this.failed) throw new Error("The last command could not finish saving. Save and stop, then reopen the environment to recover.");
      if (typeof command.program !== "string" || !command.program || !Array.isArray(command.args) || command.args.some(arg => typeof arg !== "string")) throw new Error("Invalid course command");
      if (!Number.isInteger(timeoutMs) || timeoutMs < 1 || timeoutMs > 120000) throw new Error("Invalid command timeout");
      await this.storage.claim();
      const event = newEvent("command",{parent:this.journal.state.head,command:{...command,source,lessonPath,blockId,timeoutMs}});
      try {
        // Commit source and resolved argv BEFORE dispatch, including failures.
        await this.journal.append(event);
        this.onStatus("Running command…");
        const result = await this.runCommand(event.command);
        await this.journal.append(newEvent("result",{commandId:event.id,result}));
        if (result.timedOut) {
          this.failed = true;
          await this.stopKernel();
          this.replayFailure = new Error("The command timed out. Undo it before continuing; its partial state cannot be reliably replayed.");
          this.onStatus(this.replayFailure.message);
        } else {
          this.onStatus(this.config.journalURL ? "Command saved to your account" : "Command saved in this browser");
        }
        return result;
      } catch (error) {
        this.failed = true;
        await this.stopKernel();
        this.onStatus("Saving did not finish · reopen to recover the last recorded command");
        throw error;
      }
    });
  }

  executePython(source, metadata = {}) {
    const bytes = new TextEncoder().encode(source);
    // Source and resolved argv both appear in the 64 KiB journal event.
    if (bytes.length > 16384) throw new Error("Keep Python cells under 16 KiB; put larger helpers in course files.");
    const encoded = btoa(Array.from(bytes, byte => String.fromCharCode(byte)).join(""));
    return this.execute({program:"python3", args:["/course/.base58-python.py", "--execute", encoded]},
      {...metadata, source, timeoutMs:120000});
  }

  undo() {
    return this.enqueue(async () => {
      // A divergent replay must still let the student move back through history.
      if (!this.replayFailure) await this.ready();
      if (this.journal.state.pending) throw new Error("Reopen the environment before undoing an unfinished command");
      const entry = this.journal.state.commands.get(this.journal.state.head);
      if (!entry) return;
      await this.storage.claim();
      const head = entry.parent || "";
      // Persist the requested cursor first. If rebuilding fails, another Undo
      // can move farther back without losing or duplicating any journal entries.
      await this.journal.append(newEvent("checkout",{head}));
      await this.rebuild(head);
      this.booting = Promise.resolve();
      this.onStatus("Last command undone · earlier commands replayed; generated values may differ");
      return entry;
    });
  }

  saveAndStop() {
    return this.enqueue(async () => {
      const incomplete = this.failed;
      await this.close();
      this.onStatus(incomplete ? "Stopped · reopen to replay recorded commands" : this.config?.journalURL ? "Command history saved · resume on any device by replaying it" : "Command history saved in this browser · resume replays it");
    });
  }

  async close() {
    try { await this.stopKernel(); }
    finally {
      try { await this.storage?.release(); }
      finally {
        this.storage?.db?.close();
        this.release?.(); this.release = null; this.booting = null; this.replayFailure = null;
      }
    }
  }
}
