This is the Base58 website.

Currently a work in progress.

## Course environments in the browser

The architecture is **build at publish time; execute in the browser**. The
publication workflow prepares the locked course environment on an author/CI
Linux builder and uploads it before publishing the course version. Students
download that prepared environment; their command history and VM checkpoints
sync to their account for resume and undo.

Course commands run in a Linux guest inside the student's browser, using the
[unkernel](https://github.com/pithings/unkernel) engine used by
[Try Nix](https://github.com/fzakaria/trynix). The website serves course content,
environment metadata, and progress. It does not execute student commands, operate
VMs, or queue environment builds.

A course repository contains `flake.nix`, `flake.lock`, Markdown, and any imported
scripts/configuration. The flake exposes `devShells.aarch64-linux.default`:

```nix
{
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  outputs = { nixpkgs, ... }: {
    devShells.aarch64-linux.default =
      let pkgs = nixpkgs.legacyPackages.aarch64-linux;
      in pkgs.mkShellNoCC {
        packages = [ pkgs.bitcoind pkgs.jq ];
        shellHook = ''
          mkdir -p "$HOME/node"
          if ! bitcoin-cli -regtest -datadir="$HOME/node" getblockchaininfo >/dev/null 2>&1; then
            bitcoind -regtest -datadir="$HOME/node" -daemonwait \
              -listen=0 -dnsseed=0 -discover=0 -connect=0 \
              -dbcache=32 -maxmempool=5 -par=1
          fi
        '';
      };
  };
}
```

Use `mkShellNoCC` for courses that do not need a compiler to keep browser downloads
small. The browser downloads and expands the shell's full closure.

### Prepare and publish

Run `nix flake lock` in the course repository, then prepare the environment on an
aarch64 Linux machine or a Nix host configured with an appropriate remote builder:

```sh
course-sync bundle --dir ./taproot
course-sync sync --server http://localhost:8080 --course taproot --dir ./taproot
course-sync publish --server http://localhost:8080 --course taproot
```

`bundle` evaluates the locked flake with `nix print-dev-env`, captures shell
variables/functions without running the shell hook, and exports the resulting
Nix store closure as gzip-compressed NAR files. It writes `course-runtime.json`
and `runtime/<source-hash>/*.nar.gz` inside the course directory. This synchronous
author/CI step needs Nix; the student's browser does not evaluate the flake or
build packages.

`sync` uploads the archives using the existing course asset settings:
`COURSE_ASSET_BUCKET`, `COURSE_ASSET_REGION`, `COURSE_ASSET_PREFIX`, `SPACES_KEY`,
and `SPACES_SECRET`. The website needs the matching `COURSE_ASSET_*` settings.
The asset origin must allow browser CORS GET requests. `--no-assets` is available
when archives are hosted separately at the same course asset URLs.

The generated manifest is versioned alongside the lessons. Its fingerprint
covers every uploaded source file, including imported `.nix`, `.py`, `.sh`,
`.json`, `.toml`, `.yaml`, `.yml`, `.txt`, and `.conf` files. Publishing rejects a
missing or stale bundle. Re-run `bundle` after editing course sources. Hidden
files and unrelated binary files are excluded. Add `runtime/` to the course
repository's `.gitignore`; generate the archives again in CI if needed.

Published archives use immutable paths and are retained when another version is
synced, because older student attempts still reference them. The browser verifies
each compressed archive against the SHA-256 in the pinned course manifest before
unpacking it, and caches it for subsequent visits.

### Lesson cells and saved work

```python
height = 0
!bitcoin-cli -regtest -datadir=/data/workspace/node getblockhash {height}
```

Python continues to run through Pyodide. Lines starting with `!` run in the course
VM through unkernel's JavaScript API. Scalar `{identifier}` values are passed as
individual arguments. Pipes, redirects, and shell expansion are not implicit;
a lesson can explicitly invoke `bash -c` when it needs a shell script.

The course source files are copied into `/data/workspace` on first boot. That is
also `HOME` and the command working directory.

Each student attempt has an append-only CLI journal, pinned to its course version
and environment hash. Before dispatching a command, the browser saves its resolved
program/arguments, original source, lesson/cell identifiers, and timestamp. It
then saves stdout, stderr, exit status, and a VM checkpoint. Failed commands are
included: a command can modify files before returning a nonzero exit status.

The checkpoint captures **guest memory and the matching disk generation**, not
just files. This preserves running services, generated wallet keys, and other
state that would change if commands were simply replayed. The VM is suspended
between commands; it resumes from the last checkpoint for the next command,
without rerunning `shellHook`. An idempotent hook is still needed for the initial
boot and existing workspaces created before checkpoint support.

Published courses save the journal and checkpoints to the student's account, so
**Save and stop environment** followed by signing in on another device resumes
the same CLI state. Checkpoints are compressed and split into content-addressed
pieces; unchanged pieces are shared across checkpoints within the attempt. The
browser also caches them in IndexedDB. A command is shown as saved only after its
checkpoint upload and journal result both succeed. Network/storage errors block
further commands until the environment is reopened. An interrupted command with
no completed checkpoint stays in the log and returns to the previous saved state;
it is never silently executed twice.

**Undo last command** restores the previous checkpoint and moves the history
cursor back. The original command and result remain visible as undone. Running a
new command after undo creates a new branch in the journal. Checkpoints avoid
replay changing Bitcoin addresses or transaction IDs. Ordinary replay alone is
not a guarantee of identical state for arbitrary CLI commands.

One browser tab holds the local workspace lock, and a two-minute renewable server
lease prevents two devices from advancing the same attempt concurrently. Saving
and stopping releases it immediately; a lost connection or closed tab releases it
when the lease expires. Clearing browser storage does not delete account-backed
work. Draft/local previews keep their journals and checkpoints on that device.
The history panel displays the latest 100 commands; the complete journal is kept.

Apply migration `000013_create_course_command_events` before using the account
history API. This implementation keeps private checkpoint chunks in PostgreSQL
alongside the journal, scoped to the authenticated attempt and protected by CSRF
for writes. Checkpoint storage grows with the history: size the database and its
backups accordingly; moving the chunk transport to private object storage is a
future optimization. Undo retains old branches and their checkpoints.

The checkpoint covers the Linux CLI guest. Pyodide's Python interpreter is a
separate runtime; this does not snapshot its in-memory globals. Existing lesson
progress, cell text, and displayed output still use the normal progress API.
The guest's host connections are reset on restore by unkernel; there are no
external network connections in this course integration.

No external network device is attached to the guest. Its loopback interface lets
`bitcoind`, `bitcoin-cli`, and other services in that course communicate locally.
Commands have a 30-second timeout and a 1 MiB output limit per stream. Shell
initialization has a 60-second timeout.

### Website build

```sh
make course-vm-assets  # download and verify the pinned browser SDK/kernel
make build-all
make test
# Preview an already prepared course without a database or uploads:
python3 scripts/preview-course-vm.py --dir ./taproot
```

The Nix website package includes these assets automatically. The local asset
installer downloads `unkernel@0.2.0-261119-040e142`, verifies its archive SHA-256,
and places it under `static/vendor/`. Generated SDK binaries are ignored by Git.
The SDK's MIT license and the kernel's GPL license ship beside the runtime;
[the corresponding source revision](https://github.com/pithings/unkernel/tree/040e142)
contains its build instructions and kernel patches. The NAR reader is adapted
from Try Nix, with attribution in `static/licenses/trynix.txt`.

This integration currently supports unkernel's **aarch64 Linux** guest and needs
a browser with WebAssembly JSPI. It does not include Try Nix's x86-64 QEMU fallback.
The pinned SDK restores host filesystem mounts in inline mode, so this runtime
currently runs on the page thread. Large environments, checkpoint costs, and
CPU-heavy work should be measured on students' target devices. No runner service
or runner secret is needed; the command-history migration is required.

## Testing Stripe Hooks

Stripe has [great instructions](https://stripe.com/docs/payments/handling-payment-events) for how to test their hook callbacks. Here's a quick how-to though.

1. Install the `stripe` utility
2. Register with `./stripe login`. You can use your existing API key with `./stripe login --api-key <KEY>`
3. You'll need two terminal windows. In one, startup the console: 

	./stripe listen --forward-to http://localhost:8080/stripe-hook

4. In a 2nd window, send an event. You'll need to include some metadata to get things to work.

	./stripe trigger payment_intent.succeeded --add payment_intent:metadata['registration_id']=<reg_id>

You can get a registration ID from Notion by going to the class signups and opening one of them as a Page. The Page ID (listed in the URL) is the `reg_id`.


## How to format a golang project

```
	gofmt -s -w .
```
