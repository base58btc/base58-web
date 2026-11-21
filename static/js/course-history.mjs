export function journalState(events) {
  const state = {head: "", pending: "", commands: new Map()};
  for (const event of events) {
    if (event.type === "command") {
      if (state.pending || (event.parent || "") !== state.head) throw new Error("Invalid command history");
      state.commands.set(event.id, {...event}); state.pending = event.id;
    } else if (event.type === "result" || event.type === "interrupted") {
      if (!state.pending || event.commandId !== state.pending) throw new Error("Invalid command result history");
      const entry = state.commands.get(event.commandId);
      if (event.type === "result") { entry.result = event.result; state.head = event.commandId; }
      else entry.interrupted = true;
      state.pending = "";
    } else if (event.type === "checkout") {
      let cursor = state.head;
      while (cursor && cursor !== (event.head || "")) cursor = state.commands.get(cursor).parent || "";
      if (state.pending || cursor !== (event.head || "")) throw new Error("Invalid history checkout");
      state.head = cursor;
    } else throw new Error("Unsupported command history event");
  }
  return state;
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
