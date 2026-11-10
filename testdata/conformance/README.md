# Conformance fixtures

The CLI and the macOS app are two implementations of the same rules. Each
file in this directory is one case that both must pass. The app repository
keeps a byte-identical copy, and its `scripts/sync-check.sh` compares the two.

## cleanup

A case builds a home directory in a temporary location, runs a scan with no
size floor and compares the candidates that the scan reports.

| Field | Meaning |
| --- | --- |
| `name` | Short name, shown when the case fails |
| `projectRoots` | Project roots for the scan, as `~/` templates. Empty: no project scan. |
| `files` | Files to make, as paths relative to the home directory, each with a size in bytes |
| `expect` | The complete set of candidates: the path relative to the home directory, the rule ID, the category ID, the safety tier and the area |

A case contains only behavior that the two products agree on. A known
difference goes into `SYNC.md` of the app repository, not into a case.

## usage

A case builds a directory tree in a temporary location, scans it with the
storage engine and compares the result.

| Field | Meaning |
| --- | --- |
| `files` | Files to make, as paths relative to the root, each with a size in bytes |
| `hardLinks` | Extra names for a file of `files` |
| `clones` | APFS clones of a file of `files`, made with `clonefile`. A case with clones is skipped on a volume that cannot clone. |
| `symlinks` | Symbolic links, with the target relative to the root |
| `expect.files`, `expect.logicalBytes` | Totals of the root. A file with several names counts once. When the names are in one directory, the name that sorts first carries the size. A full clone counts as a file with its logical size, but its allocated size counts once; in one directory, the name that sorts first carries it. A symbolic link counts as one file with the length of its target text. |
| `expect.children` | The direct children of the root, largest first |
| `expect.largestFiles` | The largest files of the tree, largest first, as paths relative to the root |

The cases compare logical sizes, because the allocated size of a file
depends on the file system.

## categories

A case builds a home directory, scans it with the storage engine and sums
the content by the categories of `catalog/storage.json`.

| Field | Meaning |
| --- | --- |
| `projectRoots` | Project roots, as `~/` templates. They belong to a category with `"projectRoots": true`. |
| `files` | Files to make, as paths relative to the home directory |
| `expect` | Logical bytes for each category ID. A category with no bytes is left out. |

The rule: a folder belongs to the category of the longest path rule that
is the folder or one of its parents, and to `other` when no rule matches.
When a path rule lies below a folder, the folder is split: each child is
measured by the same rule, and the files directly in the folder go to the
category of the folder.

## apps

A case builds a home directory and finds the files of one app, or the
leftovers of apps that are gone. A path in `files` ends with a file; its
folders are made.

| Field | Meaning |
| --- | --- |
| `app` | The bundle ID, the name and the cask `zap` paths of the app |
| `others` | Bundle IDs of the other installed apps |
| `withData` | The option that also selects the app's data |
| `installed`, `installedNames` | For leftovers: every known bundle ID, and the names of the installed apps |
| `expect` | The complete result in path order: path relative to the home directory, class, confidence and the default selection; for leftovers the bundle ID and the class |

The rules are in `catalog/apps.json`: a bundle ID matches only at a dot
boundary; an app name needs three characters and must not be a generic
word; a name that belongs more exactly to another installed app is not
taken; `com.apple.*` is never offered. A leftover is a folder (or, in Preferences, a `.plist` file) named by a bundle ID whose vendor (the first two parts, `com.example`) has no installed app, none of whose parts is the vendor name (`example`) or the name of an installed app, that holds no UUID and that is not a shared library or updater of the ignore list. Caches, logs and state are selected,
settings are selected for a bundle ID match, data only with `withData`, and
a low match never.

## journal

The CLI and the app write one journal:
`~/Library/Application Support/handbyte/actions.jsonl`. Each line is one
JSON record; the last line of an ID is the present state of that action. A
case gives journal lines from both products and the state that each
product must read.

| Field | Meaning |
| --- | --- |
| `lines` | Journal lines, oldest first |
| `expect` | One entry for each ID, in the order of its first line: the last `status`, the `outcome` that the app shows (`moved`, `removed`, `failed`, `restored`, or `none` for an action that changed nothing), whether a restore is possible, the `group` and the bytes moved or freed |

Keys: `ID`, `Path`, `Status` (`started`, `trashed`, `removed`, `failed`,
`skipped`, `previewed`, `cancelled`, `restoring`, `restored`),
`Destination` (the Trash place), `TrashIdentity` (`Device` and `Inode` of
the Trash item; a restore needs it), `Moved` and `Freed` (bytes), `Message`,
`StartedAt` and `EndedAt` (RFC 3339), `Group` (one uninstall), `Rule` and
`Tool` (`cli` or `app`). A reader ignores keys it does not know. A writer
appends one complete line and refuses to write after a line without its
newline.
