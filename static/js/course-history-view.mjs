export function renderCommandHistory(container, state) {
  const active=new Set();
  for (let id=state.head;id;id=state.commands.get(id)?.parent) active.add(id);
  container.replaceChildren();
  const entries=[...state.commands.values()];
  if (!entries.length) { container.textContent="Your CLI commands will appear here."; return; }
  for (const [index,entry] of entries.map((entry,index)=>[index,entry]).slice(-100)) {
    const details=document.createElement("details"), summary=document.createElement("summary"), output=document.createElement("pre");
    const status=entry.interrupted ? "interrupted" : !entry.result ? "saving" : active.has(entry.id) ? "saved" : "undone";
    summary.textContent=(index+1)+". "+[entry.command.program,...entry.command.args].map(arg=>JSON.stringify(arg)).join(" ")+" · "+status;
    output.textContent=entry.result ? (entry.result.stdout || "")+(entry.result.stderr || "")+"\nOriginal exit status: "+entry.result.exitCode : entry.interrupted ? "The session ended before its result was recorded. This command is excluded from replay." : "Waiting for the command result to be recorded.";
    details.append(summary,output); container.append(details);
  }
}
