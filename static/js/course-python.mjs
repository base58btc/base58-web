const MAX_SOURCE = 16384;
// Python bounds output before JSON encoding, which can expand control characters.
const MAX_FRAME = 8 * (1 << 19);

export function pythonCellSource(command) {
  return command.args[1] === "--execute" ? pythonCellRequest(command)?.code ?? null : null;
}

export function pythonCellRequest(command) {
  if (command.program !== "python3" || command.args.length !== 3 ||
      command.args[0] !== "/course/.base58-python.py" || !["--execute", "--check"].includes(command.args[1])) return null;
  const bytes = Uint8Array.from(atob(command.args[2]), c => c.charCodeAt(0));
  if (bytes.length > MAX_SOURCE) throw new Error("Keep Python cells under 16 KiB");
  // Decode the recorded argv, including old journal entries, rather than trusting
  // optional source metadata that may not describe the actual command.
  const source = new TextDecoder("utf-8", {fatal:true}).decode(bytes);
  if (command.args[1] === "--execute") return {code:source};
  const request = JSON.parse(source);
  if (typeof request?.code !== "string" || typeof request?.check !== "string") throw new Error("Invalid Python challenge request");
  return {code:request.code, check:request.check};
}

function abortable(task, signal) {
  return new Promise((resolve, reject) => {
    const abort = () => reject(signal.reason || new Error("Python cell aborted"));
    if (signal.aborted) { abort(); return; }
    signal.addEventListener("abort", abort, {once:true});
    Promise.resolve().then(task).then(resolve, reject).finally(() => signal.removeEventListener("abort", abort));
  });
}

// One pipe session, interpreter and namespace per VM. Transport failures must
// never silently restart Python: doing so would discard earlier cell variables.
export class PythonSession {
  constructor(process) {
    this.process = process;
    this.writer = process.stdin.getWriter();
    this.sequence = 0;
    this.diagnostics = "";
    this.ready = new Promise((resolve, reject) => { this.resolveReady = resolve; this.rejectReady = reject; });
    this.ready.catch(() => {});
    this.readResponses().catch(error => this.fail(error));
    this.readDiagnostics().catch(error => this.fail(error));
    process.ended.then(() => this.fail(new Error("Python interpreter exited")), error => this.fail(error));
  }

  fail(error) {
    if (this.failure) return;
    this.failure = new Error("Python session is unavailable. Undo the last cell or reopen to replay its history. " +
      (error.message || error) + (this.diagnostics ? "\n" + this.diagnostics : ""));
    this.rejectReady(this.failure);
    this.pending?.reject(this.failure);
    this.pending = null;
    this.process.kill("KILL");
  }

  async readDiagnostics() {
    const reader = this.process.stderr.getReader(), decoder = new TextDecoder();
    for (;;) {
      const {done, value} = await reader.read();
      if (done) return;
      this.diagnostics = (this.diagnostics + decoder.decode(value, {stream:true})).slice(-16384);
    }
  }

  async readResponses() {
    const reader = this.process.stdout.getReader(), decoder = new TextDecoder();
    let buffer = "";
    for (;;) {
      const {done, value} = await reader.read();
      if (done) throw new Error("Python response stream closed");
      buffer += decoder.decode(value, {stream:true});
      if (buffer.length > MAX_FRAME) throw new Error("Python response exceeded its limit");
      for (let end; (end = buffer.indexOf("\n")) !== -1;) {
        const frame = JSON.parse(buffer.slice(0, end));
        buffer = buffer.slice(end + 1);
        if (!this.started) {
          if (frame.ready !== 1) throw new Error("Invalid Python startup response");
          this.started = true; this.resolveReady();
        } else {
          if (!this.pending || frame.id !== this.pending.id || typeof frame.ok !== "boolean" || typeof frame.output !== "string") {
            throw new Error("Invalid Python cell response");
          }
          this.pending.resolve({stdout:frame.output, stderr:"", exitCode:frame.ok ? 0 : 1, timedOut:false,
            ...(frame.phase ? {phase:frame.phase} : {})});
          this.pending = null;
        }
      }
    }
  }

  async execute(source, signal, check) {
    if (this.failure) throw this.failure;
    try {
      return await abortable(async () => {
        await this.ready;
        if (this.failure) throw this.failure;
        if (this.pending) throw new Error("Python cells must execute sequentially");
        const id = ++this.sequence;
        const response = new Promise((resolve, reject) => { this.pending = {id, resolve, reject}; });
        this.writer.write(new TextEncoder().encode(JSON.stringify({id, code:source, ...(check === undefined ? {} : {check})}) + "\n")).catch(error => this.fail(error));
        return response;
      }, signal);
    } catch (error) {
      this.fail(error);
      throw this.failure;
    }
  }

  async waitUntilReady(signal) {
    try {
      await abortable(() => this.ready, signal);
      if (this.failure) throw this.failure;
    } catch (error) {
      this.fail(error);
      throw this.failure;
    }
  }

  async close() {
    this.fail(new Error("Course environment stopped"));
    await this.process.close();
  }
}
