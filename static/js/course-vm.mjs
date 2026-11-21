import {mountArchive} from "./course-nar.mjs";
import {CommandStorage} from "./course-command-storage.mjs";
import {CommandJournal, newEvent, replayCommands} from "./course-history.mjs";
import {PythonSession, pythonCellRequest} from "./course-python.mjs";

const SDK = "/static/vendor/unkernel-0.2.0-261119-040e142/web.mjs";
const CACHE = "base58-course-archives-v1";
const MAX_OUTPUT = 1 << 20;
export function boundedOutput(value) {
  const text = String(value || ""), bytes = new TextEncoder().encode(text);
  return bytes.length <= MAX_OUTPUT ? text : new TextDecoder().decode(bytes.subarray(0, MAX_OUTPUT - 32)) + "\n[output truncated]\n";
}

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
    if (this.closing) return Promise.reject(new DOMException("Workspace stopped", "AbortError"));
    if (this.replayFailure) return Promise.reject(this.replayFailure);
    if (!this.booting) {
      this.failed = false;
      this.sessionAbort = new AbortController();
      this.booting = this.boot(this.sessionAbort.signal).catch(async error => {
        if (!this.replayFailure && !this.sessionAbort.signal.aborted) {
          try { await this.dispose(); } catch { /* Preserve the initialization error. */ }
        }
        throw error;
      });
    }
    return this.booting;
  }

  // Prepare the same VM and interpreter that Play will use. No synthetic cell
  // is executed or recorded; the interpreter's ready frame is the handshake.
  prepare({python = false} = {}) {
    return this.enqueue(async () => {
      const preparing = (async () => {
        await this.ready();
        if (python && !this.pythonSession) {
          this.onStatus("Starting Python…");
          await this.startPython(AbortSignal.any([this.sessionAbort.signal, AbortSignal.timeout(120000)]));
        }
        this.sessionAbort.signal.throwIfAborted();
        this.onStatus("Workspace ready");
      })();
      this.preparing = preparing;
      try { await preparing; }
      catch (error) {
        if (this.booting && !this.replayFailure && !this.sessionAbort?.signal.aborted) {
          try { await this.dispose(); } catch { /* Preserve the startup error. */ }
        }
        throw error;
      } finally { this.preparing = null; }
    });
  }

  openHistory(signal) {
    if (!this.openingHistory) this.openingHistory = this.loadHistory(signal);
    return this.openingHistory;
  }

  async loadHistory(signal) {
    signal.throwIfAborted();
    this.onStatus("Loading course command history…");
    let config = this.environment;
    if (!config) {
      const response = await fetch(this.environmentURL, {credentials:"same-origin", cache:"no-store", signal});
      if (!response.ok) throw new Error((await response.text()).trim() || "Course environment is unavailable");
      config = await response.json();
    }
    this.config = config;
    const {manifest, workspace, journalURL} = config;
    if (![1, 2].includes(manifest.format) || manifest.system !== "aarch64-linux") throw new Error("Unsupported course environment");
    this.workspaceName = "base58-course-" + workspace;
    this.release = await workspaceLock(this.workspaceName);
    signal.throwIfAborted();
    this.storage = new CommandStorage({workspace, journalURL, csrfToken:this.csrfToken || config.csrfToken || "", onStatus:this.onStatus});
    await this.storage.open();
    this.journal = new CommandJournal(this.storage, this.onHistory);
    await this.journal.load();
    signal.throwIfAborted();
  }

  async boot(signal) {
    signal.throwIfAborted();
    if (typeof WebAssembly.Suspending !== "function" || typeof WebAssembly.promising !== "function") {
      throw new Error("The course VM needs a browser with WebAssembly JSPI support.");
    }
    await this.openHistory(signal);
    const {manifest, files, assetBaseURL, journalURL} = this.config;
    this.sdk = await import(SDK);
    signal.throwIfAborted();
    const store = new this.sdk.MemoryFs(), sources = new this.sdk.MemoryFs();
    store.mkdirp("/nix/store");
    for (const [path, content] of Object.entries(files).sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0)) {
      if (path.startsWith("/") || path.split("/").some(p => !p || p === "." || p === "..")) throw new Error("Invalid course source path");
      sources.writeFile("/" + path, content);
    }
    sources.writeFile("/.base58-setup.sh", manifest.setup);
    const pythonRunner = await fetch("/static/guest/course-python.py", {signal});
    if (!pythonRunner.ok) throw new Error("Unable to load course Python runner");
    sources.writeFile("/.base58-python.py", await pythonRunner.text());
    await loadArchives(store, manifest.archives, assetBaseURL, done => {
      signal.throwIfAborted();
      this.onStatus("Loading course tools (" + done + "/" + manifest.archives.length + ")…");
    }, (archive, baseURL) => verifiedArchive(archive, baseURL, fetch, signal));
    signal.throwIfAborted();
    this.mounts = {"/course-store":{fs:store,readOnly:true}, "/course":{fs:sources,readOnly:true}};
    this.shell = manifest.shell;
    // An execution without a durable result is kept in the audit log but is not
    // replayed. Its partial machine state disappears with the old VM.
    if (this.journal.state.pending) {
      await this.journal.append(newEvent("interrupted", {commandId:this.journal.state.pending}));
    }
    await this.rebuild(this.journal.state.head, signal);
    this.onStatus(journalURL ? "History replayed · commands saved to your account" : "History replayed · commands saved in this browser");
  }

  async startKernel(signal) {
    signal?.throwIfAborted();
    this.onStatus("Starting a fresh course environment…");
    if (!this.kernelModule) {
      this.kernelModule = await this.sdk.loadKernelModule(new URL("unkernel.wasm", new URL(SDK, location.href)));
      this.diskTemplate = this.sdk.decodeKwsparse(await this.sdk.gunzipIfNeeded(this.sdk.bundledBoot(this.kernelModule).datadisk));
    }
    signal?.throwIfAborted();
    // All guest writes stay in memory. Each rebuild starts from the bundled
    // empty data disk, never a previous session's files or saved machine state.
    const disk = new this.sdk.CowDisk(this.sdk.SparseBase.fromKwsparse(this.diskTemplate));
    this.vm = await this.sdk.createKernel({
      kernel:this.kernelModule, memoryMiB:1024, worker:false, mounts:this.mounts,
      disks:[disk], signal,
    });
    signal?.throwIfAborted();
    const setupSignal = AbortSignal.any([AbortSignal.timeout(60000), ...(signal ? [signal] : [])]);
    let result = await this.vm.exec("/bin/sh",["-c","ln -s /course-store/nix /nix; mkdir -p /data/workspace; cp -R /course/. /data/workspace/"], {signal:setupSignal});
    if (result.exitCode !== 0) throw new Error(result.stderr || "Unable to initialize course workspace");
    result = await this.vm.exec(this.shell,["--noprofile","--norc","-c",'. /course/.base58-setup.sh; eval "${shellHook:-}" || exit $?; export -p > /tmp/base58-environment.sh; declare -f >> /tmp/base58-environment.sh'],{cwd:"/data/workspace",signal:setupSignal,maxBuffer:MAX_OUTPUT});
    if (result.exitCode !== 0) throw new Error(result.stderr || result.stdout || "Course initialization failed");
  }

  async stopKernel() {
    try { await this.pythonSession?.close(); }
    finally {
      this.pythonSession = null;
      try { if (this.vm) await this.vm.stop(); }
      finally { this.vm = null; }
    }
  }

  async startPython(signal) {
    signal.throwIfAborted();
    if (!this.pythonSession) {
      const process = await this.vm.spawn(this.shell,["--noprofile","--norc","-c",'. /tmp/base58-environment.sh; exec python3 /course/.base58-python.py --stdio'],{pty:false,cwd:"/data/workspace",signal});
      this.pythonSession = new PythonSession(process);
    }
    await this.pythonSession.waitUntilReady(signal);
    return this.pythonSession;
  }

  async runCommand(command, timeoutMs, parentSignal) {
    const timeout = AbortSignal.timeout(timeoutMs || command.timeoutMs || (command.program === "python3" ? 120000 : 30000));
    const signal = parentSignal ? AbortSignal.any([timeout, parentSignal]) : timeout;
    try {
      const request = pythonCellRequest(command);
      if (request !== null) {
        try {
          const python = await this.startPython(signal);
          const result = await python.execute(request.code, signal, request.check);
          return {...result, stdout:boundedOutput(result.stdout)};
        } catch (error) {
          this.failed = true;
          throw error;
        }
      }
      const result = await this.vm.exec(this.shell,["--noprofile","--norc","-c",'. /tmp/base58-environment.sh; exec "$@"',"course-command",command.program,...command.args],{cwd:"/data/workspace",signal,maxBuffer:MAX_OUTPUT});
      return {stdout:boundedOutput(result.stdout),stderr:boundedOutput(result.stderr),exitCode:result.exitCode ?? -1,timedOut:false};
    } catch (error) {
      return {stdout:"",stderr:signal.aborted ? "Command timed out.\n" : boundedOutput(error.message || error),exitCode:-1,timedOut:signal.aborted};
    }
  }

  async rebuild(head, signal) {
    this.replayFailure = null;
    this.failed = false;
    try {
      await this.stopKernel();
      await this.startKernel(signal);
      const entries = replayCommands(this.journal.state, head);
      for (const [index, entry] of entries.entries()) {
        signal?.throwIfAborted();
        this.onStatus(`Replaying command ${index + 1}/${entries.length}…`);
        const result = await this.runCommand(entry.command, undefined, signal);
        signal?.throwIfAborted();
        // Generated output can differ. A changed exit status or timeout means
        // later commands may depend on work that was not reconstructed.
        if (this.failed || result.timedOut || result.exitCode !== entry.result.exitCode || entry.result.timedOut) {
          throw new Error(`Replay stopped at command ${index + 1} (${entry.command.blockId || entry.command.program}). ` +
            (result.stderr || "The command's outcome changed.") + " Your history is intact. Undo commands to return to an earlier point.");
        }
      }
      this.failed = false;
    } catch (error) {
      if (signal?.aborted) throw error;
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
        const {phase, ...recordedResult} = result;
        await this.journal.append(newEvent("result",{commandId:event.id,result:recordedResult}));
        if (result.timedOut || this.failed) {
          this.failed = true;
          await this.stopKernel();
          this.replayFailure = new Error(result.timedOut ? "The command timed out. Undo it before continuing; its partial state cannot be reliably replayed." : result.stderr);
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

  checkPython(source, check, metadata = {}) {
    const bytes = new TextEncoder().encode(JSON.stringify({code:source, check}));
    if (bytes.length > 16384) throw new Error("Keep Python challenges and checks under 16 KiB combined.");
    const encoded = btoa(Array.from(bytes, byte => String.fromCharCode(byte)).join(""));
    return this.execute({program:"python3", args:["/course/.base58-python.py", "--check", encoded]},
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

  reset({python = false} = {}) {
    // Cancel background replay, but let student commands finish recording.
    if (this.preparing) this.sessionAbort?.abort();
    return this.enqueue(async () => {
      await this.close();
      this.failed = false;
      this.sessionAbort = new AbortController();
      const signal = this.sessionAbort.signal;
      try {
        // Read and archive history before booting. Reset must also work when
        // replay is slow or fails, and when the workspace is currently stopped.
        await this.openHistory(signal);
        signal.throwIfAborted();
        this.onStatus("Resetting workspace…");
        if (this.journal.state.pending) {
          await this.journal.append(newEvent("interrupted", {commandId:this.journal.state.pending}));
        }
        signal.throwIfAborted();
        await this.journal.append(newEvent("reset"));
        signal.throwIfAborted();
        // The reset is durable before any fresh VM starts executing.
        await this.ready();
        if (python) {
          this.onStatus("Starting Python…");
          await this.startPython(AbortSignal.any([this.sessionAbort.signal, AbortSignal.timeout(120000)]));
        }
        this.sessionAbort.signal.throwIfAborted();
        this.onStatus("Workspace reset · previous commands archived. Rerun any prerequisite cells.");
      } catch (error) {
        try { await this.close(); } catch { /* Preserve the reset error. */ }
        throw error;
      }
    });
  }

  saveAndStop() {
    // Background preparation has no new student work to finish. Cancel it so
    // leaving the course doesn't have to wait for all the tools to download.
    if (this.preparing) this.sessionAbort?.abort();
    return this.enqueue(async () => {
      const incomplete = this.failed;
      await this.close();
      this.onStatus(incomplete ? "Stopped · reopen to replay recorded commands" : this.config?.journalURL ? "Command history saved · resume on any device by replaying it" : "Command history saved in this browser · resume replays it");
    });
  }

  close() {
    if (this.closing) return this.closing;
    this.sessionAbort?.abort();
    this.closing = (async () => {
      // Boot/spawn may still allocate resources after an await. Let them settle
      // before cleanup so a closed page cannot leave a late VM holding its lock.
      await Promise.allSettled([this.booting, this.preparing, this.openingHistory]);
      await this.dispose();
    })().finally(() => { this.closing = null; });
    return this.closing;
  }

  async dispose() {
    try { await this.stopKernel(); }
    finally {
      try { await this.storage?.release(); }
      finally {
        this.storage?.db?.close();
        this.release?.(); this.release = null; this.booting = null; this.replayFailure = null;
        this.openingHistory = null;
      }
    }
  }
}
