import test from "node:test";
import assert from "node:assert/strict";
import {sameCourseSession, isLessonURL, NavigationRequest, installCourseNavigation} from "./course-navigation.mjs";

test("only lesson URLs within the same course and preview mode qualify", () => {
  const current = new URL("https://base58.test/courses/lightning/00-welcome?preview=draft");
  for (const target of ["/courses/lightning/01-csv?preview=draft", "/courses/lightning/part/lesson?preview=draft#exercise"]) {
    assert.equal(isLessonURL(new URL(target,current),current,"lightning"),true);
  }
  for (const target of ["https://other.test/courses/lightning/01?preview=draft", "/courses/other/01?preview=draft", "/courses/lightning-extra/01?preview=draft", "/courses/lightning/01", "/courses/lightning"]) {
    assert.equal(isLessonURL(new URL(target,current),current,"lightning"),false);
  }
});

test("a new attempt, version or authentication page cannot reuse the active VM", () => {
  const current={courseNavigation:"1",courseSlug:"lightning",courseAttempt:"18",courseVersion:"4",draftPreview:"false",draftVersion:"5"};
  assert.equal(sameCourseSession(current,{...current,lessonPath:"02-invoices",csrfToken:"new-token"}),true);
  for (const changed of [{courseAttempt:"19"},{courseVersion:"5"},{draftPreview:"true"},{draftVersion:"6"},{courseSlug:"taproot"},{localPreview:"true"},{courseSource:"new-bundle"},{courseNavigation:""}]) {
    assert.equal(sameCourseSession(current,{...current,...changed}),false);
  }
  assert.equal(sameCourseSession(current,undefined),false);
});

test("subchapter links reuse the course session without bypassing preview-mode changes", () => {
  const current = new URL("https://base58.test/courses/lightning/02-invoices?preview=draft&section=the-human-readable-part");
  for (const path of ["02-invoices?preview=draft&section=bech32-five-bit-words", "02-invoices?preview=draft", "03-revocation-generation?preview=draft&section=last"]) {
    assert.equal(isLessonURL(new URL("/courses/lightning/" + path, current), current, "lightning"), true);
  }
  assert.equal(isLessonURL(new URL("/courses/lightning/02-invoices?section=bech32-five-bit-words", current), current, "lightning"), false);
});

test("rapid navigation invalidates stale responses even when fetch ignores abort", async () => {
  const requests = new NavigationRequest(), first=requests.begin();
  let complete;
  const response=new Promise(resolve=>{complete=resolve;});
  const committed=[];
  const waiting=response.then(()=>{if(first.active())committed.push("old lesson");});
  const second=requests.begin();
  assert.equal(first.signal.aborted,true);
  if(second.active())committed.push("new lesson");
  complete();await waiting;
  assert.deepEqual(committed,["new lesson"]);
});

// Exercise the actual navigation/history handlers with a small browser boundary.
// Scroll events are synchronous here; the browser check also covers real layout.
function navigationBrowser(t, initialURL) {
  const globals = ["window", "document", "history", "location", "scrollX", "scrollY", "fetch", "DOMParser"];
  const previous = new Map(globals.map(key => [key, Object.getOwnPropertyDescriptor(globalThis, key)]));
  t.after(() => { for (const [key, descriptor] of previous) {
    if (descriptor) Object.defineProperty(globalThis, key, descriptor);
    else delete globalThis[key];
  } });
  const listeners = new Map(), notice = {}, entries = [{url:initialURL,state:{}}];
  let entry = 0, resolveIdle;
  const on = (name, callback) => listeners.set(name, callback);
  const session = {courseNavigation:"1",courseSlug:"lightning",courseAttempt:"2",courseVersion:"3"};
  const makeShell = () => ({dataset:{...session},setAttribute() {},querySelector() {return null;},replaceWith(next) {currentShell=next;}});
  let currentShell = makeShell();
  Object.assign(globalThis, {
    location:new URL(initialURL), scrollX:0, scrollY:1400,
    history:{
      get state() {return entries[entry].state;},
      replaceState(state, _, url) {entries[entry]={state,url}; location.href=url;},
      pushState(state, _, url) {entries.splice(++entry); entries.push({state,url}); location.href=url;}
    },
    window:{
      addEventListener:on,
      scrollTo(x,y) {globalThis.scrollX=x; globalThis.scrollY=y; listeners.get("scroll")?.();}
    },
    document:{
      addEventListener:on, body:{classList:{remove() {}}},
      querySelector() {return currentShell;},
      getElementById(id) {return id === "course-navigation-status" ? notice : {scrollIntoView() {window.scrollTo(0, 200);}};},
      importNode(node) {return node;}
    },
    fetch:async url => ({ok:true,url,text:async()=>"lesson"}),
    DOMParser:class {parseFromString() {return {title:"lesson",querySelector:()=>makeShell()};}}
  });
  installCourseNavigation({beforeNavigate:async()=>{},beforeSwap:()=>{},afterSwap:()=>{},
    leave:()=>{throw new Error("Unexpected full navigation");},setBusy:busy=>{if (!busy) resolveIdle?.();}});
  return {
    entries, notice,
    async click(url) {
      const idle = new Promise(resolve => {resolveIdle=resolve;});
      let intercepted=false;
      const link={href:new URL(url,location).href,hasAttribute:name=>name==="data-course-definition"};
      listeners.get("click")({target:{closest:()=>link},button:0,preventDefault() {intercepted=true;},stopPropagation() {}});
      assert.equal(intercepted,true,"definition jumps must create a distinct history entry");
      await idle;
    },
    scroll(y) {window.scrollTo(0,y);},
    async go(delta) {
      const idle = new Promise(resolve=>{resolveIdle=resolve;});
      entry+=delta; location.href=entries[entry].url;
      listeners.get("popstate")({state:entries[entry].state});
      listeners.get("hashchange")?.();
      await idle;
    }
  };
}

for (const destination of [
  "?section=examples#definition", // Same document, different cell.
  "?section=bech32#definition", // Earlier subchapter.
  "/courses/lightning/01-intro#definition" // Earlier chapter.
]) test("Back restores the error viewport after a definition jump to " + destination, async t => {
  const origin="https://base58.test/courses/lightning/02-invoices?section=examples#error-cell";
  const browser=navigationBrowser(t,origin), originalKey=history.state.courseLesson.key;
  await browser.click(destination);
  assert.notEqual(history.state.courseLesson.key,originalKey);
  assert.equal(scrollY,200);
  // Running a long definition can move the viewport well below its anchor.
  browser.scroll(850);
  await browser.go(-1);
  assert.equal(location.href,origin);
  assert.equal(scrollY,1400,"Back must prefer the saved error viewport over #error-cell");
  await browser.go(1);
  assert.equal(scrollY,850,"Forward should restore the definition's last viewport too");
  assert.equal(browser.notice.hidden,true);
});

test("a definition link to the current URL still preserves a Back destination", async t => {
  const origin="https://base58.test/courses/lightning/02-invoices?section=examples#definition";
  const browser=navigationBrowser(t,origin);
  await browser.click(origin);
  assert.equal(browser.entries.length,2);
  assert.equal(scrollY,200);
  await browser.go(-1);
  assert.equal(scrollY,1400);
});
