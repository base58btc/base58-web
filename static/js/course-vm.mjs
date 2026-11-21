import {mountArchive} from "./course-nar.mjs";
import {CheckpointStorage} from "./course-checkpoints.mjs";
import {CommandJournal, newEvent} from "./course-history.mjs";

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
    if (!this.booting) this.booting = this.boot().catch(async error => {
      try { await this.close(); } catch { /* Preserve the initialization error. */ }
      throw error;
    });
    return this.booting;
  }

  async boot() {
    if (typeof WebAssembly.Suspending !== "function" || typeof WebAssembly.promising !== "function") {
      throw new Error("The course VM needs a browser with WebAssembly JSPI support.");
    }
    this.onStatus("Loading saved course environment…");
    let config = this.environment;
    if (!config) {
      const response = await fetch(this.environmentURL, {credentials:"same-origin", cache:"no-store"});
      if (!response.ok) throw new Error((await response.text()).trim() || "Course environment is unavailable");
      config = await response.json();
    }
    this.config = config;
    const {manifest, files, assetBaseURL, workspace, journalURL} = config;
    if (manifest.format !== 1 || manifest.system !== "aarch64-linux") throw new Error("Unsupported course environment");
    this.diskName = "base58-course-" + workspace;
    this.release = await workspaceLock(this.diskName);
    this.storage = new CheckpointStorage({workspace, journalURL, csrfToken:this.csrfToken || config.csrfToken || "", onStatus:this.onStatus});
    await this.storage.open();
    this.journal = new CommandJournal(this.storage, this.onHistory);
    await this.journal.load();
    this.sdk = await import(SDK);
    const store = new this.sdk.MemoryFs(), sources = new this.sdk.MemoryFs();
    store.mkdirp("/nix/store");
    for (const [path, content] of Object.entries(files)) {
      if (path.startsWith("/") || path.split("/").some(p => !p || p === "." || p === "..")) throw new Error("Invalid course source path");
      sources.writeFile("/" + path, content);
    }
    sources.writeFile("/.base58-setup.sh", manifest.setup);
    const pythonRunner = await fetch("/static/guest/course-python.py");
    if (!pythonRunner.ok) throw new Error("Unable to load course Python runner");
    sources.writeFile("/.base58-python.py", await pythonRunner.text());
    let next = 0, done = 0;
    await Promise.all([0,1].map(async () => {
      while (next < manifest.archives.length) {
        const archive = manifest.archives[next++];
        mountArchive(store, archive.storePath, await verifiedArchive(archive, assetBaseURL));
        this.onStatus("Loading course tools (" + (++done) + "/" + manifest.archives.length + ")…");
      }
    }));
    this.mounts = {"/course-store":{fs:store,readOnly:true}, "/course":{fs:sources,readOnly:true}};
    this.shell = manifest.shell;
    // The device lease has expired or was released before we can reach here.
    // Never rerun a command whose browser disappeared halfway through execution.
    if (this.journal.state.pending) {
      const id = this.journal.state.pending;
      let checkpoint = await this.storage.checkpoint(id);
      if (!checkpoint) {
        checkpoint = await this.storage.get("checkpoints",this.storage.key(id));
        if (checkpoint) await this.storage.upload(id,checkpoint);
      }
      if (checkpoint?.state.result) await this.journal.append(newEvent("result",{commandId:id,result:checkpoint.state.result}));
      else await this.journal.append(newEvent("interrupted",{commandId:id}));
    }
    this.saved = await this.storage.checkpoint(this.journal.state.head);
    if (!this.saved) {
      if (this.journal.state.head) throw new Error("The saved VM checkpoint is missing. Your command history has been kept.");
      await this.startKernel();
      this.saved = await this.capture("");
    }
    this.failed = false;
    this.onStatus(journalURL ? "Course restored · saved to your account" : "Course restored · saved in this browser");
  }

  async startKernel(checkpoint) {
    this.onStatus(checkpoint ? "Resuming course VM…" : "Starting course services…");
    const restore = checkpoint ? await this.storage.restore(checkpoint,this.diskName,SDK) : undefined;
    // This pinned SDK restores host filesystem mounts only in inline mode.
    // No external network device is attached; guest loopback remains available.
    this.vm = await this.sdk.createKernel({
      kernel:new URL("unkernel.wasm",new URL(SDK,location.href)).href,
      memoryMiB:512, worker:false, restore, mounts:this.mounts,
      disks:[{kind:"data",template:"bundled",name:this.diskName}],
    });
    if (restore) return;
    let result = await this.vm.exec("/bin/sh",["-c","ln -s /course-store/nix /nix; mkdir -p /data/workspace; if [ ! -e /data/workspace/.base58-initialized ]; then cp -R /course/. /data/workspace/ && touch /data/workspace/.base58-initialized; fi"]);
    if (result.exitCode !== 0) throw new Error(result.stderr || "Unable to initialize course workspace");
    result = await this.vm.exec(this.shell,["--noprofile","--norc","-c",'. /course/.base58-setup.sh; eval "${shellHook:-}" || exit $?; export -p > /tmp/base58-environment.sh; declare -f >> /tmp/base58-environment.sh'],{cwd:"/data/workspace",signal:AbortSignal.timeout(60000),maxBuffer:MAX_OUTPUT});
    if (result.exitCode !== 0) throw new Error(result.stderr || result.stdout || "Course initialization failed");
  }

  async capture(id, result) {
    this.onStatus("Saving course checkpoint…");
    let snapshot;
    for (let attempt=0;attempt<10;attempt++) {
      try { snapshot = await this.vm.snapshot({stop:true}); break; }
      catch (error) {
        if (attempt === 9 || !/in flight|did not park|EBUSY/.test(error.message)) throw error;
        await new Promise(resolve => setTimeout(resolve,100));
      }
    }
    await this.vm.stop(); this.vm = null;
    return this.storage.save(id,snapshot,this.diskName,SDK,result);
  }

  execute(command, {timeoutMs = 30000, source = "", lessonPath = "", blockId = ""} = {}) {
    return this.enqueue(async () => {
      await this.ready();
      if (this.failed) throw new Error("The last command could not finish saving. Save and stop, then reopen the environment to recover.");
      if (typeof command.program !== "string" || !command.program || !Array.isArray(command.args) || command.args.some(arg => typeof arg !== "string")) throw new Error("Invalid course command");
      await this.storage.claim();
      await this.startKernel(this.saved);
      const event = newEvent("command",{parent:this.journal.state.head,command:{...command,source,lessonPath,blockId}});
      try {
        // Commit resolved argv BEFORE dispatch, including failed commands.
        await this.journal.append(event);
        const signal = AbortSignal.timeout(timeoutMs);
        let result;
        try {
          result = await this.vm.exec(this.shell,["--noprofile","--norc","-c",'. /tmp/base58-environment.sh; exec "$@"',"course-command",command.program,...command.args],{cwd:"/data/workspace",signal,maxBuffer:MAX_OUTPUT});
          result = {stdout:boundedOutput(result.stdout),stderr:boundedOutput(result.stderr),exitCode:result.exitCode ?? -1,timedOut:false};
        } catch (error) {
          // A timeout may have changed files. Save that actual state as well.
          result = {stdout:"",stderr:signal.aborted ? "Command timed out.\n" : boundedOutput(error.message || error),exitCode:-1,timedOut:signal.aborted};
        }
        this.saved = await this.capture(event.id,result);
        await this.journal.append(newEvent("result",{commandId:event.id,result}));
        this.onStatus(this.config.journalURL ? "Command saved to your account" : "Command saved in this browser");
        return result;
      } catch (error) {
        this.failed = true;
        this.onStatus("Saving did not finish · your previous checkpoint is safe");
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
      await this.ready();
      if (this.failed || this.journal.state.pending) throw new Error("Finish saving or reopen the environment before undoing");
      const entry = this.journal.state.commands.get(this.journal.state.head);
      if (!entry) return;
      const head = entry.parent || "", checkpoint = await this.storage.checkpoint(head);
      if (!checkpoint) throw new Error("The previous checkpoint is unavailable; command history was kept");
      await this.storage.claim();
      // Validate and restore the target before changing the durable cursor.
      await this.startKernel(checkpoint);
      await this.vm.stop(); this.vm = null;
      await this.journal.append(newEvent("checkout",{head}));
      this.saved = checkpoint;
      this.onStatus("Last command undone · earlier commands and wallet state preserved");
      return entry;
    });
  }

  saveAndStop() {
    return this.enqueue(async () => {
      const incomplete = this.failed;
      await this.close();
      this.onStatus(incomplete ? "Stopped · reopen to recover the last saved command" : this.config?.journalURL ? "Course work saved · open on any device to continue" : "Course work saved in this browser");
    });
  }

  async close() {
    try { if (this.vm) await this.vm.stop(); }
    finally {
      this.vm = null;
      try { await this.storage?.release(); }
      finally { this.release?.(); this.release = null; this.booting = null; }
    }
  }
}
