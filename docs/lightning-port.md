# Lightning notebook port

Source: [base58btc/lightning_class](https://github.com/base58btc/lightning_class/tree/62491b76177123c8cebcc0fadccc93ca826d8dc7), commit `62491b76177123c8cebcc0fadccc93ca826d8dc7` (November 2023 workshop).

The port lives in `courses/lightning`. These are native course Markdown pages, not embedded notebooks or exported notebook HTML. The normal course renderer supplies navigation, editable code cells, syntax highlighting, notes, multiple-choice questions, and saved code/output.

| Original notebook | Course Markdown |
| --- | --- |
| `CSV and CLTV.ipynb` | `01-csv-and-cltv.md` |
| `Invoices.ipynb` | `02-invoices.md` |
| `Revocation Generation.ipynb` | `03-revocation-generation.md` |
| `Lightning Gossip for Fun + Profit.ipynb` | `04-gossip-for-fun-and-profit.md` and `05-build-and-send-an-onion.md` |

`00-welcome.md` links to the lessons. Student-facing pages omit VM implementation explanations. The original explanations, invoice examples, five revocation vectors, example channel, and routing endpoints are retained. Historical execution outputs and empty notebook cells are omitted. The notebooks’ algorithms are visible in editable cells; `lab.py` only handles local CLI calls and regtest setup.

## Python execution

The new fence is explicit:

````markdown
+++ invoice-signature python-vm
from coincurve import PublicKey
...
+++
````

Existing `+++` and `+++ id` cells still use Pyodide. All Lightning cells use `python-vm`. The runtimes have separate globals and packages; do not mix them when a lesson needs shared Python variables.

The course flake supplies Python, `coincurve` (native `libsecp256k1` bindings), the original pinned `pyln_client`, `pyln_proto`, and `pyln_bolt7` forks, Bitcoin Core, Core Lightning, Bash, and small CLI utilities. Dependencies are built into the publish bundle. There is no student-side `pip`, Nix build, or external node connection.

`CourseVM.executePython` records each cell's CLI invocation, original source, and cell ID in the command journal. `static/guest/course-python.py` maintains one Python namespace in the live VM. The browser starts it once with `--stdio` and exchanges framed JSON over its open input/output pipes; subsequent cells launch neither a shell nor another Python client. The VM and interpreter remain running between cells. Older recorded `--execute` arguments use this same transport when replayed. On resume or undo, a fresh VM replays the completed cells on the active history branch; it does not restore memory or disk snapshots. Each cell is one journal command; individual RPCs/subprocesses called inside it are not separate entries.

The protocol uses private descriptors so student `input()` sees EOF and raw subprocess output cannot corrupt responses. A lost interpreter/pipe stops execution and requires replay or undo; it never silently creates an empty Python namespace. The Unix-socket CLI remains available for standalone compatibility, but browser cells do not launch that client.

Lesson navigation replaces the course page content while keeping the VM and interpreter alive. Sidebar, previous/next, keyboard shortcuts, and browser back/forward share this navigation path. Running cells finish and save before the content changes; editor drafts and answers survive navigation. A different attempt/version, preview mode, or course requires a full navigation. Reloading, leaving the course, or Save and stop closes the VM; reopening reconstructs it from the journal.

Chapters with multiple level-two headings display one subchapter at a time: an overview followed by each heading's section. The Lightning Markdown headings define the views linked from the sidebar and Previous/Next controls. A `?section=...` URL opens a particular subchapter; `?section=last` supports returning to the end of the previous chapter. Pagination preserves the original chapter path and cell IDs for saved answers, editor drafts, outputs, and command history. Template-only pagination changes preserve the bundle and history. Runtime-input edits require a new matching bundle; lesson Markdown edits reuse it. Without JavaScript, the complete chapter is still readable.

The first visible lesson with VM cells starts preparing its workspace just after the page paints, without waiting for Play. Preparation loads the tools, replays the recorded history, and warms the persistent Python interpreter for Python lessons. It runs no new lesson cells and adds no synthetic commands to the journal. An early Play click waits for the same preparation; later clicks use the warm interpreter. Save and stop cancels background startup and suppresses further automatic startup until the student explicitly resumes or runs a cell. Loading details stay in the Workspace menu.

Replay can generate different wallet keys, addresses, invoices, timestamps, and transaction IDs. Original output remains in the journal. Changes outside recorded cells are not saved. Replay stops on a changed exit status or timeout, and Undo can move the history back to an earlier prefix.

Workspace → **Reset workspace** confirms before starting a fresh VM without replaying earlier commands. A durable `reset` journal event archives the previous commands and results; the next command starts a new history root. The reset syncs with account history and also works in local previews. Local preview answers and editor drafts are scoped to the course content hash; previous versions remain stored, and cannot overwrite new exercise starters. Code edits and lesson answers stay, while archived cell output is cleared from the lesson. Rerun prerequisite cells to rebuild their variables, files, and nodes. Reset can cancel background replay and recover a failed replay; Undo cannot return to an archived workspace.

The runner prints the final expression, captures Python output and exceptions, limits output and cell source size, and applies a 90-second Python alarm inside the outer 120-second CLI timeout. This is ordinary Python, not an IPython kernel: notebook magics are replaced with explicit Python helpers. Interactive input and rich HTML displays are not implemented.

## Code challenges and questions

The revocation and route-finding exercises use the existing `???` challenge format
with `id:` followed by `runtime: python-vm`. The four sections separated by `---`
are prompt, editable starter, checks, and failure feedback. Omit `runtime:` to
retain the existing Pyodide challenge behavior.

Both Play and Run Tests use the course Python session. Run Tests records the
student's source and checks together as one `python3 --check` journal command;
replay therefore reruns the actual submitted attempt, including failed checks.
Student definitions remain available to following cells. Check-only names are
kept out of the notebook namespace. Checks are instructional feedback, not a
security boundary or server-authoritative grading system.

The course has two code challenges and twelve multiple-choice questions. Existing
questions are retained; added questions cover invoice information and units,
commitment sequence flags, forwarding direction, fees, CLTV deltas, and final-hop
payloads. Reference implementations used by automated validation live under
`scripts/testdata`, outside the published course sources.

## References in course Markdown

Use a `::: references` block for a numbered bibliography with linked titles and
short descriptions. Indent each description by four spaces in a paragraph beneath its
link. The renderer supplies the References heading and a subdued callout style.
Use this format for all lesson references, including a single source or the
original course material, so every page presents its sources consistently.

```markdown
::: references
1. [BIP 65 — Absolute locktimes](https://github.com/bitcoin/bips/blob/master/bip-0065.mediawiki)

    The specification for `OP_CHECKLOCKTIMEVERIFY`.
:::
```

## Adaptations and corrections

- Timelocks retain explicit UTXO selection, annotated raw funding transaction assembly with `codes.cleanup_tx`, wallet signing, and broadcasting. They use freshly funded wallet outputs and dynamic heights. The delay is six blocks rather than 1000. Fixed integer serialization, recorded funding transaction IDs, and checks immediately before/after maturity replace stale outputs from the notebook.
- Invoice decoding retains both original examples. It checks the Bech32 checksum, preserves unknown tags, checks field boundaries, handles zero-padding correctly, and verifies an explicit payee against the recovered key. This remains a teaching parser, not complete wallet invoice validation.
- Secret generation restores the original unfinished exercise: students must fix the missing assignment of the bit-flipped bytes to pass all five vectors. The test runner verifies that the starter fails, then validates the minimal correction. Index zero remains valid. Payment basepoints are derived from the original private integers 666 and 777. Commitment number encoding distinguishes the increasing commitment number from the decreasing secret index.
- Routing uses the original `Gossmap`, node, channel, and half-channel objects over the full historical store. Terminating breadth-first search finds a route; selected policies are converted to dictionaries for fee and onion calculations. Amount/fee/CLTV calculations work backward from the recipient and exclude the sender’s own fee. Capacity is explicitly distinguished from available liquidity.
- Onion creation binds associated data to the **payment hash**, includes the invoice’s final CLTV delta, and uses relative delay only for `sendonion`’s first-hop instruction. The payment runs on Alice → Bob → Carol, separately from the historical mainnet graph.
- Bitcoin Core 31.0, Core Lightning 26.04.1, Python 3.13, and coincurve 21.0.0 come from the pinned nixpkgs revision. The three Python `pyln` forks use their original source revisions and hashes with explicit dependencies for modern Python. The Bech32 decoder remains visible in a lesson cell rather than importing the original Bech32 fork.
- Original `codes.py`, `hodlinvoice.py`, and `startup_regtest.sh` are included. The startup script omits the now-obsolete `experimental-offers` and `experimental-splicing` flags, creates its data directories, supports a separate test port base, requests Bech32 wallet addresses explicitly, and fixes iteration over explicitly supplied node numbers. Its three-node setup and matching dual-funded channels were validated locally. These are supplementary tools; lesson execution still uses `lab.py`. `lab.py` keeps the three-node, single-funded payment setup.

## Dataset provenance

The runtime contains the complete original `nov_17.gossip_store`: 174,794,305 bytes,
14,137 nodes and 62,309 channels. The flake fetches the original Git LFS object
from the pinned upstream commit and verifies SHA256
`8db52d5a9b4ac8c637bafd82447a84c74fc7c866ca3c73ae3fdce817c609e8cd`.
It is mounted read-only from the Nix closure; `LIGHTNING_GOSSIP_STORE` identifies
it. Nix also builds a lossless index using the original `pyln.client.Gossmap` parser. `gossip.load_gossip()` reconstructs the original node, channel, and half-channel objects from that trusted index; it preserves the full store and its metadata. Native validation fingerprints every announcement and directional update against the unmodified parser. This avoids parsing nearly a million historical records each time the browser starts the lesson.

`data/gossip.json` and `scripts/extract-lightning-gossip.py` retain the earlier
64-node subset for provenance, but the lessons no longer use that subset.

## Local validation and rendered preview

The current validation executes 44 Markdown blocks (42 runnable cells and two code challenges), including manual timelock
funding and the Alice → Bob → Carol onion payment. The revocation starter is
expected to fail its vectors; the runner explicitly checks that failure before
applying and validating the intended correction. The route-finding exercise also starts unfinished; its checks cover shortest paths, disabled directions, cycles, unreachable destinations, and identical endpoints. Native validation does not
replace the Linux/browser checks below.

From the repository root:

```sh
# Execute the Python-only lessons and protocol edge cases.
nix develop path:./courses/lightning --command python3 scripts/test-lightning-course.py

# Also execute the actual Bitcoin/Lightning cells against fresh temporary nodes.
# Uses local ports 18643 and 29731–29733 and stops its own nodes afterward.
nix develop path:./courses/lightning --command python3 scripts/test-lightning-course.py --regtest

# Persistent interpreter protocol, errors, timeouts, and output bounds.
python3 scripts/test-course-python.py

# Render the actual production lesson templates (requires the bundle below).
BASE58_RENDER_COURSE_DIR="$PWD/target/lightning-preview" \
  nix develop --command go test ./internal/handlers -run '^TestLightningCourseMarkdown$' -count=1
python3 scripts/preview-course-markdown.py
```

Open `http://127.0.0.1:8099/courses/lightning/00-welcome`. The preview serves all
six actual lesson pages, the prepared environment endpoint, and the NAR archives.
Use the sidebar to work through the lessons in order. Python,
Bitcoin, and Lightning execute in the browser VM. Executed cell text/output and
quiz answers persist in local browser storage; the command journal
persists in IndexedDB. This local mode needs no database, login, or publishing
credentials and does not sync to a course account.

The server requires a matching `courses/lightning/course-runtime.json` and
`runtime/` directory. It rejects stale runtime inputs or missing archives. Use
`--course-dir /path/to/extracted/courses/lightning` for a downloaded bundle, but
render the pages from those same sources. Re-render after changing lesson text or
templates, then reload the page; no bundle build or preview-server restart is
needed. The server refreshes source files and the content fingerprint when serving
lessons and environment requests. The lesson preview has a separate VM workspace from the terminal
preview, so terminal experiments do not advance the lesson environment.

## Build the Linux bundle with GitHub Actions

`.github/workflows/lightning-runtime.yml` uses GitHub's native
`ubuntu-24.04-arm` runner. Pushing runtime-input or bundler changes to the
`codex/lightning-runtime-build` branch starts the build. Markdown-only, generated
manifest, template, and CSS changes do not trigger it. Markdown under `nix/` is
a build input. After the workflow is on the default branch, it can also be
started manually from the Actions tab. It needs no server or publishing secrets.

The job tests the bundler, builds the locked `aarch64-linux` closure, verifies
every archive's size and SHA-256, and validates the 44 executable blocks (including both unfinished exercises) against fresh
Bitcoin/Lightning regtest nodes. It uploads an artifact named
`lightning-runtime-<commit>` containing `lightning-course.tar`, `SHA256SUMS`, and
`BUILD_COMMIT`. The tar contains the matching course sources, manifest, and NAR
archives. Download it within seven days; the workflow does not publish a course.

The [exercise build](https://github.com/base58btc/base58-web/actions/runs/44876607845)
uses commit `f0117bf20f508e310074eea2a3659ed0b1b5b97b`. It passes all 44 executable
block checks on ARM64 Linux, including both unfinished starters and their
reference solutions, the complete graph fingerprint, timelocks, and the three-node
payment. Its 103 archives total 338.3 MiB compressed. The downloaded bundle
passed source, archive-size, and SHA-256 verification locally.

The local manifest now uses format 2 with separate runtime and content
fingerprints. Its existing archives were verified and reused: the Lightning flake
imports `gossip.py` but does not consume lesson Markdown or the complete flake
source tree. No Linux rebuild was needed for this conversion. The content hash,
and therefore the existing command-history and local-progress keys, stayed the
same. Future runtime changes use the filtered source tree described in the README.

Before adding the graded challenge controls, the restored course passed all 43 cell checks in the browser, including the
expected exercise failure/correction and Carol receiving 10,000,000 msat. The
indexed full graph loaded in 35 seconds on the validation machine; routing took
0.5 seconds, and ordinary Python cells completed in about 0–0.1 seconds. The VM
now has a 1 GiB memory budget because loading the complete graph exceeds 512 MiB.
These timings are measurements from this machine, not cross-device guarantees.

The current browser checks also cover failed starters and passing solutions through
both Run Tests buttons, the submitted revocation function being available in the
next section, saved code and grading surviving a reload, and correct/incorrect
question feedback. The sidebar scrolls to Workspace in a 500-pixel-high desktop
window; the mobile contents drawer retains its own scrolling.

To inspect a downloaded artifact without overwriting local edits:

```sh
gh run download RUN_ID --repo base58btc/base58-web \
  --name lightning-runtime-COMMIT --dir target/lightning-download
cd target/lightning-download
shasum -a 256 -c SHA256SUMS
mkdir extracted
tar -xf lightning-course.tar -C extracted
cd ../..
nix run .#course-sync -- verify-bundle --dir target/lightning-download/extracted/courses/lightning
```

For local execution, install the pinned VM assets with `make course-vm-assets`
and run `python3 scripts/preview-course-vm.py --dir /absolute/path/to/extracted/courses/lightning`.
Both the terminal preview and the actual lesson preview above work without a
database or asset uploads. Only runtime-input changes require a new bundle;
lesson edits reuse the existing archives.

The Lightning environment starts its services in lesson cells, not at boot.
In the terminal preview, initialize the funded regtest wallet with:

```sh
python3 -c "from lab import start_bitcoin; print(start_bitcoin())"
```

This creates `/data/workspace/.lab/bitcoin`, starts Bitcoin Core, and mines the
initial 101 blocks. Then query it with:

```sh
bitcoin-cli -regtest -datadir=/data/workspace/.lab/bitcoin getblockchaininfo
```

The generic preview's `pwd` command works before this initialization. The
`/data/workspace/node` directory from the README's separate example flake is not
used by the Lightning course.

The browser mounts archives in manifest order. Earlier versions stored machine
checkpoints; that path is retired. Existing command journals can be replayed,
while old snapshot data is retained without being read or updated. Replayed values
can differ from the original outputs shown in the course.

## Browser validation and publishing

The Linux build and native lesson tests do not replace browser acceptance checks.
Native execution does not validate the browser emulator's memory requirements or
native Python/three-node command replay.

On an aarch64 Linux Nix builder, with `course-sync` available:

```sh
course-sync bundle --dir ./courses/lightning
course-sync sync --server https://YOUR-COURSE-SERVER --course lightning --dir ./courses/lightning
```

Review the draft with the bundled environment, then publish through the normal course workflow. Rebuild after runtime-input changes; lesson-only edits reuse the bundle. No deployment or publication is performed by this port.

Before publishing, run the lessons in a supported browser, confirm the 10,000-satoshi payment settles, stop and resume on another device, verify Python globals and all three nodes are rebuilt by replay, and undo the send cell to verify the replayed invoice is unpaid. Generated identifiers may differ. Measure initial download, replay duration, and memory use against the 1 GiB VM budget (the full graph exceeds the previous 512 MiB limit during loading).
