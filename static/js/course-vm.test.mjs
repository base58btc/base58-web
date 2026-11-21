import test from "node:test";
import assert from "node:assert/strict";
import {gzipSync} from "node:zlib";
import {createHash} from "node:crypto";
import {parseNar, mountArchive} from "./course-nar.mjs";
import {CourseVM, boundedOutput, commandQueue, verifiedArchive, loadArchives} from "./course-vm.mjs";
import {CommandJournal, journalState, replayCommands} from "./course-history.mjs";

function nar(...tokens) {
  return Buffer.concat(tokens.map(token => {
    const bytes = Buffer.from(token);
    const size = Buffer.alloc(8); size.writeBigUInt64LE(BigInt(bytes.length));
    return Buffer.concat([size, bytes, Buffer.alloc((8 - bytes.length % 8) % 8)]);
  }));
}
const file = () => nar("nix-archive-1", "(", "type", "regular", "executable", "", "contents", "hello\n", ")");

test("reads executable NAR files without changing their bytes", () => {
  const entries = parseNar(file());
  assert.equal(entries[0].executable, true);
  assert.equal(Buffer.from(entries[0].data).toString(), "hello\n");
});

test("rejects truncated archives, traversal, and trailing data", () => {
  assert.throws(() => parseNar(file().subarray(0, 25)), /Truncated|length/);
  assert.throws(() => parseNar(Buffer.concat([file(), Buffer.from("x")])), /Unexpected/);
  assert.throws(() => parseNar(nar("nix-archive-1", "(", "type", "directory", "entry", "(", "name", "..")), /path/);
  assert.throws(() => mountArchive({}, "/tmp/not-a-store-path", file()), /store path/);
});

test("verifies downloaded archives before decompression", async () => {
  const bytes = gzipSync(file());
  const archive = {path: "runtime/test.nar.gz", size: bytes.length, sha256: createHash("sha256").update(bytes).digest("hex")};
  const fetcher = async () => new Response(bytes);
  assert.deepEqual(Buffer.from(await verifiedArchive(archive, "https://example.test/", fetcher)), file());
  await assert.rejects(verifiedArchive({...archive, sha256: "0".repeat(64)}, "https://example.test/", fetcher), /integrity/);
});

test("commands stay ordered and a failed cell does not block the next", async () => {
  const enqueue = commandQueue();
  const events = [];
  const first = enqueue(async () => { events.push("first"); throw new Error("cell failed"); });
  const second = enqueue(async () => { events.push("second"); return 7; });
  await assert.rejects(first, /cell failed/);
  assert.equal(await second, 7);
  assert.deepEqual(events, ["first", "second"]);
});

test("archive mounting stays reproducible despite download order", async () => {
  const archives = ["a", "b"].map(name => ({storePath: "/nix/store/" + name.repeat(32) + "-example"}));
  const load = async reverse => {
    const inodes = new Map(), pending = new Map();
    const files = {writeFile: path => inodes.set(path, inodes.size + 1)};
    const loading = loadArchives(files, archives, "https://example.test/", () => {}, archive =>
      new Promise(resolve => pending.set(archive.storePath, resolve)));
    // Force opposite network/cache completion orders without timing assumptions.
    for (const archive of reverse ? [...archives].reverse() : archives) pending.get(archive.storePath)(file());
    await loading;
    return inodes;
  };
  assert.deepEqual(await load(false), await load(true));
});

function memoryJournal(trace, refuse = false) {
  const events=[];
  return {
    events,
    claim:async () => {},
    release:async () => {},
    readJournal:async () => events.slice(),
    appendEvent:async event => {
      trace.push(event.type);
      if (refuse) throw new Error("offline");
      events.push(event); return events.slice();
    }
  };
}

test("saved output stays within the server byte limit for multibyte text", () => {
  const value=boundedOutput("🚀".repeat(300000));
  assert.ok(Buffer.byteLength(value)<=1<<20);
  assert.match(value,/output truncated/);
});

test("native Python keeps original Unicode source and encodes argv without shell evaluation", async () => {
  const runtime = new CourseVM({});
  let captured;
  runtime.execute = async (command, metadata) => { captured = {command, metadata}; return {exitCode:0}; };
  const source = 'value = "⚡ $(echo unsafe)"\nvalue';
  await runtime.executePython(source, {lessonPath:"02-invoices", blockId:"invoice"});
  assert.equal(captured.command.program, "python3");
  assert.equal(captured.command.args[0], "/course/.base58-python.py");
  assert.equal(Buffer.from(captured.command.args[2], "base64").toString(), source);
  assert.equal(captured.metadata.source, source);
  assert.equal(captured.metadata.blockId, "invoice");
  assert.throws(() => runtime.executePython("⚡".repeat(6000)), /16 KiB/);
});

