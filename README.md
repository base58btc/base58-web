This is the Base58 website.

Currently a work in progress.

## Course environments in the browser

The architecture is **build at publish time; execute in the browser**. The
publication workflow prepares the locked course environment on an author/CI
Linux builder and uploads it before publishing the course version. Students
download that prepared environment; their command history
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

The Lightning workshop port is in `courses/lightning`; see
[its notebook mapping and validation instructions](docs/lightning-port.md).
Cells marked `+++ id python-vm` run the flake's Python inside the VM, including
native libraries and persistent globals. Existing unmarked cells use Pyodide.

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

Unmarked Python cells run through Pyodide. Lines starting with `!` run in the course
VM through unkernel's JavaScript API. Scalar `{identifier}` values are passed as
individual arguments. Pipes, redirects, and shell expansion are not implicit;
a lesson can explicitly invoke `bash -c` when it needs a shell script.

The course source files are copied into `/data/workspace` on first boot. That is
also `HOME` and the command working directory.

Each student attempt has an append-only command journal, pinned to its course
version and environment hash. Before dispatching a command, the browser saves its
resolved program/arguments, original source, lesson/cell identifiers, timeout,
and timestamp. It then records stdout, stderr, and exit status. Failed commands
are included because they can change state before returning a nonzero exit code.

The VM and Python interpreter stay alive between cells and lesson pages. Course
links and browser back/forward replace the lesson content without unloading the
runtime. Navigation waits for in-flight cells and progress writes, and retains
editor drafts, outputs, and answers. Leaving the course or selecting Save and stop
closes the environment; reopening or reloading the page replays the command log.

Python cells travel over
one open input/output pipe to that interpreter; they do not launch a shell or
another Python client for each cell. The recorded Python CLI arguments remain
compatible with older journals and are dispatched over the same pipe on replay.
Guest files and memory are temporary: **no VM snapshots or disk images are saved**.
On resume, a fresh
VM starts from the pinned course bundle, runs its setup hook, and replays the
completed commands on the active history branch. Replaying does not append new
executions or overwrite the original results. Commands interrupted before a
result was recorded remain in the audit log but are excluded from replay.

**Undo last command** moves the history cursor to the previous command and
rebuilds the environment by replaying that prefix. Undone commands and results
remain in the log. Executing another command after undo creates a new branch.
If a command's exit status changes during replay, or it times out, replay stops
and identifies the command. Further undo can move back to a replayable prefix.

**Reset workspace** in the Workspace menu starts a fresh VM with an empty replay
history. A confirmation explains that variables, created files, and running nodes
will be cleared; code edits and lesson answers stay. The append-only `reset` event
archives previous commands and their output, which remain in Command history.
Undo cannot cross that boundary. Reset can interrupt background replay and recover
a failed or stopped workspace without executing its old commands. Archived cell
output is cleared from lessons; prerequisite cells must be run again.

Replay is deliberately less exact: randomness, clocks, wallet keys, addresses,
transaction IDs, and other generated values can differ. A command containing an
old generated value may no longer work. Changes outside recorded commands are
not recoverable. Resume and undo take time proportional to the commands replayed;
normal cell execution has no snapshot/restore cost. Saved output describes the
original execution, not necessarily the current replayed values.

Published courses save the journal to the student's account, so **Save and stop**
followed by signing in elsewhere can replay the same history. A local preview
keeps its journal in IndexedDB. The existing database name is retained to reuse
older command logs; old snapshot stores are left untouched and unused.

One browser tab holds the local workspace lock, and a two-minute renewable server
lease prevents two devices from advancing the same attempt concurrently. Saving
and stopping releases it immediately. Network/storage errors stop execution until
recovery. The history panel displays the latest 100 commands in each of its current
and archived groups; replay uses the
complete active branch, including failed commands with recorded results.

Apply migration `000013_create_course_command_events` before using account history.
The journal is stored in PostgreSQL, scoped to the authenticated attempt, with CSRF
protection for writes. Legacy checkpoint/chunk endpoints return HTTP 410 and no
longer read or write VM state; their historical database rows are retained.

`python-vm` cells reconstruct their shared Python namespace through replay.
Unmarked Pyodide cells still use a separate interpreter and are not part of the
VM journal. Lesson progress, cell text, and displayed output use the normal
progress API.

No external network device is attached to the guest. Its loopback interface lets
`bitcoind`, `bitcoin-cli`, and other services in that course communicate locally.
CLI commands default to a 30-second timeout, Python cells to 120 seconds, with a
1 MiB output limit per stream. Shell
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
This runtime currently runs on the page thread. Large environments, replay costs, and
CPU-heavy work should be measured on students' target devices. No runner service
or runner secret is needed; the command-history migration is required.

## Retired class checkout and newsletters

Class checkout, newsletter signup, course waitlists, campaign scheduling, and
the email-test endpoint have been removed. Old URLs return HTTP 410. The footer
and course pages no longer collect mailing-list addresses. Student/admin login,
course enrollment, course progress, and contact emails continue to work.

This change does **not** delete, export, or migrate historical Notion data. Keep
the existing databases and their original IDs for the later rebuild:

| Former setting | Historical records to retain |
| --- | --- |
| `NOTION_NEWSLETTER` | Subscriber addresses and list memberships |
| `NOTION_WAITLIST` | Legacy waitlist addresses and class references |
| `NOTION_MISSIVES` | Newsletter Markdown, scheduling rules, and sent status |
| `NOTION_SIGNUPS` | Registration addresses and payment references |
| `NOTION_CARTS`, `NOTION_ITEM_CARTS` | Both cart formats, contact details, and payment references |

These settings are no longer loaded by the web app. Preserve their values in
private operational records for export/import; do not delete the databases or
their credentials. Keep the mailer service's historical records too. Catalog
and scheduled-class metadata still use `NOTION_COURSES` and `NOTION_SESSIONS`.

Old unsubscribe links remain functional as a narrow compatibility endpoint.
Migration `000014` records opt-outs in Postgres `email_opt_outs`, without deleting
the original subscriber data in Notion. The endpoint cancels pending messages
using the original mailer subscription key; cancellation failures return 503
and can be retried using the same link. Rows with `mailer_cancelled_at IS NULL`
need reconciliation. Preserve `TOKEN_SEC`, `MAIL_API`, and `MAIL_SEC`; the mailer
also remains necessary for account and contact emails. Any future mailing-list
import must honor `email_opt_outs`, as well as existing unsubscribe/suppression
records, before sending anything. The down migration retains opt-outs.
Do not resume sending directly from the old Notion lists: new unsubscribe
requests are recorded in Postgres, and no longer update those lists.

Before deploying this removal, reconcile outstanding Stripe/OpenNode purchases
and disable the legacy payment integrations at the providers. The old webhook
URLs return 503 rather than acknowledge unprocessed payments; retries alone
are not a reconciliation strategy. Export and reconcile queued newsletter jobs
in the separate mailer, then cancel those campaigns there. Removing the website
scheduler does not cancel jobs already queued in another service. These external
changes are not performed by updating this repository.

The remaining `checkout` package and `CourseItem` type only decode old carts in
existing browser sessions, preserving sign-in data. They cannot create orders,
calculate prices, initiate payments, or run registration callbacks.


## How to format a golang project

```
	gofmt -s -w .
```
