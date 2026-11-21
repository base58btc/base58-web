export function journalState(events) {
  const state = {head: "", pending: "", workspace: "", commands: new Map()};
  for (const event of events) {
    if (event.type === "command") {
      if (state.pending || (event.parent || "") !== state.head) throw new Error("Invalid command history");
      state.commands.set(event.id, {...event, workspace: state.workspace}); state.pending = event.id;
    } else if (event.type === "result" || event.type === "interrupted") {
      if (!state.pending || event.commandId !== state.pending) throw new Error("Invalid command result history");
      const entry = state.commands.get(event.commandId);
      if (event.type === "result") { entry.result = event.result; state.head = event.commandId; }
      else entry.interrupted = true;
      state.pending = "";
    } else if (event.type === "reset") {
      if (state.pending || event.head || event.parent || event.command || event.commandId || event.result) throw new Error("Invalid workspace reset");
      state.head = "";
      state.workspace = event.id;
    } else if (event.type === "checkout") {
      let cursor = state.head;
      while (cursor && cursor !== (event.head || "")) cursor = state.commands.get(cursor).parent || "";
      if (state.pending || cursor !== (event.head || "")) throw new Error("Invalid history checkout");
      state.head = cursor;
    } else throw new Error("Unsupported command history event");
  }
  return state;
}

// Follow the active branch, not every historical execution (which includes
// undone branches and interrupted commands).
export function replayCommands(state, head = state.head) {
  const entries = [], seen = new Set();
  for (let id = head; id; ) {
    const entry = state.commands.get(id);
    if (!entry?.result || entry.interrupted || seen.has(id)) throw new Error("Invalid replay history");
    seen.add(id); entries.push(entry); id = entry.parent || "";
  }
  return entries.reverse();
}

export const newEvent = (type, fields = {}) => ({id: crypto.randomUUID(), type, time: new Date().toISOString(), ...fields});

export class CommandJournal {
  constructor(storage, onHistory) { this.storage = storage; this.onHistory = onHistory; this.events = []; }
  async load() {
    this.events = await this.storage.readJournal(); this.update();
  }
  update() { this.state = journalState(this.events); this.onHistory?.(this.state); }
  async append(event) {
    journalState([...this.events, event]);
    this.events = await this.storage.appendEvent(event, this.events.length);
    this.update();
  }
}
