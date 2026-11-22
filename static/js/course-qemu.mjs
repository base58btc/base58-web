import {pythonCellRequest} from "./course-python.mjs";

export function courseEngine(wasm = globalThis.WebAssembly, requested = "") {
  if (typeof wasm?.instantiate !== "function") throw new Error("This browser cannot run course exercises. Please use a browser with WebAssembly enabled.");
  return requested !== "qemu" && typeof wasm.Suspending === "function" && typeof wasm.promising === "function" ? "unkernel" : "qemu";
}

// A worker owns the engine, filesystem, pthreads, and serial line. Terminating
// it releases the whole guest on stop/undo rather than leaving a worker pool.
export class QemuKernel {
  constructor({onStatus = () => {}, workerFactory = () => new Worker("/static/js/course-qemu-worker.js")} = {}) {
    this.worker = workerFactory();
    this.sequence = 0;
    this.pending = new Map();
    this.worker.onmessage = ({data}) => {
      if (data.status) { onStatus(data.status); return; }
      if (data.error) { this.fail(new Error(data.error)); return; }
      const pending = this.pending.get(data.id);
      if (!pending) return;
      this.pending.delete(data.id);
      pending.resolve(data.result);
    };
    this.worker.onerror = event => this.fail(new Error(event.message || "The compatibility VM stopped unexpectedly."));
  }

  fail(error) {
    if (this.failure) return;
    this.failure = error;
    this.worker.terminate();
    for (const {reject} of this.pending.values()) reject(error);
    this.pending.clear();
  }

  request(message, signal) {
    if (this.failure) return Promise.reject(this.failure);
    signal?.throwIfAborted();
    const id = ++this.sequence;
    return new Promise((resolve, reject) => {
      const abort = () => this.fail(signal.reason || new DOMException("Workspace stopped", "AbortError"));
      const finish = callback => value => { signal?.removeEventListener("abort", abort); callback(value); };
      this.pending.set(id, {resolve:finish(resolve), reject:finish(reject)});
      signal?.addEventListener("abort", abort, {once:true});
      this.worker.postMessage({...message, id});
    });
  }

  async boot(config, signal) {
    if (!globalThis.crossOriginIsolated || typeof SharedArrayBuffer !== "function") {
      this.fail(new Error("The compatibility VM needs an isolated course page. Reload this page to try again."));
      throw this.failure;
    }
    return this.request({type:"boot", config}, signal);
  }

  runCommand(command, signal) {
    const python = pythonCellRequest(command);
    return this.request(python ? {type:"python", ...python} :
      {type:"exec", program:command.program, args:command.args, timeoutMs:command.timeoutMs}, signal);
  }

  async stop() { this.fail(new DOMException("Workspace stopped", "AbortError")); }
}
