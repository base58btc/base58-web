// The SDK disk adapter is deliberately tied to our pinned unkernel build. A
// checkpoint includes both its memory and its disk's identity/write generation.
// Copying only files or reseeding a disk would invalidate a machine snapshot.
const DB_NAME = "base58-course-checkpoints-v1";
const CHUNK = 1 << 20;
const request = req => new Promise((resolve, reject) => { req.onsuccess = () => resolve(req.result); req.onerror = () => reject(req.error); });
const complete = tx => new Promise((resolve, reject) => { tx.oncomplete = resolve; tx.onabort = tx.onerror = () => reject(tx.error || new Error("Checkpoint transaction failed")); });
const hash = async bytes => Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", bytes)), b => b.toString(16).padStart(2,"0")).join("");
const zip = async bytes => new Uint8Array(await new Response(new Blob([bytes]).stream().pipeThrough(new CompressionStream("gzip"))).arrayBuffer());
const unzip = async bytes => new Uint8Array(await new Response(new Blob([bytes]).stream().pipeThrough(new DecompressionStream("gzip"))).arrayBuffer());

async function openDB(name, create = false) {
  const req = indexedDB.open(name, 1);
  req.onupgradeneeded = () => {
    if (!create) { req.transaction.abort(); return; }
    for (const store of ["chunks", "rawHashes", "checkpoints", "journals"]) req.result.createObjectStore(store);
  };
  return request(req);
}

async function diskData(name) {
  const db = await openDB("kwdisk:" + name + "-v1");
  try {
    const tx = db.transaction(["blocks", "meta"]), done = complete(tx);
    const [keys, values, metaKeys, metaValues] = await Promise.all([
      request(tx.objectStore("blocks").getAllKeys()), request(tx.objectStore("blocks").getAll()),
      request(tx.objectStore("meta").getAllKeys()), request(tx.objectStore("meta").getAll())
    ]);
    await done;
    const bytes = new Uint8Array(keys.length * 4096);
    values.forEach((value, i) => bytes.set(value, i * 4096));
    return {keys, bytes, meta: metaKeys.map((key,i) => [key,metaValues[i]])};
  } finally { db.close(); }
}

async function replaceDisk(name, disk, bytes) {
  // A fresh device has not opened the SDK database yet. These are the two stores
  // defined by unkernel's IdbStore; restore their metadata without minting a UID.
  const req = indexedDB.open("kwdisk:" + name + "-v1", 1);
  req.onupgradeneeded = () => { req.result.createObjectStore("blocks"); req.result.createObjectStore("meta"); };
  const db = await request(req);
  try {
    const tx = db.transaction(["blocks","meta"], "readwrite", {durability:"strict"}), done = complete(tx);
    const blocks = tx.objectStore("blocks"), meta = tx.objectStore("meta");
    blocks.clear(); meta.clear();
    disk.keys.forEach((key,i) => blocks.put(bytes.slice(i*4096,(i+1)*4096),key));
    for (const [key,value] of disk.meta) meta.put(value,key);
    await done;
  } finally { db.close(); }
}