test("records resolved argv before dispatch and keeps failed commands in the log without stopping the VM", async () => {
  const trace=[], storage=memoryJournal(trace);
  const runtime=new CourseVM({});
  runtime.storage=storage; runtime.config={}; runtime.journal=new CommandJournal(storage);
  await runtime.journal.load();
  runtime.ready=async () => {};
  runtime.vm={exec:async () => {trace.push("execute");return {exitCode:1,stdout:"partial write",stderr:"error"};}};
  const result=await runtime.execute({program:"bash",args:["-c","touch partial; exit 1"]},{source:"!bash ...",lessonPath:"01_intro",blockId:"cell-1"});
  assert.deepEqual(trace,["command","execute","result"]);
  assert.equal(result.exitCode,1);
  assert.equal(runtime.journal.state.commands.get(runtime.journal.state.head).command.args[1],"touch partial; exit 1");
});

test("a journal write failure prevents command dispatch", async () => {
  const trace=[],storage=memoryJournal(trace,true),runtime=new CourseVM({});
  runtime.storage=storage;runtime.config={};runtime.journal=new CommandJournal(storage);await runtime.journal.load();
  runtime.ready=async () => {};
  runtime.vm={exec:async () => {trace.push("execute");},stop:async () => {}};
  await assert.rejects(runtime.execute({program:"touch",args:["file"]}),/offline/);
  assert.deepEqual(trace,["command"]);
});

test("undo preserves original results, branches, and interrupted commands", () => {
  const events=[
    {id:"a",type:"command",command:{program:"createwallet",args:[]}},
    {id:"ar",type:"result",commandId:"a",result:{exitCode:0}},
    {id:"b",type:"command",parent:"a",command:{program:"mine",args:[]}},
    {id:"br",type:"result",commandId:"b",result:{exitCode:0}},
    {id:"u",type:"checkout",head:"a"},
    {id:"c",type:"command",parent:"a",command:{program:"sleep",args:[]}},
    {id:"i",type:"interrupted",commandId:"c"}
  ];
  const state=journalState(events);
  assert.equal(state.head,"a");assert.equal(state.pending,"");assert.equal(state.commands.size,3);
  assert.equal(state.commands.get("b").result.exitCode,0);assert.equal(state.commands.get("c").interrupted,true);
  assert.throws(()=>journalState([...events,{id:"bad",type:"checkout",head:"b"}]),/checkout/);
});

function replayFixture() {
  const trace=[], storage=memoryJournal(trace), runtime=new CourseVM({});
  runtime.storage=storage; runtime.config={}; runtime.journal=new CommandJournal(storage);
  let counter=0, boots=0;
  runtime.startKernel=async () => {
    boots++; counter=0;
    runtime.vm={
      stop:async () => {},
      exec:async (_shell,args) => {
        const program=args[args.indexOf("course-command")+1];
        if(program==="increment") counter++;
        return {exitCode:0,stdout:String(counter),stderr:""};
      }
    };
  };
  runtime.ready=async () => {};
  return {runtime, storage, boots:()=>boots};
}

test("ordinary cells reuse one VM; resume replays without adding journal events", async () => {
  const {runtime,storage,boots}=replayFixture();
  await runtime.journal.load(); await runtime.rebuild("");
  assert.equal((await runtime.execute({program:"increment",args:[]})).stdout,"1");
  assert.equal((await runtime.execute({program:"increment",args:[]})).stdout,"2");
  assert.equal(boots(),1);
  const before=JSON.stringify(storage.events);
  await runtime.rebuild(runtime.journal.state.head);
  assert.equal(boots(),2); assert.equal(JSON.stringify(storage.events),before);
  assert.equal((await runtime.execute({program:"increment",args:[]})).stdout,"3");
});

test("undo rebuilds a fresh VM and new commands exclude the undone branch", async () => {
  const {runtime,storage}=replayFixture();
  await runtime.journal.load(); await runtime.rebuild("");
  await runtime.execute({program:"increment",args:[]});
  await runtime.execute({program:"increment",args:[]});
  const undone=await runtime.undo();
  assert.equal((await runtime.execute({program:"increment",args:[]})).stdout,"2");
  assert.equal(runtime.journal.state.commands.get(undone.id).result.stdout,"2");
  assert.deepEqual(replayCommands(runtime.journal.state).map(e=>e.result.stdout),["1","2"]);
  const before=storage.events.length;
  await runtime.rebuild(runtime.journal.state.head);
  assert.equal(storage.events.length,before);
});

