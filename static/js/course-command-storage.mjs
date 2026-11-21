// Keep the existing database name so local command histories survive the switch
// to replay. Legacy snapshot stores are left untouched and never read or written.
const DB_NAME = "base58-course-checkpoints-v1";
const request = req => new Promise((resolve, reject) => { req.onsuccess = () => resolve(req.result); req.onerror = () => reject(req.error); });
const complete = tx => new Promise((resolve, reject) => { tx.oncomplete = resolve; tx.onabort = tx.onerror = () => reject(tx.error || new Error("Command history transaction failed")); });

export class CommandStorage {
  constructor({workspace, journalURL, csrfToken = ""}) {
    Object.assign(this, {workspace, journalURL, csrfToken});
    this.owner = crypto.randomUUID();
  }
  async open() {
    const req = indexedDB.open(DB_NAME, 1);
    req.onupgradeneeded = () => req.result.createObjectStore("journals");
    this.db = await request(req);
    await this.claim();
  }
  async get(store, key) { return request(this.db.transaction(store).objectStore(store).get(key)); }
  async put(store, key, value) {
    const tx = this.db.transaction(store, "readwrite", {durability:"strict"}), done = complete(tx);
    tx.objectStore(store).put(value, key); await done;
  }
  url(resource, key) { const url = new URL(this.journalURL, location.href); url.pathname += "/" + resource + "/" + key; return url.href; }
  async fetch(url, options = {}) {
    const response = await fetch(url,{...options, credentials:"same-origin", cache:"no-store", headers:{"X-CSRF-Token":this.csrfToken,"X-Course-Session":this.owner,...options.headers}});
    if (!response.ok && response.status !== 404) throw new Error((await response.text()).trim() || "Unable to save course work");
    return response;
  }
  async claim() {
    if (!this.journalURL) return;
    const response = await this.fetch(this.url("lease","session"),{method:"POST"});
    if (!response.ok) throw new Error("Course history is unavailable");
    clearInterval(this.heartbeat);
    this.heartbeat = setInterval(() => { this.fetch(this.url("lease","session"),{method:"POST"}).catch(() => {}); },30000);
  }
  async release() {
    clearInterval(this.heartbeat);
    if (this.journalURL) await this.fetch(this.url("lease","session"),{method:"DELETE",keepalive:true});
  }
  async readJournal() {
    if (!this.journalURL) return await this.get("journals",this.workspace) || [];
    const response = await this.fetch(this.journalURL);
    if (!response.ok) throw new Error("Command history is unavailable; apply the course history migration");
    return (await response.json()).events;
  }
  async appendEvent(event, revision) {
    if (!this.journalURL) {
      const events = await this.readJournal();
      if (events.length !== revision) throw new Error("Command history changed; reload");
      events.push(event); await this.put("journals",this.workspace,events); return events;
    }
    const response = await this.fetch(this.journalURL,{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({expectedRevision:revision,event})});
    if (!response.ok) throw new Error("Unable to save command history");
    return (await response.json()).events;
  }
}
