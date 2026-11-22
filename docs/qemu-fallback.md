# QEMU course compatibility runtime

The browser checks for both `WebAssembly.Suspending` and
`WebAssembly.promising`. If either is absent, it chooses QEMU. `?engine=qemu`
forces this path for testing on a newer browser. This override is a diagnostic
URL option, not a setting saved in a student's account.

## Device requirements

The course code currently requires Safari/iOS **17.4 or newer**, based on
`AbortSignal.any` (17.4), Compression Streams (16.4), and shared WebAssembly
memory with COOP/COEP (15.2). Safari 27+ can use the faster JSPI engine.
These are API requirements, not a tested hardware support matrix.

Apple lists iPhone XS/XR (2018) among devices capable of running iOS 17. Those
models are possible testing candidates, not a promised minimum. Actual memory
limits, CPU speed, browser eviction, and the particular exercises matter.
The fallback reserves **1536 MiB of shared WebAssembly memory**, plus the
decompressed course files, compiled code, and page memory. A device able to open
the lessons may still be unable to run their exercises. Physical iPhone/Safari
testing remains required before declaring support for a model.

Sources: [Safari 17.4](https://webkit.org/blog/15063/webkit-features-in-safari-17-4/),
[Safari 16.4](https://developer.apple.com/documentation/safari-release-notes/safari-16_4-release-notes),
[Safari 15.2](https://webkit.org/blog/12140/new-webkit-features-in-safari-15-2/),
[Apple's device support table](https://www.apple.com/environment/pdf/Longevity_by_Design-June_2024-Apple.pdf).

## Runtime and memory

The engine comes from Try Nix's `engine-20260913-2056` release. Its Emscripten
build imports a fixed 2300 MiB memory. `course-qemu-memory.mjs` lowers that
import's minimum to 1536 MiB before instantiation; `INITIAL_MEMORY` supplies a
matching allocation. It rejects an unexpected declaration and does not modify
instructions or static data. A 1024 MiB heap booted Python but exhausted the
emulator's heap in Lightning's Bitcoin exercise, so it is not the default.
The guest has 512 MiB RAM and a 128 MiB TCG translation cache.

The guest cold-boots from a Linux kernel and initramfs. There is no `vm.state`
download and no user snapshot. Published course and Nix files are
provided through an in-memory 9p share. Relative symlinks are expanded before
mounting to avoid Emscripten's readlink/stat length mismatch. Student files live
under `/data/workspace`. Stopping or undoing terminates the worker and its guest.

The serial agent imports the same `course-python.py` evaluator used by unkernel
and keeps one Python namespace. Shell commands retain their argv boundaries.
Private protocol descriptors keep Python prints and subprocess output from
corrupting response frames. The host records each command before dispatch and
its result afterward. Replay never creates additional command events. Switching
architectures preserves history, but replay can still change generated values
or stop if a command's outcome differs.

The guest enables loopback for Bitcoin/Core Lightning RPC and has **no network
device**. This fallback does not introduce remote execution or a server VM.

## Assets and builds

`make course-vm-assets` installs and SHA-256 checks all engine files using Go.
`cmd/course-vm-assets/qemu-assets.json` is also consumed by the Nix website
package. The kernel and initramfs are checked into `cmd/course-vm-assets/guest/`
and embedded in the downloader because the upstream immutable release omits
them; relying on the changing `trynix.dev` deployment would break old builds.
Their original hashes and provenance remain in the pin file. Other assets use
versioned releases, a commit-pinned GitHub URL, or a version-pinned npm CDN URL.
Licenses and corresponding-source links ship in `static/licenses/qemu/`.

Run the `Build QEMU course runtimes` GitHub Actions workflow for native x86
builds and lesson validation. Download the two artifacts, verify their
`SHA256SUMS`, and extract each into its course directory. Then run:

```sh
target/course-sync verify-bundle --dir courses/lightning --system x86_64-linux
target/course-sync verify-bundle --dir courses/taproot --system x86_64-linux
```

Both architecture manifests exclude each other from runtime/content hashes.
The ordinary sync tool validates and uploads the union of both archive sets.
When runtime inputs change, rebuild both architectures before publishing. A
student's pinned version must contain the fallback manifest and matching assets;
deploying website code alone cannot add a fallback to an older published version.

## Local verification

```sh
make course-vm-assets
make test
python3 scripts/preview-course-vm.py --dir courses/lightning --port 8130
# Open http://127.0.0.1:8130/?engine=qemu
```

The preview servers send the required isolation headers. Production sends them
only for VM course documents and workers. Google font CSS and external course
images load with CORS; externally embedded videos in a VM course must also
permit isolated embedding.

Validation includes native x86 Linux lesson suites in Actions, manifest and
Postgres journal identity tests, command transport tests, and a forced-QEMU
Chromium browser check. Browser checks cover persistent Python, stop/resume,
undo, Bitcoin regtest wallet startup, a subsequent RPC cell, secp256k1 signing
and verification, and all three Core Lightning nodes starting together. These checks
are not a substitute for running the course on physical older iPhones.
