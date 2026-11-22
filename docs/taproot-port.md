# Taproot notebook port

Source: [base58btc/taproot_class](https://github.com/base58btc/taproot_class/tree/5416589c49c9fd0424cff64c6e60090c2feb3b3c), commit `5416589c49c9fd0424cff64c6e60090c2feb3b3c` (November 2023 workshop).

The native course Markdown lives in `courses/taproot`. It reuses the existing
`taproot` catalog identity and artwork, email-verified enrollment, course attempts,
subchapter navigation, saved answers, definition backlinks, and command replay.
All executable cells use the persistent Nix Python interpreter in the browser VM.

| Original notebook | Course lessons |
| --- | --- |
| `Welcome to Taproot!.ipynb` | Introduction; finding Taproot; Schnorr signatures; building an output; spending a Tapscript; balanced and unbalanced trees; OP_CHECKSIGADD |
| `MuSig2 + FROST.ipynb` | MuSig2 and Tapscript spends; FROST threshold signatures |

There are eight lesson chapters plus an introduction, 188 executable blocks
(including three coding challenges), and eight multiple-choice questions. Cell IDs
`taproot-wNNN` and `taproot-mNNN` identify the zero-based upstream notebook cell,
so the source and port can be compared directly. `taproot-start` initializes the
regtest wallet. Empty cells and historical execution outputs are omitted.

The port retains the original class scripts, historical Schnorr transaction and
signature example, visible transaction parsers and signing algorithms, raw
transaction assembly, balanced and unbalanced trees, proof construction, and all
three multisignature approaches. The tagged-hash, TapLeaf, and tree-building
implementation prompts are graded challenges. Their reference solutions live in
`scripts/testdata`, outside published course sources. The missing early TapLeaf
implementation uses the definition supplied later in the original notebook.
Questions preserve the original topics and give answer feedback.

## Adaptations

- IPython shell captures and `jq` pipelines become explicit `bitcoin(method, ...args)`
  calls. `lab.py` starts one isolated regtest node and funded wallet, invokes the
  CLI with argument arrays, and finds an output by scriptPubKey. It performs no
  cryptographic calculations for the student.
- Historical funding txids, output indices, addresses, and completed raw spends
  become values from the student's current wallet. The original hand-assembled
  transaction fields remain editable; every final spend is checked by Bitcoin Core.
- The previously missing `pprint` import and early `make_leaf` definition are
  supplied. CompactSize encoding in `codes.py` includes the 65535 and 4294967295
  boundaries in their shortest encodings. Other teaching helpers remain upstream.
- Historical signature verification reads its key from the spent output, so
  exploring another address in the first chapter does not change that example.
- References use the shared bibliography callout. Notebook code cells containing
  only section headings become Markdown headings. Raw transaction illustrations
  use current values instead of stale session-specific hex dumps.
- The MuSig2 names 999, 1110, and 1221 remain as labels for signer pairs; the text
  explains that BIP327 key aggregation uses coefficients rather than private-key
  addition. The included `bip327.py` is unchanged from upstream.
- The FROST notebook imports a nonexistent `frost_niftys_version`. The port uses
  the supplied, unchanged `frost.py`. It demonstrates the original direct output-key
  spend, without claiming a TapTweak implementation. Its unsigned transaction omits
  the SegWit marker until a witness exists. The final signature is independently
  verified with libsecp256k1, accepted by Bitcoin Core, broadcast, and mined.
- These remain workshop implementations with public example keys and explicit
  demonstration Schnorr nonces, not wallet signing libraries. The original FROST
  and MuSig2 source warnings are retained.

## Runtime and validation

The flake pins the same nixpkgs revision as Lightning, with Python 3.13,
coincurve 21.0.0, Bitcoin Core 31.0, Bash, Coreutils, and jq. There is no Core
Lightning daemon, gossip dataset, Jupyter, student-side package installation,
or Python dependency in the website's Docker build.

[GitHub Actions run 44966726796](https://github.com/base58btc/base58-web/actions/runs/44966726796)
built commit `d284234d8b85aae1272c9e28488c148dae09606a` on native ARM64 Linux.
It passed all 188 blocks, checked all three unfinished starters and their solutions,
and accepted six Taproot spends. Its 78 archives total 141.2 MiB compressed.
The downloaded tar checksum and every runtime archive were verified locally.
The runtime fingerprint is
`283c4f7be4c2c6bcd52c34fdcf60a14cbf3c50b5828120a6d83380945fe96686`.
Later Markdown and template edits reuse these archives.

Run from the repository root:

```sh
# Every lesson cell, with fresh temporary nodes and an unused local RPC port.
nix develop path:./courses/taproot --command python3 scripts/test-taproot-course.py

# Render the production templates for a standalone preview.
BASE58_RENDER_COURSE_DIR="$PWD/target/taproot-preview" \
  nix develop --command go test ./internal/handlers \
  -run '^TestTaprootCourseMarkdown$' -count=1
python3 scripts/preview-course-markdown.py --course taproot \
  --course-dir courses/taproot --dir target/taproot-preview --port 8123
```

Open `http://127.0.0.1:8123/courses/taproot/00-welcome`. This preview uses its own
browser-local history and answers, without an account or email. It requires the
prepared bundle and the normal VM support assets (`make course-vm-assets`).
The production renderer test checks every lesson's IDs, navigation, executable
cells, question feedback, and Markdown fences. Postgres integration tests exercise
catalog activation, verified signup, enrollment, runtime access, and dashboard resume.

## Rebuild and publish

`.github/workflows/taproot-runtime.yml` builds on the
`codex/taproot-runtime-build` branch when runtime inputs change. Once merged to the
default branch it also supports manual dispatch. The artifact is
`taproot-runtime-COMMIT`, containing `taproot-course.tar`, `SHA256SUMS`, and
`BUILD_COMMIT`. It expires after seven days. The workflow builds and tests; it
does not deploy the website or upload to the production asset bucket.

```sh
gh run download 44966726796 --repo base58btc/base58-web \
  --name taproot-runtime-d284234d8b85aae1272c9e28488c148dae09606a \
  --dir target/taproot-download
# Check SHA256SUMS and extract into a temporary directory before copying the
# manifest and runtime/ into courses/taproot. Preserve later lesson edits.
./target/course-sync verify-bundle --dir courses/taproot
```

The full app discovers `courses/taproot` automatically. After importing the
existing catalog into the intended database, run this explicit one-time action:

```sh
go run ./cmd/course-catalog activate-taproot
```

Activation updates the course link, descriptions, chapter list, and included
materials. It preserves the original ID, slug, title, artwork, price, availability,
visibility, difficulty, formats, student data, and historical class sessions.
Rerunning it preserves later admin edits. No new database migration is required.
Local activation has been applied; production activation and runtime archive
uploads are still needed when this branch is deployed. Use the existing course
asset publishing workflow, then verify the deployed environment endpoint before
opening enrollment to the new material.
