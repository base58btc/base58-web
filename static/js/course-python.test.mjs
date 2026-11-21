import test from "node:test";
import assert from "node:assert/strict";
import {PythonSession, pythonCellSource, pythonCellRequest} from "./course-python.mjs";
import {CourseVM} from "./course-vm.mjs";
import {CommandJournal} from "./course-history.mjs";

function pipeProcess(handle = () => ({ok:true, output:"2\n"}), ready = true) {
  let stdout, stderr, end;
  const requests = [];
  const encoder = new TextEncoder();
  const process = {
    stdout:new ReadableStream({start: controller => { stdout = controller; }}),
    stderr:new ReadableStream({start: controller => { stderr = controller; }}),
    stdin:new WritableStream({write: bytes => {
      const request = JSON.parse(new TextDecoder().decode(bytes));
      requests.push(request);
      const response = handle(request);
      if (response) send({id:request.id, ...response});
    }}),
    ended:new Promise(resolve => { end = resolve; }),
    kill:() => {},
    close:async () => { if (!process.closed) { process.closed = true; stdout.close(); stderr.close(); end({type:"closed"}); } }
  };
  const send = frame => stdout.enqueue(encoder.encode(JSON.stringify(frame) + "\n"));
  if (ready) send({ready:1});
  return {process, requests, send, bytes: bytes => stdout.enqueue(bytes)};
}
const signal = () => new AbortController().signal;

test("one Python process receives multiple cells, including errors, over the same pipe", async () => {
  const fixture = pipeProcess(request => ({ok:request.code !== "fail", output:request.code + "⚡\n"}));
  const python = new PythonSession(fixture.process);
  assert.equal((await python.execute("value = 7", signal())).exitCode, 0);
  assert.equal((await python.execute("fail", signal())).exitCode, 1);
  assert.equal((await python.execute("value", signal())).stdout, "value⚡\n");
  assert.deepEqual(fixture.requests.map(r=>r.id), [1,2,3]);
  await python.close();
});

test("response framing handles UTF-8 split across chunks and escaped newlines", async () => {
  const fixture = pipeProcess(() => null), python = new PythonSession(fixture.process);
  const pending = python.execute("value", signal());
  await new Promise(setImmediate);
  const bytes = new TextEncoder().encode(JSON.stringify({id:1,ok:true,output:"⚡\n\0"})+"\n");
  for (const byte of bytes) fixture.bytes(Uint8Array.of(byte));
  assert.equal((await pending).stdout, "⚡\n\0");
  await python.close();
});

test("lost pipes and malformed responses fail permanently rather than losing variables silently", async () => {
  for (const fault of [f=>f.process.close(), f=>f.send({id:100,ok:true,output:"wrong cell"})]) {
    const fixture = pipeProcess(() => null), python = new PythonSession(fixture.process);
    const pending = python.execute("value = 7", signal());
    const failed = assert.rejects(pending, /Python session is unavailable/);
    await new Promise(setImmediate);
    await fault(fixture);
    await failed;
    await assert.rejects(python.execute("value", signal()), /Python session is unavailable/);
    assert.equal(fixture.requests.length, 1);
    await python.close();
  }
});

test("timeouts abort a pending cell and poison its Python session", async () => {
  const fixture = pipeProcess(() => null), python = new PythonSession(fixture.process), controller = new AbortController();
  const pending = python.execute("while True: pass", controller.signal);
  const failed = assert.rejects(pending, /Python session is unavailable/);
  await new Promise(setImmediate);
  controller.abort();
  await failed;
  await assert.rejects(python.execute("1+1", signal()), /Python session is unavailable/);
  await python.close();
});

test("legacy Python journal entries decode their argv; other CLI commands are unaffected", () => {
  const command = {program:"python3",args:["/course/.base58-python.py","--execute",Buffer.from("'⚡'").toString("base64")],source:"wrong metadata"};
  assert.equal(pythonCellSource(command), "'⚡'");
  assert.equal(pythonCellSource({program:"python3",args:["-c","pass"]}), null);
  assert.throws(()=>pythonCellSource({...command,args:[...command.args.slice(0,2),"!"]}));
});

