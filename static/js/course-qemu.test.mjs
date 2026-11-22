import test from "node:test";
import assert from "node:assert/strict";
import {readFile} from "node:fs/promises";
import {courseEngine, QemuKernel} from "./course-qemu.mjs";
import {qemuLinkTarget} from "./course-qemu-files.mjs";
import {courseQemuWasm} from "./course-qemu-memory.mjs";

test("chooses QEMU only when JSPI is missing or explicitly requested", () => {
  const wasm = {instantiate() {}};
  assert.equal(courseEngine(wasm), "qemu");
  wasm.Suspending = () => {};
  assert.equal(courseEngine(wasm), "qemu");
  wasm.promising = () => {};
  assert.equal(courseEngine(wasm), "unkernel");
  assert.equal(courseEngine(wasm, "qemu"), "qemu");
  assert.throws(() => courseEngine(null), /WebAssembly enabled/);
});

function fixture() {
  const messages = [], worker = {postMessage:message => messages.push(message), terminate:() => { worker.stopped = true; }};
  return {worker, messages, kernel:new QemuKernel({workerFactory:() => worker})};
}

test("serial requests retain argv and Python challenges without shell interpolation", async () => {
  const {kernel, worker, messages} = fixture();
  const shell = kernel.runCommand({program:"printf", args:["%s", "$(touch nope); ' ⚡"]});
  assert.deepEqual(messages[0].args, ["%s", "$(touch nope); ' ⚡"]);
  worker.onmessage({data:{id:1,result:{stdout:"literal",exitCode:0}}});
  assert.equal((await shell).stdout, "literal");
  const request = {code:"def answer(): return 42", check:"assert answer() == 42"};
  const python = kernel.runCommand({program:"python3",args:["/course/.base58-python.py", "--check", Buffer.from(JSON.stringify(request)).toString("base64")]});
  assert.deepEqual(messages[1], {id:2,type:"python",...request});
  worker.onmessage({data:{id:2,result:{stdout:"",exitCode:0,phase:"check"}}});
  assert.equal((await python).phase, "check");
  await kernel.stop();
});

test("aborting a command terminates its VM and rejects every pending operation", async () => {
  const {kernel, worker, messages} = fixture(), controller = new AbortController();
  const first = kernel.runCommand({program:"sleep",args:["100"]}, controller.signal);
  const second = kernel.runCommand({program:"pwd",args:[]});
  controller.abort();
  await assert.rejects(first, {name:"AbortError"});
  await assert.rejects(second, {name:"AbortError"});
  assert.equal(worker.stopped, true);
  await assert.rejects(kernel.runCommand({program:"pwd",args:[]}), {name:"AbortError"});
  assert.equal(messages.length, 2);
});

test("9p links preserve absolute store paths and expand relative targets without truncation", () => {
  assert.equal(qemuLinkTarget("/share/nix/store/pkg/bin/python3", "python3.13"), "/share/nix/store/pkg/bin/python3.13");
  assert.equal(qemuLinkTarget("/share/nix/store/pkg/lib/libx.so", "../lib64/./libx.so.1"), "/share/nix/store/pkg/lib64/libx.so.1");
  assert.equal(qemuLinkTarget("/share/nix/store/pkg/bin/python", "/nix/store/python/bin/python"), "/nix/store/python/bin/python");
});

test("the pinned QEMU module remains valid with the smaller memory import", async t => {
  let original;
  try { original = new Uint8Array(await readFile(new URL("../vendor/qemu-20260913/qemu-system-x86_64.wasm", import.meta.url))); }
  catch (error) { if (error.code === "ENOENT") { t.skip("run make course-vm-assets to check the pinned binary"); return; } throw error; }
  const smaller = courseQemuWasm(original);
  assert.equal(smaller.length, original.length);
  assert.ok(WebAssembly.validate(smaller));
  assert.notDeepEqual(smaller, original);
  assert.throws(() => courseQemuWasm(smaller), /Unexpected QEMU/);
  assert.throws(() => courseQemuWasm(new Uint8Array(32)), /Unexpected QEMU/);
});
