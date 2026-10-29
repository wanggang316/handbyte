# handbyte

An interactive macOS disk cleanup CLI.

`handbyte` helps you review caches and build output before cleanup. It groups
candidates by application or project, explains the rebuild cost, and checks
eligibility again before each action. Native macOS Trash is the default.

Large personal-data stores remain inspection-only. Use directory exploration to
see what is inside them without giving the cleaner permission to remove them.

## Install

One-liner (downloads the latest release, installs to `/usr/local/bin`):

```sh
curl -fsSL https://handbyte.gumpw.com/install.sh | bash
```

Pin a version, or install to a user-writable prefix:

```sh
curl -fsSL https://handbyte.gumpw.com/install.sh | VERSION=v0.2.0 bash
curl -fsSL https://handbyte.gumpw.com/install.sh | PREFIX=$HOME/.local bash
```

From source (requires Go 1.26+):

```sh
go install github.com/wanggang316/handbyte@latest   # or
make install                                        # → $HOME/.local/bin/handbyte
```

## Uninstall

Removing the binary uninstalls the executable. Settings and the local action
journal remain in `~/Library/Application Support/handbyte/` on macOS so you can
keep exclusions and recovery records. Remove that directory separately only if
you no longer need those records.

```sh
# One-liner (same script, UNINSTALL=1 mode).
curl -fsSL https://handbyte.gumpw.com/install.sh | UNINSTALL=1 bash

# If you installed to a custom prefix:
curl -fsSL https://handbyte.gumpw.com/install.sh | UNINSTALL=1 PREFIX=$HOME/.local bash

# From source:
make uninstall                          # uses PREFIX (default $HOME/.local)

# Manual:
sudo rm "$(command -v handbyte)"
```

## Usage

```sh
handbyte                         # Review candidates in the terminal
handbyte --dry-run               # Preview; no cleanup actions
handbyte --scope=quick           # Known caches and Xcode data
handbyte --scope=projects        # Project artifacts only
handbyte --explore=~/Downloads   # Read-only directory explorer
handbyte --min=200M              # Minimum candidate size
handbyte --roots=~/work          # Override project roots for this scan
handbyte --roots=~/work --save-roots
handbyte --no-projects           # Skip project artifact discovery
handbyte --protect=~/work/keep    # Save a protected path
handbyte --unprotect=~/work/keep  # Remove that exact protection
handbyte --scan-only             # JSON inventory with coverage and eligibility
handbyte --history               # Local action records as JSON
handbyte --restore=ACTION_ID     # Restore a recorded Trash action
handbyte --rm                    # Explicit permanent deletion mode
```

The start screen offers Quick cleanup, Project cleanup, and Explore storage.
No items start selected. The default scan retains the existing `all` scope;
`--roots` overrides saved roots without changing them unless `--save-roots` is
also supplied. Default project roots are `~/dev`, `~/Projects`, `~/Code`, and
`~/workspace`.

## Review controls

| Key | Action |
| --- | --- |
| j/k or arrows | Navigate |
| Space | Toggle an eligible item |
| a | Toggle eligible items in the current filtered group |
| A | Toggle recommended items |
| c | Clear selection |
| / | Search names, paths and descriptions |
| f / s | Cycle eligibility filter / sort order |
| Enter | Full details, collapse a group, or enter a directory in Explore |
| p | Add or remove an exact persistent path protection |
| e / o | Explore a directory / reveal it in Finder |
| Backspace / Esc | Previous directory / leave Explore |
| d | Review the complete selection |
| y | Confirm on the review screen; Enter does not confirm |
| r | Rescan and clear selection |
| q / Ctrl-C | Quit; during cleanup, stop after the current action |
| ? | Help |

Long confirmations and details scroll with j/k or Page Up/Down. Results provide
per-item reasons. On the results screen, `u` restores the highlighted Trash
item, `f` reviews failed/skipped/cancelled items for retry, and `r` rescans.
Successful actions are not included in a retry.

## Eligibility and recovery