test("challenge argv retains both edited source and checks for replay", () => {
  const request = {code:"answer = '⚡'", check:"assert answer == '⚡'"};
  const command = {program:"python3",args:["/course/.base58-python.py","--check",Buffer.from(JSON.stringify(request)).toString("base64")],source:"wrong metadata"};
  assert.deepEqual(pythonCellRequest(command), request);
  assert.equal(pythonCellSource(command), null);
  assert.throws(()=>pythonCellRequest({...command,args:[...command.args.slice(0,2),Buffer.from('{"code":"pass"}').toString("base64")]}),/Invalid Python challenge/);
  assert.throws(()=>new CourseVM({}).checkPython("x".repeat(16384),"assert True"),/16 KiB/);
});

test("challenge results, later cells, replay and undo use one logged Python session", async () => {
  const events=[], processes=[], runtime=new CourseVM({});
  const storage={claim:async()=>{},readJournal:async()=>events.slice(),appendEvent:async event=>{events.push(event);return events.slice();}};
  runtime.storage=storage; runtime.config={}; runtime.journal=new CommandJournal(storage);
  runtime.ready=async()=>{};
  runtime.startKernel=async()=>{
    let value=0;
    runtime.vm={stop:async()=>{},spawn:async()=>{
      const fixture=pipeProcess(request=>{
        if(request.code!=="read") value=Number(request.code);
        return {ok:!request.check || value===42, output:String(value), ...(request.check ? {phase:"check"} : {})};
      });
      processes.push(fixture);return fixture.process;
    },exec:async()=>{throw new Error("Unexpected Python subprocess");}};
  };
  await runtime.journal.load();await runtime.rebuild("");
  assert.equal((await runtime.checkPython("41","assert answer() == 42",{blockId:"answer"})).exitCode,1);
  const passed=await runtime.checkPython("42","assert answer() == 42",{blockId:"answer"});
  assert.equal(passed.exitCode,0);assert.equal(passed.phase,"check");
  assert.equal((await runtime.executePython("read")).stdout,"42");
  assert.equal(processes.length,1);
  assert.equal(events[0].command.source,"41");
  assert.equal(events[1].result.exitCode,1);
  assert.equal(events[1].result.phase,undefined);
  const recorded=JSON.stringify(events);
  await runtime.rebuild(runtime.journal.state.head);
  assert.equal(JSON.stringify(events),recorded);
  assert.equal(processes[1].requests[0].check,"assert answer() == 42");
  await runtime.undo();await runtime.undo();
  assert.equal((await runtime.executePython("read")).stdout,"41");
  await runtime.stopKernel();
});

test("logging, resume and undo reuse Python within each VM and start fresh for replay", async () => {
  const events = [], processes = [], trace = [];
  const storage = {claim:async()=>{},readJournal:async()=>events.slice(),appendEvent:async event=>{trace.push(event.type);events.push(event);return events.slice();}};
  const runtime = new CourseVM({});
  runtime.storage = storage; runtime.config = {}; runtime.journal = new CommandJournal(storage);
  runtime.ready = async()=>{};
  runtime.startKernel = async()=>{
    let value = 0;
    runtime.vm = {stop:async()=>{},spawn:async()=>{
      trace.push("spawn");
      const fixture = pipeProcess(request=>{
        trace.push("cell");
        if(request.code === "increment") value++;
        return {ok:true,output:String(value)};
      });
      processes.push(fixture); return fixture.process;
    },exec:async()=>{throw new Error("Python cells must not use vm.exec");}};
  };
  await runtime.journal.load(); await runtime.rebuild("");
  const results = await Promise.all([runtime.executePython("increment"),runtime.executePython("increment")]);
  assert.deepEqual(results.map(r=>r.stdout), ["1","2"]);
  assert.equal(processes.length,1);
  assert.deepEqual(trace,["command","spawn","cell","result","command","cell","result"]);
  const saved = JSON.stringify(events);
  await runtime.rebuild(runtime.journal.state.head);
  assert.equal(processes.length,2); assert.equal(processes[0].process.closed,true);
  assert.equal(processes[1].requests.length,2); assert.equal(JSON.stringify(events),saved);
  await runtime.undo();
  assert.equal(processes.length,3); assert.equal(processes[2].requests.length,1);
  assert.equal((await runtime.executePython("increment")).stdout,"2");
  assert.equal(processes.length,3);
  await runtime.stopKernel();
});

