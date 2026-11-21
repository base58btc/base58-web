import test from "node:test";
import assert from "node:assert/strict";
import {gzipSync} from "node:zlib";
import {createHash} from "node:crypto";
import {parseNar, mountArchive} from "./course-nar.mjs";
import {CourseVM, boundedOutput, commandQueue, verifiedArchive} from "./course-vm.mjs";
import {CommandJournal, journalState} from "./course-history.mjs";

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

function memoryJournal(trace, refuse = false) {
  const events=[];
  return {
    events,
    claim:async () => {},
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

test("records resolved argv before dispatch and commits failed commands only after their checkpoint", async () => {
  const trace=[], storage=memoryJournal(trace);
  const runtime=new CourseVM({});
  runtime.storage=storage; runtime.config={}; runtime.journal=new CommandJournal(storage);
  await runtime.journal.load();
  runtime.ready=async () => {};
  runtime.startKernel=async () => {runtime.vm={exec:async () => {trace.push("execute");return {exitCode:1,stdout:"partial write",stderr:"error"};}};};
  runtime.capture=async () => {trace.push("checkpoint");return {};};
  const result=await runtime.execute({program:"bash",args:["-c","touch partial; exit 1"]},{source:"!bash ...",lessonPath:"01_intro",blockId:"cell-1"});
  assert.deepEqual(trace,["command","execute","checkpoint","result"]);
  assert.equal(result.exitCode,1);
  assert.equal(runtime.journal.state.commands.get(runtime.journal.state.head).command.args[1],"touch partial; exit 1");
});

test("a journal write failure prevents command dispatch", async () => {
  const trace=[],storage=memoryJournal(trace,true),runtime=new CourseVM({});
  runtime.storage=storage;runtime.config={};runtime.journal=new CommandJournal(storage);await runtime.journal.load();
  runtime.ready=async () => {};
  runtime.startKernel=async () => {runtime.vm={exec:async () => {trace.push("execute");}};};
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