| Group | Meaning |
| --- | --- |
| Recommended | A supported rule, complete scan, no detected owner process, and modification activity older than seven days |
| Review | A supported, idle target whose rebuild or download cost needs review; recent activity is not recommended automatically |
| Protected / inspect | Personal state, exclusions, active or unknown owners, incomplete scans, tracked files, nested repositories, or unsupported cleanup rules |

Modification age is not last use. Process checks are conservative: an active
process in the same tool family can protect unrelated caches or projects.
Rebuilding dependencies can require network access. A cache label alone does
not establish that data is disposable.

Before applying an action, handbyte rechecks the target identity, path, rule,
exclusions, contents, Git ownership, modification activity and owner processes.
A changed or uncertain target is skipped. Parent/child selections cannot execute
twice. Child symlinks are not followed; a changed target or ancestor symlink
requires a new scan. These checks do not provide an atomic filesystem lease.

- **Preview:** reports potential actions; it does not claim that space was freed.
- **Trash:** uses Finder and records the actual destination. macOS may request
  permission to control Finder. No permanent deletion fallback is used. Moving
  to Trash retains disk usage until the Trash is emptied separately.
- **Permanent deletion:** requires `--rm` and explicit review. Removed-byte totals
  are estimates. A volume free-space delta can include unrelated system activity.
- **Restore:** checks the recorded Trash identity and refuses to overwrite an
  existing original path. If the Trash item or parent changed, inspect it in
  Finder. Recovery is unavailable after permanent deletion or emptying Trash.
- **Cancellation:** stops before the next action, waits for the current operation,
  and reports remaining actions as cancelled. A crash or timeout can leave an
  uncertain intent record; inspect the filesystem before retrying.

## Cleanup depth

- Xcode DerivedData is split by project; DeviceSupport is split by version.
  Archives and dSYMs remain protected. Unavailable simulators are listed through
  `simctl` for inspection only.
- Project artifacts require a matching ecosystem marker, such as `package.json`,
  `Cargo.toml`, `pom.xml`, `Package.swift`, `pyproject.toml` or a build manifest.
  Nested projects and Git worktrees are inspected without deleting their roots.
- Known Go, Cargo, npm, Yarn, Gradle, SwiftPM, Homebrew and tool caches retain
  dedicated explanations. Unknown application/XDG caches remain inspection-only.
- Downloads, sandbox containers, toolchains, large shared stores and VM data are
  inspection entries. This release does not perform blanket container pruning,
  remove worktrees, or clean AI conversations, credentials and models.

## Settings and JSON

On macOS, `config.json` and `actions.jsonl` live in
`~/Library/Application Support/handbyte/`. Settings use version 1 and contain
`roots` and `exclusions`. An unreadable or invalid config stops cleanup; it is
never treated as an empty protection list. Excluding a child also protects its
ancestors from deletion. To remove a protection inherited from a parent, remove
that exact parent path with `--unprotect`.

`--scan-only` keeps the existing `free_bytes` and `items` fields and adds
`schema_version: 2`, `scan_status`, and `warnings`. Item fields now include owner,
project, identity, eligibility, reason, activity and scan coverage. The old
numeric `Safety` field remains for compatibility; use `Eligibility` for cleanup
authority. JSON inventory is not an executable cleanup plan.

Coverage is `complete`, `partial`, `unavailable` or `cancelled` per item; Explore
also uses `pending` while measuring. Partial sizes are lower bounds, not zero or
empty directories. A completed scan can exit successfully with partial coverage;
automation must check `scan_status`. Cancellation and fatal scan errors retain
available JSON results and exit nonzero. Size estimates do not guarantee unique
physical blocks across APFS clones or hard-linked stores.

The journal stores paths, action IDs, outcomes and Trash destinations, not file
contents. It is local, user-writable evidence, not a transaction log with crash
recovery or a tamper-proof audit trail. Keep it until recovery is no longer needed.

## Validation

```sh
go test -race ./...
go vet ./...
go build .
# Opt in to moving and restoring disposable fixtures through native macOS Trash:
HANDBYTE_NATIVE_TEST=1 go test ./internal/cleaner -run TestNativeTrashCollisionAndRestore -v
```
