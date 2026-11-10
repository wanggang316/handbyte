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
| `expect` | The complete set of candidates: the path relative to the home directory, the rule ID, the category ID and the safety tier |

A case contains only behavior that the two products agree on. A known
difference goes into `SYNC.md` of the app repository, not into a case.

## usage

A case builds a directory tree in a temporary location, scans it with the
storage engine and compares the result.

| Field | Meaning |
| --- | --- |
| `files` | Files to make, as paths relative to the root, each with a size in bytes |
| `hardLinks` | Extra names for a file of `files` |
| `symlinks` | Symbolic links, with the target relative to the root |
| `expect.files`, `expect.logicalBytes` | Totals of the root. A file with several names counts once. When the names are in one directory, the name that sorts first carries the size. A symbolic link counts as one file with the length of its target text. |
| `expect.children` | The direct children of the root, largest first |
| `expect.largestFiles` | The largest files of the tree, largest first, as paths relative to the root |

The cases compare logical sizes, because the allocated size of a file
depends on the file system.