test("replay accepts changed output and recorded nonzero exits but stops on a changed outcome", async () => {
  const {runtime,storage}=replayFixture();
  await runtime.journal.load(); await runtime.rebuild("");
  await runtime.execute({program:"increment",args:[]});
  await runtime.execute({program:"increment",args:[]});
  const before=JSON.stringify(storage.events);
  runtime.runCommand=async () => ({exitCode:1,stdout:"different",stderr:"missing input",timedOut:false});
  await assert.rejects(runtime.rebuild(runtime.journal.state.head),/Replay stopped at command 1/);
  assert.equal(runtime.vm,null); assert.equal(runtime.failed,true);
  assert.equal(JSON.stringify(storage.events),before);
  // Even when an earlier command fails, repeated Undo must be able to reach an
  // empty history. Failed rebuilds must not trap the student behind the last cell.
  await assert.rejects(runtime.undo(),/Replay stopped/);
  await runtime.undo();
  assert.equal(runtime.journal.state.head,""); assert.equal(runtime.failed,false);
  await runtime.execute({program:"expected-failure",args:[]});
  runtime.runCommand=async () => ({exitCode:1,stdout:"new output",stderr:"",timedOut:false});
  await runtime.rebuild(runtime.journal.state.head);
  assert.equal(runtime.failed,false);
});

test("an interrupted command is retained in history but never replayed", () => {
  const state=journalState([
    {id:"a",type:"command",command:{program:"increment",args:[]}},
    {id:"ar",type:"result",commandId:"a",result:{exitCode:0}},
    {id:"b",type:"command",parent:"a",command:{program:"increment",args:[]}},
    {id:"bi",type:"interrupted",commandId:"b"}
  ]);
  assert.deepEqual(replayCommands(state).map(entry=>entry.id),["a"]);
});

test("a failed result write stops the VM and never reexecutes the command", async () => {
  const {runtime,storage}=replayFixture();
  await runtime.journal.load(); await runtime.rebuild("");
  const append=storage.appendEvent;
  storage.appendEvent=async event => { if(event.type==="result")throw new Error("offline"); return append(event); };
  await assert.rejects(runtime.execute({program:"increment",args:[]}),/offline/);
  assert.equal(runtime.vm,null); assert.equal(runtime.failed,true);
  assert.equal(storage.events.length,1);
  await assert.rejects(runtime.execute({program:"increment",args:[]}),/could not finish saving/);
  assert.equal(storage.events.length,1);
});


test("a timed-out execution is logged, stops the VM, and can be undone", async () => {
  const {runtime}=replayFixture();
  await runtime.journal.load(); await runtime.rebuild("");
  runtime.runCommand=async () => ({stdout:"",stderr:"timeout",exitCode:-1,timedOut:true});
  const result=await runtime.execute({program:"sleep",args:["100"]});
  assert.equal(result.timedOut,true); assert.equal(runtime.vm,null);
  assert.ok(runtime.replayFailure);
  assert.equal(runtime.journal.state.commands.get(runtime.journal.state.head).result.timedOut,true);
  await runtime.undo();
  assert.equal(runtime.failed,false); assert.equal(runtime.journal.state.head,"");
});

function resetFixture() {
  const fixture = replayFixture(), {runtime, storage} = fixture;
  runtime.ready = CourseVM.prototype.ready;
  runtime.loadHistory = async () => {
    runtime.journal = new CommandJournal(storage);
    await runtime.journal.load();
  };
  runtime.boot = async signal => {
    await runtime.openHistory(signal);
    await runtime.rebuild(runtime.journal.state.head, signal);
  };
  return fixture;
}

