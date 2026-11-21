# Lightning notebook port

Source: [base58btc/lightning_class](https://github.com/base58btc/lightning_class/tree/62491b76177123c8cebcc0fadccc93ca826d8dc7), commit `62491b76177123c8cebcc0fadccc93ca826d8dc7` (November 2023 workshop).

The port lives in `courses/lightning`. These are native course Markdown pages, not embedded notebooks or exported notebook HTML. The normal course renderer supplies navigation, editable code cells, syntax highlighting, notes, multiple-choice questions, and saved code/output.

| Original notebook | Course Markdown |
| --- | --- |
| `CSV and CLTV.ipynb` | `01-csv-and-cltv.md` |
| `Invoices.ipynb` | `02-invoices.md` |
| `Revocation Generation.ipynb` | `03-revocation-generation.md` |
| `Lightning Gossip for Fun + Profit.ipynb` | `04-gossip-for-fun-and-profit.md` and `05-build-and-send-an-onion.md` |

`00-welcome.md` introduces the environment and command-replay behavior. The original explanations, invoice examples, five revocation vectors, example channel, and routing endpoints are retained. Historical execution outputs and empty notebook cells are omitted. The notebooks’ algorithms are visible in editable cells; `lab.py` only handles local CLI calls and regtest setup.

## Python execution

The new fence is explicit:

````markdown
+++ invoice-signature python-vm
from coincurve import PublicKey
...
+++
````

Existing `+++` and `+++ id` cells still use Pyodide. All Lightning cells use `python-vm`. The runtimes have separate globals and packages; do not mix them when a lesson needs shared Python variables.

The course flake supplies Python and `coincurve` (native `libsecp256k1` bindings), Bitcoin Core, Core Lightning, Bash, and small CLI utilities. Dependencies are built into the publish bundle. There is no student-side `pip`, Nix build, or external node connection.

`CourseVM.executePython` records each cell's CLI invocation, original source, and cell ID in the command journal. `static/guest/course-python.py` maintains one Python namespace in the live VM. Both remain running between cells. On resume or undo, a fresh VM replays the completed cells on the active history branch; it does not restore memory or disk snapshots. Each cell is one journal command; individual RPCs/subprocesses called inside it are not separate entries.

Replay can generate different wallet keys, addresses, invoices, timestamps, and transaction IDs. Original output remains in the journal. Changes outside recorded cells are not saved. Replay stops on a changed exit status or timeout, and Undo can move the history back to an earlier prefix.

The runner prints the final expression, captures Python output and exceptions, limits output and cell source size, and applies a 90-second Python alarm inside the outer 120-second CLI timeout. This is ordinary Python, not an IPython kernel: notebook magics are replaced with explicit Python helpers. Interactive input and rich HTML displays are not implemented.

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

- Timelocks use freshly funded wallet outputs and dynamic heights. The delay is six blocks rather than 1000. Fixed integer serialization, recorded funding transaction IDs, and checks immediately before/after maturity replace stale outputs from the notebook.
- Invoice decoding retains both original examples. It checks the Bech32 checksum, preserves unknown tags, checks field boundaries, handles zero-padding correctly, and verifies an explicit payee against the recovered key. This remains a teaching parser, not complete wallet invoice validation.
- Secret generation uses the returned bit-flipped bytes and accepts index zero. Commitment number encoding distinguishes the increasing commitment number from the decreasing secret index.
- Routing uses terminating breadth-first search and consistent half-channel dictionaries. Amount/fee/CLTV calculations work backward from the recipient and exclude the sender’s own fee. Capacity is explicitly distinguished from available liquidity.
- Onion creation binds associated data to the **payment hash**, includes the invoice’s final CLTV delta, and uses relative delay only for `sendonion`’s first-hop instruction. The payment runs on Alice → Bob → Carol, separately from the historical mainnet graph.
- The 2023 release candidate/forked Python dependencies are replaced with tools from the site’s pinned nixpkgs revision. Current pin: Bitcoin Core 31.0, Core Lightning 26.04.1, Python 3.13, coincurve 21.0.0. `lab.py` uses the CLIs directly, so pyln, Jupyter, and the custom Bech32 fork are not runtime dependencies. The original `hodlinvoice.py` plugin is not used by the notebooks and is not included in this port.

## Dataset provenance

`data/gossip.json` contains 64 real nodes and 558 directed channel updates extracted from the original Git LFS object. It retains the original endpoints and `803775x1091x1`, then adds a bounded neighborhood. The exact selection, original hash, revision, size, and full channel count are recorded in the JSON. It is an offline historical sample, not a live network snapshot.

To reproduce it:

```sh
curl -L --fail -o /tmp/nov_17.gossip_store \
  https://media.githubusercontent.com/media/base58btc/lightning_class/62491b76177123c8cebcc0fadccc93ca826d8dc7/gossip_stores/nov_17.gossip_store
python3 scripts/extract-lightning-gossip.py /tmp/nov_17.gossip_store courses/lightning/data/gossip.json
```

The extractor requires SHA256 `8db52d5a9b4ac8c637bafd82447a84c74fc7c866ca3c73ae3fdce817c609e8cd`; the input is 174,794,305 bytes. It intentionally supports this pinned historical store, not arbitrary future store formats.

## Local validation and rendered preview

All 37 executable Markdown cells pass on the local Nix-provided tools, including
the timelock acceptance checks and Alice → Bob → Carol onion payment. The Go
suite, 14 JavaScript tests, and Python session tests pass. The six lesson pages,
question feedback, and mobile layout were checked in the browser. These are local
validation results; they do not replace the Linux/browser checks below.

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
Run the welcome cell, then use the sidebar to work through the lessons. Python,
Bitcoin, and Lightning execute in the browser VM. Executed cell text/output and
quiz answers persist in local browser storage; the command journal
persists in IndexedDB. This local mode needs no database, login, or publishing
credentials and does not sync to a course account.

The server requires a matching `courses/lightning/course-runtime.json` and
`runtime/` directory. It rejects stale sources or missing archives. Use
`--course-dir /path/to/extracted/courses/lightning` for a downloaded bundle, but
render the pages from those same sources. Re-render after changing lesson text or
templates. The lesson preview has a separate VM workspace from the terminal
preview, so terminal experiments do not advance the lesson environment.

## Build the Linux bundle with GitHub Actions

`.github/workflows/lightning-runtime.yml` uses GitHub's native
`ubuntu-24.04-arm` runner. Pushing the `codex/lightning-runtime-build` branch starts
the one-off build. After the workflow is on the default branch, it can also be
started manually from the Actions tab. It needs no server or publishing secrets.

The job tests the bundler, builds the locked `aarch64-linux` closure, verifies
every archive's size and SHA-256, and executes all 37 lesson cells against fresh
Bitcoin/Lightning regtest nodes. It uploads an artifact named
`lightning-runtime-<commit>` containing `lightning-course.tar`, `SHA256SUMS`, and
`BUILD_COMMIT`. The tar contains the matching course sources, manifest, and NAR
archives. Download it within seven days; the workflow does not publish a course.

The [first successful build](https://github.com/base58btc/base58-web/actions/runs/44863287070)
used commit `6cc76009f63b36116f3c1a498ed39bbf634bf88b` and passed all 37 cells on
ARM64 Linux. Its 92 runtime archives total 188.5 MiB compressed. The downloaded
bundle passed source and archive verification locally, and the browser VM ran
Python, coincurve's native secp256k1 public-key derivation, `bitcoind --version`,
and `lightningd --version` successfully. This smoke test does not cover the full
three-node lesson sequence or cross-device command replay in the browser.

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
database or asset uploads. Any changes to the course sources require a new bundle.

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

Review the draft with the bundled environment, then publish through the normal course workflow. Rebuild after any course source change. No deployment or publication is performed by this port.

Before publishing, run the lessons in a supported browser, confirm the 10,000-satoshi payment settles, stop and resume on another device, verify Python globals and all three nodes are rebuilt by replay, and undo the send cell to verify the replayed invoice is unpaid. Generated identifiers may differ. Measure initial download, replay duration, and memory use against the current 512 MiB VM budget.