test("interpreter death is logged and stops the VM before another cell can execute", async () => {
  const events=[], fixture=pipeProcess(()=>null), runtime=new CourseVM({});
  const storage={claim:async()=>{},readJournal:async()=>events.slice(),appendEvent:async event=>{events.push(event);return events.slice();}};
  runtime.storage=storage; runtime.config={}; runtime.journal=new CommandJournal(storage);
  runtime.ready=async()=>{}; runtime.vm={spawn:async()=>fixture.process,stop:async()=>{}};
  await runtime.journal.load();
  const pending=runtime.executePython("exit");
  await new Promise(setImmediate); await fixture.process.close();
  assert.equal((await pending).exitCode,-1);
  assert.equal(events.length,2); assert.equal(runtime.vm,null); assert.ok(runtime.replayFailure);
  await assert.rejects(runtime.executePython("1+1"),/could not finish saving/);
  assert.equal(events.length,2);
});

function preparationFixture({beforeBoot = async () => {}, pythonReady = true} = {}) {
  const events = [], processes = [];
  let boots = 0, stops = 0, releases = 0;
  const storage = {
    claim:async () => {}, release:async () => { releases++; },
    readJournal:async () => events.slice(),
    appendEvent:async event => { events.push(event); return events.slice(); }
  };
  const runtime = new CourseVM({});
  runtime.boot = async signal => {
    boots++;
    await beforeBoot(signal);
    signal.throwIfAborted();
    runtime.storage = storage; runtime.config = {};
    runtime.journal = new CommandJournal(storage);
    await runtime.journal.load();
    runtime.vm = {
      stop:async () => { stops++; },
      spawn:async () => {
        const fixture = pipeProcess(undefined, pythonReady);
        processes.push(fixture); return fixture.process;
      }
    };
  };
  return {runtime, events, processes, counts:() => ({boots, stops, releases})};
}

test("preparing Python waits for its handshake without executing or logging a cell", async () => {
  const {runtime, events, processes, counts} = preparationFixture({pythonReady:false});
  let finished = false;
  const preparing = runtime.prepare({python:true}).then(() => { finished = true; });
  await new Promise(setImmediate);
  assert.equal(finished, false);
  assert.equal(processes.length, 1);
  assert.deepEqual(processes[0].requests, []);
  processes[0].send({ready:1});
  await preparing;
  assert.deepEqual(events, []);
  await runtime.prepare({python:true});
  assert.equal((await runtime.executePython("1+1")).stdout, "2\n");
  assert.equal(processes.length, 1);
  assert.equal(counts().boots, 1);
  assert.deepEqual(events.map(event => event.type), ["command", "result"]);
  await runtime.close();
});

test("Play during preparation waits for the same boot and interpreter", async () => {
  let boot;
  const {runtime, events, processes, counts} = preparationFixture({beforeBoot:() => new Promise(resolve => { boot = resolve; })});
  const preparing = runtime.prepare({python:true});
  const playing = runtime.executePython("1+1");
  await new Promise(setImmediate);
  assert.equal(counts().boots, 1);
  assert.deepEqual(events, []);
  boot();
  await preparing;
  assert.equal((await playing).stdout, "2\n");
  assert.equal(processes.length, 1);
  assert.equal(processes[0].requests.length, 1);
  await runtime.close();
});

test("stopping during Python warmup closes the process, preserves history, and allows resume", async () => {
  const {runtime, events, processes, counts} = preparationFixture({pythonReady:false});
  const preparing = runtime.prepare({python:true});
  const rejected = assert.rejects(preparing, /Python session is unavailable/);
  await new Promise(setImmediate);
  await runtime.saveAndStop(); await rejected;
  assert.equal(processes[0].process.closed, true);
  assert.equal(runtime.vm, null);
  assert.equal(runtime.replayFailure, null);
  assert.equal(counts().stops, 1);
  assert.deepEqual(events, []);
  const resumed = runtime.prepare({python:true});
  await new Promise(setImmediate);
  processes[1].send({ready:1});
  await resumed;
  assert.equal((await runtime.executePython("1+1")).exitCode, 0);
  await runtime.close();
});

test("closing during boot cancels preparation before it can create a late VM", async () => {
  const {runtime, events, counts} = preparationFixture({beforeBoot:signal => new Promise((resolve, reject) => {
    signal.addEventListener("abort", () => reject(signal.reason), {once:true});
  })});
  const preparing = runtime.prepare({python:true});
  const rejected = assert.rejects(preparing, {name:"AbortError"});
  await new Promise(setImmediate);
  await runtime.close(); await rejected;
  assert.equal(runtime.booting, null);
  assert.equal(runtime.vm, null);
  assert.equal(counts().stops, 0);
  assert.deepEqual(events, []);
});