export class CheckpointStorage {
  constructor({workspace, journalURL, csrfToken = "", onStatus = () => {}}) {
    this.workspace = workspace; this.journalURL = journalURL; this.csrfToken = csrfToken; this.onStatus = onStatus;
    this.owner = crypto.randomUUID(); this.raw = new Map(); this.remoteChunks = new Set();
  }
  key(id) { return this.workspace + ":" + (id || "initial"); }
  async open() { this.db = await openDB(DB_NAME,true); await this.claim(); }
  async get(store,key) { const tx=this.db.transaction(store); return request(tx.objectStore(store).get(key)); }
  async put(store,key,value) { const tx=this.db.transaction(store,"readwrite",{durability:"strict"}), done=complete(tx); tx.objectStore(store).put(value,key); await done; }
  url(resource,key) { const url = new URL(this.journalURL,location.href); url.pathname += "/"+resource+"/"+key; return url.href; }
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
  async parts(bytes, chunks) {
    const refs=[];
    for (let offset=0;offset<bytes.length;offset+=CHUNK) {
      const part=bytes.subarray(offset,offset+CHUNK), raw=await hash(part);
      let digest=this.raw.get(raw) || await this.get("rawHashes",raw);
      if (!digest) {
        const compressed=await zip(part); digest=await hash(compressed);
        await this.put("chunks",digest,compressed); await this.put("rawHashes",raw,digest);
      }
      this.raw.set(raw,digest); chunks.add(digest); refs.push(digest);
    }
    return {bytes:bytes.length,refs};
  }
  async save(id, snapshot, diskName, sdk, result) {
    const chunks=new Set(), disk=await diskData(diskName);
    const memory=await this.parts(snapshot.memory,chunks), blocks=await this.parts(disk.bytes,chunks);
    const {memory:ignored,input,...metadata}=snapshot;
    const checkpoint={format:1,chunks:[...chunks],state:{sdk,metadata:{...metadata,input:Array.from(input)},memory,disk:{keys:disk.keys,meta:disk.meta,blocks},result}};
    await this.put("checkpoints",this.key(id),checkpoint);
    await this.upload(id,checkpoint);
    return checkpoint;
  }
  async upload(id,checkpoint) {
    if (!this.journalURL) return;
    await this.claim();
    let index=0;
    await Promise.all([0,1,2,3].map(async () => {
      while(index<checkpoint.chunks.length) {
        const digest=checkpoint.chunks[index++];
        if (this.remoteChunks.has(digest)) continue;
        const body=await this.get("chunks",digest);
        if (!body) throw new Error("A saved checkpoint piece is missing");
        const response=await this.fetch(this.url("chunks",digest),{method:"PUT",body});
        if (!response.ok) throw new Error("Unable to upload course checkpoint");
        this.remoteChunks.add(digest);
      }
    }));
    const response=await this.fetch(this.url("checkpoints",id || "initial"),{method:"PUT",headers:{"Content-Type":"application/json"},body:JSON.stringify(checkpoint)});
    if (!response.ok) throw new Error("Unable to upload course checkpoint");
  }
  async checkpoint(id) {
    // The server is authoritative when signed in. A locally saved but not yet
    // uploaded checkpoint must not silently advance this account's history.
    if (this.journalURL) {
      const response=await this.fetch(this.url("checkpoints",id || "initial"));
      if (response.status===404) return null;
      const checkpoint=await response.json();
      for (const digest of checkpoint.chunks) this.remoteChunks.add(digest);
      await this.put("checkpoints",this.key(id),checkpoint); return checkpoint;
    }
    return await this.get("checkpoints",this.key(id));
  }
  async bytes(parts) {
    if (!Number.isSafeInteger(parts.bytes) || parts.bytes<0 || parts.bytes>1024*1024*1024 || parts.refs.length!==Math.ceil(parts.bytes/CHUNK)) throw new Error("Invalid checkpoint size");
    const bytes=new Uint8Array(parts.bytes), decoded=new Map();
    for (let i=0;i<parts.refs.length;i++) {
      const digest=parts.refs[i];
      let part=decoded.get(digest);
      if (!part) {
        let compressed=await this.get("chunks",digest);
        if (!compressed && this.journalURL) {
          const response=await this.fetch(this.url("chunks",digest));
          if (!response.ok) throw new Error("Saved checkpoint is incomplete");
          compressed=new Uint8Array(await response.arrayBuffer());
        }
        if (!compressed || await hash(compressed)!==digest) throw new Error("Saved checkpoint failed its integrity check");
        await this.put("chunks",digest,compressed);
        part=await unzip(compressed);
        decoded.set(digest,part);
      }
      if (part.length!==Math.min(CHUNK,parts.bytes-i*CHUNK)) throw new Error("Invalid checkpoint piece length");
      bytes.set(part,i*CHUNK);
    }
    return bytes;
  }
  async restore(checkpoint,diskName,sdk) {
    if (checkpoint.format!==1 || checkpoint.state.sdk!==sdk) throw new Error("This saved course needs its original VM engine version");
    const {metadata,memory,disk}=checkpoint.state;
    const mem=await this.bytes(memory), blocks=await this.bytes(disk.blocks);
    if (blocks.length!==disk.keys.length*4096) throw new Error("Invalid saved disk");
    await replaceDisk(diskName,disk,blocks);
    return {...metadata,input:new Uint8Array(metadata.input),memory:mem};
  }
}
