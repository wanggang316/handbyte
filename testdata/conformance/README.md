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