test("reset archives the log, boots empty, and excludes archived commands on resume and undo", async () => {
  const {runtime, storage} = resetFixture();
  await runtime.execute({program:"increment",args:[]});
  await runtime.execute({program:"increment",args:[]});
  const original = storage.events.slice(), oldHead = runtime.journal.state.head;
  await runtime.reset();
  assert.deepEqual(storage.events.slice(0, original.length), original);
  assert.equal(storage.events.at(-1).type, "reset");
  assert.equal(runtime.journal.state.head, "");
  assert.deepEqual(replayCommands(runtime.journal.state), []);
  assert.equal(await runtime.undo(), undefined);
  assert.throws(() => journalState([...storage.events, {type:"checkout",head:oldHead}]), /checkout/);
  assert.equal((await runtime.execute({program:"increment",args:[]})).stdout, "1");
  assert.equal(storage.events.at(-2).parent, "");
  await runtime.saveAndStop();
  assert.equal((await runtime.execute({program:"increment",args:[]})).stdout, "2");
  assert.equal(replayCommands(runtime.journal.state).length, 2);
  await runtime.undo(); await runtime.undo(); await runtime.undo();
  assert.equal(runtime.journal.state.head, "");
  assert.equal(runtime.journal.state.commands.get(oldHead).result.stdout, "2");
  await runtime.close();
});

test("reset from a stopped or failed workspace never executes the old commands", async () => {
  for (const failed of [false, true]) {
    const {runtime, storage} = resetFixture();
    await runtime.execute({program:"increment",args:[]});
    if (failed) runtime.replayFailure = new Error("divergent replay");
    else await runtime.saveAndStop();
    runtime.runCommand = async () => { throw new Error("old history was replayed"); };
    await runtime.reset();
    assert.equal(storage.events.length, 3);
    assert.equal(runtime.journal.state.head, "");
    assert.equal(runtime.failed, false);
    assert.ok(runtime.vm);
    await runtime.close();
  }
});

test("reset recovers an unfinished command and preserves its interrupted audit entry", async () => {
  const {runtime, storage} = resetFixture();
  storage.events.push({id:"unfinished",type:"command",command:{program:"increment",args:[]}});
  assert.throws(() => journalState([...storage.events, {type:"reset",id:"too-early"}]), /reset/);
  await runtime.reset();
  assert.deepEqual(storage.events.map(e=>e.type), ["command","interrupted","reset"]);
  assert.equal(runtime.journal.state.commands.get("unfinished").interrupted, true);
  assert.equal(runtime.journal.state.head, "");
  await runtime.close();
});

test("a failed reset write retains the old replay cursor and never boots a fresh VM", async () => {
  const {runtime, storage, boots} = resetFixture();
  await runtime.execute({program:"increment",args:[]});
  const before = JSON.stringify(storage.events), originalBoots = boots();
  storage.appendEvent = async () => { throw new Error("offline"); };
  await assert.rejects(runtime.reset(), /offline/);
  assert.equal(JSON.stringify(storage.events), before);
  assert.equal(boots(), originalBoots);
  assert.equal(runtime.vm, null);
});

test("a reset stays saved if the new VM fails to start", async () => {
  const {runtime, storage} = resetFixture();
  await runtime.execute({program:"increment",args:[]});
  runtime.startKernel = async () => { throw new Error("startup failed"); };
  await assert.rejects(runtime.reset(), /startup failed/);
  const restored = journalState(storage.events);
  assert.equal(restored.head, "");
  assert.equal(restored.commands.size, 1);
  assert.equal(storage.events.at(-1).type, "reset");
});

test("reset interrupts background preparation without waiting for old commands to replay", async () => {
  const {runtime, storage} = resetFixture();
  await runtime.execute({program:"increment",args:[]});
  await runtime.saveAndStop();
  let started;
  const replayStarted = new Promise(resolve => { started = resolve; });
  runtime.runCommand = async (_command, _timeout, signal) => {
    started();
    await new Promise(resolve => signal.addEventListener("abort", resolve, {once:true}));
    signal.throwIfAborted();
  };
  const preparing = runtime.prepare();
  const cancelled = assert.rejects(preparing, {name:"AbortError"});
  await replayStarted;
  await runtime.reset(); await cancelled;
  assert.equal(storage.events.at(-1).type, "reset");
  assert.equal(runtime.journal.state.head, "");
  assert.ok(runtime.vm);
  await runtime.close();
});

test("reset waits for a running student command to finish recording", async () => {
  const {runtime, storage} = resetFixture();
  let started, finish;
  const running = new Promise(resolve => { started = resolve; });
  runtime.runCommand = async () => {
    started(); await new Promise(resolve => { finish = resolve; });
    return {exitCode:0,stdout:"kept",stderr:""};
  };
  const command = runtime.execute({program:"increment",args:[]});
  await running;
  const reset = runtime.reset();
  assert.equal(storage.events.length, 1);
  finish(); await command; await reset;
  assert.deepEqual(storage.events.map(e=>e.type), ["command","result","reset"]);
  assert.equal(storage.events[1].result.stdout, "kept");
  await runtime.close();
});
