import test from "node:test";
import assert from "node:assert/strict";
import {sameCourseSession, isLessonURL, NavigationRequest} from "./course-navigation.mjs";

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
