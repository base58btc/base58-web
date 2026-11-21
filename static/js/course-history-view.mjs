export function renderCommandHistory(container, state) {
  const active=new Set();
  for (let id=state.head;id;id=state.commands.get(id)?.parent) active.add(id);
  container.replaceChildren();
  const entries=[...state.commands.values()].map((entry,index)=>[index,entry]);
  const current=entries.filter(([,entry])=>entry.workspace===state.workspace);
  const archived=entries.filter(([,entry])=>entry.workspace!==state.workspace);
  if (!current.length) {
    const empty=document.createElement("p");
    empty.textContent=state.workspace ? "Fresh workspace · no commands to replay." : "Your CLI commands will appear here.";
    container.append(empty);
  }
  appendEntries(container,current,active);
  if (archived.length) {
    const archive=document.createElement("details"), summary=document.createElement("summary");
    summary.textContent="Archived commands ("+archived.length+")";
    archive.append(summary); container.append(archive);
    appendEntries(archive,archived,active,true);
  }
}

function appendEntries(container, entries, active, archived=false) {
  if (entries.length>100) {
    const note=document.createElement("p");
    note.textContent="Showing the latest 100 commands. All commands remain saved.";
    container.append(note);
  }
  for (const [index,entry] of entries.slice(-100)) {
    const details=document.createElement("details"), summary=document.createElement("summary"), output=document.createElement("pre");
    const status=entry.interrupted ? "interrupted" : archived ? "archived" : !entry.result ? "saving" : active.has(entry.id) ? "saved" : "undone";
    summary.textContent=(index+1)+". "+[entry.command.program,...entry.command.args].map(arg=>JSON.stringify(arg)).join(" ")+" · "+status;
    output.textContent=entry.result ? (entry.result.stdout || "")+(entry.result.stderr || "")+"\nOriginal exit status: "+entry.result.exitCode : entry.interrupted ? "The session ended before its result was recorded. This command is excluded from replay." : "Waiting for the command result to be recorded.";
    details.append(summary,output); container.append(details);
  }
}
